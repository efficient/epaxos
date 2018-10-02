package bindings

import (
	"net"
	"bufio"
	"state"
	"genericsmrproto"
	"fmt"
	"log"
	"net/rpc"
	"masterproto"
	"math"
	"strings"
	"os/exec"
	"strconv"
	"time"
	"errors"
	"bytes"
	"encoding/binary"
)

const TRUE = uint8(1)

type Parameters struct {
	masterAddr     string
	masterPort     int
	verbose        bool
	localReads     bool
	closestReplica int
	Leader         int
	leaderless     bool
	isFast         bool
	n              int
	replicaLists   []string
	servers        []net.Conn
	readers        []*bufio.Reader
	writers        []*bufio.Writer
	id             int32
	retries        int32
}

func NewParameters(masterAddr string, masterPort int, verbose bool, leaderless bool, fast bool, localReads bool) *Parameters{
	return &Parameters{
		masterAddr,
		masterPort,
		verbose,
		localReads,
		0,
		0,
		leaderless,
		fast,
		0,
		nil,
		nil,
		nil,
		nil,
		0,
		5}}

func (b *Parameters) Connect() error {

	master, err := rpc.DialHTTP("tcp", fmt.Sprintf("%s:%d", b.masterAddr, b.masterPort))
	if err != nil {
		log.Printf("Error connecting to master\n")
		return err
	}

	var rlReply *masterproto.GetReplicaListReply
	for done := false; !done; {
		rlReply = new(masterproto.GetReplicaListReply)
		err = master.Call("Master.GetReplicaList", new(masterproto.GetReplicaListArgs), rlReply)
		if err != nil {
			log.Printf("Error making the GetReplicaList RPC")
			return err
		}
		if rlReply.Ready {
			done = true
		}
	}

	minLatency := math.MaxFloat64
	b.replicaLists = rlReply.ReplicaList
	for i := 0; i < len(b.replicaLists); i++ {
		if !rlReply.AliveList[i]{
			continue
		}
		addr := strings.Split(string(b.replicaLists[i]), ":")[0]
		if addr == "" {
			addr = "127.0.0.1"
		}
		out, err := exec.Command("ping", addr, "-c 3", "-q").Output()
		if err == nil {
			latency, _ := strconv.ParseFloat(strings.Split(string(out), "/")[4], 64)
			log.Printf("%v -> %v", i, latency)
			if minLatency > latency {
				b.closestReplica = i
				minLatency = latency
			}
		} else {
			log.Printf("cannot connect to " + b.replicaLists[i])
		}
	}

	log.Printf("node list %v, closest (alive) = (%v,%vms)",b.replicaLists, b.closestReplica, minLatency)

	b.n = len(b.replicaLists)

	b.servers = make([]net.Conn, b.n)
	b.readers = make([]*bufio.Reader, b.n)
	b.writers = make([]*bufio.Writer, b.n)

	var toConnect []int
	toConnect=append(toConnect,b.closestReplica)

	if b.leaderless == false {
		reply := new(masterproto.GetLeaderReply)
		if err = master.Call("Master.GetLeader", new(masterproto.GetLeaderArgs), reply); err != nil {
			log.Fatalf("Error making the GetLeader RPC\n")
		}
		b.Leader = reply.LeaderId
		if  b.closestReplica != b.Leader{
			toConnect =append(toConnect,b.Leader)
		}
		log.Printf("The Leader is replica %d\n", b.Leader)
	}

	for _,i := range toConnect {
		log.Println("Connection to ", i, " -> ",b.replicaLists[i])
		b.servers[i], err = net.DialTimeout("tcp", b.replicaLists[i], 10*time.Second)
		if err != nil {
			log.Println("Connection error with ", b.replicaLists[i])
			return err
		} else {
			b.readers[i] = bufio.NewReader(b.servers[i])
			b.writers[i] = bufio.NewWriter(b.servers[i])
		}
	}

	log.Println("Connected")

	return nil

}

func (b *Parameters) Disconnect(){
	for _,server := range b.servers{
		if server!=nil {
			server.Close()
		}
	}
	log.Printf("Disconnected")
}

// not idempotent in case of a failure
func (b *Parameters) Write(key int64, value []byte) {
	b.id++
	args := genericsmrproto.Propose{b.id, state.Command{state.PUT, 0, state.NIL()}, 0}
	args.CommandId = b.id
	args.Command.K = state.Key(key)
	args.Command.V = value
	args.Command.Op = state.PUT

	if b.verbose{
		log.Println(args.Command.String())
	}

	b.execute(args)
}

func (b *Parameters) Read(key int64) []byte{
	b.id++
	args := genericsmrproto.Propose{b.id, state.Command{state.PUT, 0, state.NIL()}, 0}
	args.CommandId = b.id
	args.Command.K = state.Key(key)
	args.Command.Op = state.GET

	if b.verbose{
		log.Println(args.Command.String())
	}

	return b.execute(args)
}

func (b *Parameters) Scan(key int64, count int64) []byte{
	b.id++
	args := genericsmrproto.Propose{b.id, state.Command{state.PUT, 0, state.NIL()}, 0}
	args.CommandId = b.id
	args.Command.K = state.Key(key)
	args.Command.V = make([]byte, 8)
	binary.LittleEndian.PutUint64(args.Command.V, uint64(count))
	args.Command.Op = state.SCAN

	if b.verbose{
		log.Println(args.Command.String())
	}

	return b.execute(args)
}

func (b *Parameters) Stats() (string){
	b.writers[b.closestReplica].WriteByte(genericsmrproto.STATS)
	b.writers[b.closestReplica].Flush()
	arr := make([]byte,1000)
	b.readers[b.closestReplica].Read(arr)
	return string(bytes.Trim(arr, "\x00"))
}

// internals

func (b *Parameters) execute(args genericsmrproto.Propose) []byte{

	if b.isFast {
		log.Fatal("NYI")
	}

	err:=errors.New("")
	value:=state.NIL()

	for err!=nil {

		submitter := b.Leader
		if b.leaderless || ((args.Command.Op == state.GET || args.Command.Op == state.SCAN) && b.localReads) {
			submitter = b.closestReplica
		}

		if !b.isFast {
			b.writers[submitter].WriteByte(genericsmrproto.PROPOSE)
			args.Marshal(b.writers[submitter])
			b.writers[submitter].Flush()
		} else {
			//send to everyone
			for rep := 0; rep < b.n; rep++ {
				b.writers[rep].WriteByte(genericsmrproto.PROPOSE)
				args.Marshal(b.writers[rep])
				b.writers[rep].Flush()
			}
		}

		if b.verbose {
			log.Println("Sent to ", submitter)
		}

		value, err = b.waitReplies(submitter)

		if err!=nil{

			log.Println("Error: ", err)

			for err != nil && b.retries > 0 {
				b.retries--
				b.Disconnect()
				log.Println("Reconnecting ...")
				time.Sleep(10 * time.Second) // must be inline with the closest quorum re-computation
				err = b.Connect()
			}

			if err!=nil && b.retries == 0{
				log.Fatal("Cannot recover.")
			}

		}

	}

	if b.verbose{
		log.Println("Returning: ",value.String())
	}

	return value
}

func (b *Parameters) waitReplies(submitter int) (state.Value,error) {
	var err error
	ret := state.NIL()

	rep := new(genericsmrproto.ProposeReplyTS)
	if err = rep.Unmarshal(b.readers[submitter]); err == nil {
		if rep.OK == TRUE {
			ret = rep.Value
		} else {
			err = errors.New("Failed to receive a response.")
		}
	}

	return ret,err
}
