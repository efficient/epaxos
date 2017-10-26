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
)

const TRUE = uint8(1)

type Parameters struct {
	verbose        bool
	HasFailed      bool
	ClosestReplica int
	Leader         int
	IsLeaderless   bool
	IsFast         bool
	N              int
	ReplicaList    []string
	servers        []net.Conn
	readers        []*bufio.Reader
	writers        []*bufio.Writer
	id             int32
	done           chan state.Value
}

func NewParameters() *Parameters{ return &Parameters{ false, false,0,0, false,false,0,nil, nil,nil,nil,0, make(chan state.Value, 1)} }

func (b *Parameters) Connect(masterAddr string, masterPort int, verbose bool, leaderless bool, fast bool) {

	b.verbose = verbose
	b.IsLeaderless = leaderless
	b.IsFast = fast
	b.id = 0

	master, err := rpc.DialHTTP("tcp", fmt.Sprintf("%s:%d", masterAddr, masterPort))
	if err != nil {
		log.Fatalf("Error connecting to master\n")
	}

	var rlReply *masterproto.GetReplicaListReply
	for done := false; !done; {
		rlReply = new(masterproto.GetReplicaListReply)
		err = master.Call("Master.GetReplicaList", new(masterproto.GetReplicaListArgs), rlReply)
		if err != nil {
			log.Fatalf("Error making the GetReplicaList RPC")
		}
		if rlReply.Ready {
			done = true
		}
	}

	minLatency := math.MaxFloat64
	b.ReplicaList = rlReply.ReplicaList
	for i := 0; i < len(b.ReplicaList); i++ {
		addr := strings.Split(string(b.ReplicaList[i]), ":")[0]
		if addr == "" {
			addr = "127.0.0.1"
		}
		out, err := exec.Command("ping", addr, "-c 3", "-q").Output()
		if err == nil {
			latency, _ := strconv.ParseFloat(strings.Split(string(out), "/")[4], 64)
			log.Printf("%v -> %v", i, latency)
			if minLatency > latency {
				b.ClosestReplica = i
				minLatency = latency
			}
		} else {
			log.Fatal("cannot connect to " + b.ReplicaList[i])
		}
	}

	log.Printf("node list %v, closest = (%v,%vms)",b.ReplicaList, b.ClosestReplica, minLatency)

	b.N = len(b.ReplicaList)

	b.servers = make([]net.Conn, b.N)
	b.readers = make([]*bufio.Reader, b.N)
	b.writers = make([]*bufio.Writer, b.N)

	var toConnect []int
	toConnect=append(toConnect,b.ClosestReplica)

	if leaderless == false {
		reply := new(masterproto.GetLeaderReply)
		if err = master.Call("Master.GetLeader", new(masterproto.GetLeaderArgs), reply); err != nil {
			log.Fatalf("Error making the GetLeader RPC\n")
		}
		b.Leader = reply.LeaderId
		toConnect =append(toConnect,b.Leader)
		log.Printf("The Leader is replica %d\n", b.Leader)
	}

	for _,i := range toConnect {
		log.Println("Connection to ", i, " -> ",b.ReplicaList[i])
		b.servers[i], err = net.DialTimeout("tcp", b.ReplicaList[i], 10*time.Second)
		if err != nil {
			log.Fatal("Connection error with ", b.ReplicaList[i])
		} else {
			b.readers[i] = bufio.NewReader(b.servers[i])
			b.writers[i] = bufio.NewWriter(b.servers[i])
		}
	}

	log.Println("Connected")

}

func (b *Parameters) Disconnect(){
	for _,server := range b.servers{
		if server!=nil {
			server.Close()
		}
	}
	log.Printf("Disconnected")
}

func (b *Parameters) Write(key int64, value []byte) {
	b.id++
	args := genericsmrproto.Propose{b.id, state.Command{state.PUT, 0, state.NIL()}, 0}
	args.CommandId = b.id
	args.Command.K = state.Key(key)
	args.Command.V = value
	args.Command.Op = state.PUT

	if b.verbose{
		log.Println("PUT(",args.Command.K.String(),",",args.Command.V.String(),")")
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
		log.Println("GET(",args.Command.K.String(),")")
	}

	return b.execute(args)
}

func (b *Parameters) Scan(key int64) []byte{
	b.id++
	args := genericsmrproto.Propose{b.id, state.Command{state.PUT, 0, state.NIL()}, 0}
	args.CommandId = b.id
	args.Command.K = state.Key(key)
	args.Command.Op = state.SCAN

	if b.verbose{
		log.Println("SCAN(",args.Command.K.String(),")")
	}

	return b.execute(args)
}

func (b *Parameters) execute(args genericsmrproto.Propose) []byte{

	if b.IsFast {
		log.Fatal("NYI")
	}

	submitter := b.ClosestReplica
	if (!b.IsLeaderless && args.Command.Op == state.PUT )|| b.HasFailed {
		submitter = b.Leader
	}

	go b.waitReplies(submitter)

	if !b.IsFast {
		b.writers[submitter].WriteByte(genericsmrproto.PROPOSE)
		args.Marshal(b.writers[submitter])
		b.writers[submitter].Flush()
	} else {
		//send to everyone
		for rep := 0; rep < b.N; rep++ {
			b.writers[rep].WriteByte(genericsmrproto.PROPOSE)
			args.Marshal(b.writers[rep])
			b.writers[rep].Flush()
		}
	}

	if b.verbose{
		log.Println("Sent to ",submitter)
	}

	value := <-b.done

	if b.verbose{
		log.Println("Returning: ",value)
	}

	return value
}

func (b *Parameters) waitReplies(submitter int) {
	e := state.NIL()
	var err error

	// FIXME handle b.Fast properly
	rep := new(genericsmrproto.ProposeReplyTS)
	if err = rep.Unmarshal(b.readers[submitter]); err != nil {
		log.Println("Error when reading:", err)
		log.Println("Reconncting ...")
		b.servers[submitter].Close()
		b.servers[submitter], err = net.DialTimeout("tcp", b.ReplicaList[submitter], 10*time.Second)
		if err != nil {
			if !b.HasFailed {
				b.HasFailed = true
			} else {
				log.Fatal("cannot recover")
			}
		}else {
			b.readers[submitter] = bufio.NewReader(b.servers[submitter])
			b.writers[submitter] = bufio.NewWriter(b.servers[submitter])
		}
	} else {
		if rep.OK == TRUE {
			e = rep.Value
		} else {
			log.Println("Failed to receive a response")
			if !b.HasFailed {
				b.HasFailed = true
			} else {
				log.Fatal("cannot recover")
			}
		}
	}

	b.done <- e
}