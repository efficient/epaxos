package genericsmr

import (
	"bufio"
	"encoding/binary"
	"fastrpc"
	"fmt"
	"genericsmrproto"
	"io"
	"log"
	"net"
	"os"
	"state"
	"time"
	"sync"
	"dlog"
	"math"
	"encoding/json"
)

const CHAN_BUFFER_SIZE = 200000
const TRUE = uint8(1)
const FALSE = uint8(0)

type RPCPair struct {
	Obj  fastrpc.Serializable
	Chan chan fastrpc.Serializable
}

type Propose struct {
	*genericsmrproto.Propose
	Reply *bufio.Writer
	Mutex *sync.Mutex
}

type Beacon struct {
	Rid       int32
	Timestamp int64
}

type Replica struct {
	N            int        // total number of replicas
	Id           int32      // the ID of the current replica
	PeerAddrList []string   // array with the IP:port address of every replica
	Peers        []net.Conn // cache of connections to all other replicas
	PeerReaders  []*bufio.Reader
	PeerWriters  []*bufio.Writer
	Alive        []bool // connection status
	Listener     net.Listener

	State *state.State

	ProposeChan chan *Propose // channel for client proposals
	BeaconChan  chan *Beacon  // channel for beacons from peer replicas

	Shutdown bool

	Thrifty bool // send only as many messages as strictly required?
	Exec    bool // execute commands?
	LRead   bool // execute local reads?
	Dreply  bool // reply to client after command has been executed?
	Beacon  bool // send beacons to detect how fast are the other replicas?

	Durable     bool     // log to a stable store?
	StableStore *os.File // file support for the persistent log

	PreferredPeerOrder []int32 // replicas in the preferred order of communication

	rpcTable map[uint8]*RPCPair
	rpcCode  uint8

	Ewma []float64
	Latencies []int64

	Mutex sync.Mutex

	Stats *genericsmrproto.Stats
}

func NewReplica(id int, peerAddrList []string, thrifty bool, exec bool, lread bool, dreply bool) *Replica {
	r := &Replica{
		len(peerAddrList),
		int32(id),
		peerAddrList,
		make([]net.Conn, len(peerAddrList)),
		make([]*bufio.Reader, len(peerAddrList)),
		make([]*bufio.Writer, len(peerAddrList)),
		make([]bool, len(peerAddrList)),
		nil,
		state.InitState(),
		make(chan *Propose, CHAN_BUFFER_SIZE),
		make(chan *Beacon, CHAN_BUFFER_SIZE),
		false,
		thrifty,
		exec,
		lread,
		dreply,
		false,
		false,
		nil,
		make([]int32, len(peerAddrList)),
		make(map[uint8]*RPCPair),
		genericsmrproto.GENERIC_SMR_BEACON_REPLY + 1,
		make([]float64, len(peerAddrList)),
		make([]int64, len(peerAddrList)),
		sync.Mutex{},
		&genericsmrproto.Stats{make(map[string]int)}}

	var err error

	if r.StableStore, err = os.Create(fmt.Sprintf("stable-store-replica%d", r.Id)); err != nil {
		log.Fatal(err)
	}

	for i := 0; i < r.N; i++ {
		r.PreferredPeerOrder[i] = int32((int(r.Id) + 1 + i) % r.N)
		r.Ewma[i] = 0.0
		r.Latencies[i] = 0
	}

	return r
}

/* Client API */

func (r *Replica) Ping(args *genericsmrproto.PingArgs, reply *genericsmrproto.PingReply) error {
	return nil
}

func (r *Replica) BeTheLeader(args *genericsmrproto.BeTheLeaderArgs, reply *genericsmrproto.BeTheLeaderReply) error {
	return nil
}

/* ============= */

func (r *Replica) ConnectToPeers() {
	var b [4]byte
	bs := b[:4]
	done := make(chan bool)

	go r.waitForPeerConnections(done)

	//connect to peers
	for i := 0; i < int(r.Id); i++ {
		for done := false; !done; {
			if conn, err := net.Dial("tcp", r.PeerAddrList[i]); err == nil {
				r.Peers[i] = conn
				done = true
			} else {
				time.Sleep(1e9)
			}
		}
		binary.LittleEndian.PutUint32(bs, uint32(r.Id))
		if _, err := r.Peers[i].Write(bs); err != nil {
			fmt.Println("Write id error:", err)
			continue
		}
		r.Alive[i] = true
		r.PeerReaders[i] = bufio.NewReader(r.Peers[i])
		r.PeerWriters[i] = bufio.NewWriter(r.Peers[i])

		log.Printf("OUT Connected to %d", i)
	}
	<-done
	log.Printf("Replica id: %d. Done connecting to peers\n", r.Id)
	log.Printf("Node list %v", r.PeerAddrList)

	for rid, reader := range r.PeerReaders {
		if int32(rid) == r.Id {
			continue
		}
		go r.replicaListener(rid, reader)
	}

}

func (r *Replica) ConnectToPeersNoListeners() {
	var b [4]byte
	bs := b[:4]
	done := make(chan bool)

	go r.waitForPeerConnections(done)

	//connect to peers
	for i := 0; i < int(r.Id); i++ {
		for done := false; !done; {
			if conn, err := net.Dial("tcp", r.PeerAddrList[i]); err == nil {
				r.Peers[i] = conn
				done = true
			} else {
				time.Sleep(1e9)
			}
		}
		binary.LittleEndian.PutUint32(bs, uint32(r.Id))
		if _, err := r.Peers[i].Write(bs); err != nil {
			fmt.Println("Write id error:", err)
			continue
		}
		r.Alive[i] = true
		r.PeerReaders[i] = bufio.NewReader(r.Peers[i])
		r.PeerWriters[i] = bufio.NewWriter(r.Peers[i])
	}
	<-done
	log.Printf("Replica id: %d. Done connecting to peers\n", r.Id)
}

/* Peer (replica) connections dispatcher */
func (r *Replica) waitForPeerConnections(done chan bool) {
	var b [4]byte
	bs := b[:4]

	r.Listener, _ = net.Listen("tcp", r.PeerAddrList[r.Id])
	for i := r.Id + 1; i < int32(r.N); i++ {
		conn, err := r.Listener.Accept()
		if err != nil {
			fmt.Println("Accept error:", err)
			continue
		}
		if _, err := io.ReadFull(conn, bs); err != nil {
			fmt.Println("Connection establish error:", err)
			continue
		}
		id := int32(binary.LittleEndian.Uint32(bs))
		r.Peers[id] = conn
		r.PeerReaders[id] = bufio.NewReader(conn)
		r.PeerWriters[id] = bufio.NewWriter(conn)
		r.Alive[id] = true

		log.Printf("IN Connected to %d", id)
	}

	done <- true
}

/* Client connections dispatcher */
func (r *Replica) WaitForClientConnections() {
	log.Println("Waiting for client connections")

	for !r.Shutdown {
		conn, err := r.Listener.Accept()
		if err != nil {
			log.Println("Accept error:", err)
			continue
		}
		go r.clientListener(conn)

	}
}

func (r *Replica) replicaListener(rid int, reader *bufio.Reader) {
	var msgType uint8
	var err error = nil
	var gbeacon genericsmrproto.Beacon
	var gbeaconReply genericsmrproto.BeaconReply

	for err == nil && !r.Shutdown {

		if msgType, err = reader.ReadByte(); err != nil {
			break
		}

		switch uint8(msgType) {

		case genericsmrproto.GENERIC_SMR_BEACON:
			if err = gbeacon.Unmarshal(reader); err != nil {
				break
			}
			beacon := &Beacon{int32(rid), gbeacon.Timestamp}
			r.ReplyBeacon(beacon)
			break

		case genericsmrproto.GENERIC_SMR_BEACON_REPLY:
			if err = gbeaconReply.Unmarshal(reader); err != nil {
				break
			}
			dlog.Println("receive beacon ", gbeaconReply.Timestamp, " reply from ",rid)
			//TODO: UPDATE STUFF
			r.Mutex.Lock()
			r.Latencies[rid] += time.Now().UnixNano() - gbeaconReply.Timestamp
			r.Mutex.Unlock()
			r.Ewma[rid] = 0.99*r.Ewma[rid] + 0.01*float64(time.Now().UnixNano()-gbeaconReply.Timestamp)
			break

		default:
			if rpair, present := r.rpcTable[msgType]; present {
				obj := rpair.Obj.New()
				if err = obj.Unmarshal(reader); err != nil {
					break
				}
				rpair.Chan <- obj
			} else {
				log.Fatal("Error: received unknown message type ", msgType," from  ", rid)
			}
		}
	}

	r.Mutex.Lock()
	r.Alive[rid] = false
	r.Mutex.Unlock()
}

func (r *Replica) clientListener(conn net.Conn) {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	var msgType byte //:= make([]byte, 1)
	var err error

	r.Mutex.Lock()
	log.Println("Client up ", conn.RemoteAddr(),"(",r.LRead,")")
	r.Mutex.Unlock()

	mutex := &sync.Mutex{}

	for !r.Shutdown && err == nil {

		if msgType, err = reader.ReadByte(); err != nil {
			break
		}

		switch uint8(msgType) {

		case genericsmrproto.PROPOSE:
			propose := new(genericsmrproto.Propose)
			if err = propose.Unmarshal(reader); err != nil {
				break
			}
			dlog.Println("Got proposal (key=",propose.Command.K.String(),"value=",propose.Command.V.String(),")")
			if r.LRead && (propose.Command.Op == state.GET || propose.Command.Op == state.SCAN) {
				val := propose.Command.Execute(r.State)
				propreply := &genericsmrproto.ProposeReplyTS{
					TRUE,
					propose.CommandId,
					val,
					propose.Timestamp}
				r.ReplyProposeTS(propreply, writer, mutex)
			}else{
				r.ProposeChan <- &Propose{propose, writer, mutex}
			}
			break

		case genericsmrproto.READ:
			read := new(genericsmrproto.Read)
			if err = read.Unmarshal(reader); err != nil {
				break
			}
			//r.ReadChan <- read
			break

		case genericsmrproto.PROPOSE_AND_READ:
			pr := new(genericsmrproto.ProposeAndRead)
			if err = pr.Unmarshal(reader); err != nil {
				break
			}
			//r.ProposeAndReadChan <- pr
			break

		case genericsmrproto.STATS:
			r.Mutex.Lock()
			b,_ := json.Marshal(r.Stats)
			r.Mutex.Unlock()
			writer.Write(b)
			writer.Flush()
		}
	}

	conn.Close()

	log.Println("Client down ", conn.RemoteAddr())

}

func (r *Replica) RegisterRPC(msgObj fastrpc.Serializable, notify chan fastrpc.Serializable) uint8 {
	code := r.rpcCode
	r.rpcCode++
	r.rpcTable[code] = &RPCPair{msgObj, notify}
	dlog.Println("registering RPC ",r.rpcCode)
	return code
}

func (r *Replica) SendMsg(peerId int32, code uint8, msg fastrpc.Serializable) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	w := r.PeerWriters[peerId]
	if w==nil{
		log.Printf("Connection to %d lost!\n", peerId)
		return
	}
	w.WriteByte(code)
	msg.Marshal(w)
	w.Flush()
}

func (r *Replica) SendMsgNoFlush(peerId int32, code uint8, msg fastrpc.Serializable) {
	w := r.PeerWriters[peerId]
	if w==nil{
		log.Printf("Connection to %d lost!\n", peerId)
		return
	}
	w.WriteByte(code)
	msg.Marshal(w)
}

func (r *Replica) ReplyProposeTS(reply *genericsmrproto.ProposeReplyTS, w *bufio.Writer, lock *sync.Mutex) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	reply.Marshal(w)
	w.Flush()
}

func (r *Replica) SendBeacon(peerId int32) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	w := r.PeerWriters[peerId]
	if w==nil{
		log.Printf("Connection to %d lost!\n", peerId)
		return
	}
	w.WriteByte(genericsmrproto.GENERIC_SMR_BEACON)
	beacon := &genericsmrproto.Beacon{Timestamp: time.Now().UnixNano()}
	beacon.Marshal(w)
	w.Flush()
	dlog.Println("send beacon ", beacon.Timestamp, " to ", peerId)
}

func (r *Replica) ReplyBeacon(beacon *Beacon) {
	dlog.Println("replying beacon to ",beacon.Rid)
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	w := r.PeerWriters[beacon.Rid]
	if w==nil{
		log.Printf("Connection to %d lost!\n", beacon.Rid)
		return
	}
	w.WriteByte(genericsmrproto.GENERIC_SMR_BEACON_REPLY)
	rb := &genericsmrproto.BeaconReply{beacon.Timestamp}
	rb.Marshal(w)
	w.Flush()
}

// updates the preferred order in which to communicate with peers according to a preferred quorum
func (r *Replica) UpdatePreferredPeerOrder(quorum []int32) {
	aux := make([]int32, r.N)
	i := 0
	for _, p := range quorum {
		if p == r.Id {
			continue
		}
		aux[i] = p
		i++
	}

	for _, p := range r.PreferredPeerOrder {
		found := false
		for j := 0; j < i; j++ {
			if aux[j] == p {
				found = true
				break
			}
		}
		if !found {
			aux[i] = p
			i++
		}
	}

	r.PreferredPeerOrder = aux
}

func (r *Replica) ComputeClosestPeers() {

	npings := 20

	for j := 0; j < npings; j++ {
		for i := int32(0); i < int32(r.N); i++ {
			if i == r.Id {
				continue
			}
			r.Mutex.Lock()
			if r.Alive[i] {
				r.Mutex.Unlock()
				r.SendBeacon(i)
			} else {
				r.Latencies[i] = math.MaxInt64
				r.Mutex.Unlock()
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	quorum := make([]int32, r.N)

	r.Mutex.Lock()
	for i := int32(0); i < int32(r.N); i++ {
		pos := 0
		for j := int32(0); j < int32(r.N); j++ {
			if (r.Latencies[j] < r.Latencies[i]) || ((r.Latencies[j] == r.Latencies[i]) && (j < i)) {
				pos++
			}
		}
		quorum[pos] = int32(i)
	}
	r.Mutex.Unlock()

	r.UpdatePreferredPeerOrder(quorum)

	for i := 0; i < r.N-1; i++ {
		node := r.PreferredPeerOrder[i]
		lat := float64(r.Latencies[node]) / float64(npings* 1000000)
		log.Println(node, " -> ", lat , "ms")
	}

}
