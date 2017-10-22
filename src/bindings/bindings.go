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
	"dlog"
)

const TRUE = uint8(1)

type Parameters struct {
	HasFailed bool
	ClosestReplica int
	Leader         int
	IsLeaderless   bool
	IsFast         bool
	N              int
	servers        []net.Conn
	readers        []*bufio.Reader
	writers        []*bufio.Writer
	id             int32
	done           chan state.Value
}

func NewParameters() *Parameters{ return &Parameters{ false,0,0, false,false,0,nil,nil,nil,0, make(chan state.Value, 1)} }

func (b *Parameters) Connect(masterAddr string, masterPort int, leaderless bool, fast bool) {

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
	for i := 0; i < len(rlReply.ReplicaList); i++ {
		addr := strings.Split(string(rlReply.ReplicaList[i]), ":")[0]
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
			log.Fatal("cannot connect to " + rlReply.ReplicaList[i])
		}
	}

	log.Printf("node list %v, closest = (%v,%vms)",rlReply.ReplicaList, b.ClosestReplica, minLatency)

	b.N = len(rlReply.ReplicaList)

	b.servers = make([]net.Conn, b.N)
	b.readers = make([]*bufio.Reader, b.N)
	b.writers = make([]*bufio.Writer, b.N)

	for i := 0; i < b.N; i++ {
		var err error
		b.servers[i], err = net.DialTimeout("tcp", rlReply.ReplicaList[i], 10*time.Second)
		if err != nil {
			log.Fatal("Connection error with ",rlReply.ReplicaList[i])
		}else {
			b.readers[i] = bufio.NewReader(b.servers[i])
			b.writers[i] = bufio.NewWriter(b.servers[i])
		}
	}

	if leaderless == false {
		reply := new(masterproto.GetLeaderReply)
		if err = master.Call("Master.GetLeader", new(masterproto.GetLeaderArgs), reply); err != nil {
			log.Fatalf("Error making the GetLeader RPC\n")
		}
		b.Leader = reply.LeaderId
		log.Printf("The Leader is replica %d\n", b.Leader)
	}
	log.Printf("Connected")

}

func (b *Parameters) Disconnect(){
	for _,server := range b.servers{
		server.Close()
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
	b.execute(args)
}

func (b *Parameters) Read(key int64) []byte{
	b.id++
	args := genericsmrproto.Propose{b.id, state.Command{state.PUT, 0, state.NIL()}, 0}
	args.CommandId = b.id
	args.Command.K = state.Key(key)
	args.Command.Op = state.GET
	return b.execute(args)
}

func (b *Parameters) Scan(key int64) []byte{
	b.id++
	args := genericsmrproto.Propose{b.id, state.Command{state.PUT, 0, state.NIL()}, 0}
	args.CommandId = b.id
	args.Command.K = state.Key(key)
	args.Command.Op = state.SCAN
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

	dlog.Println("Sent to ",submitter)

	value := <-b.done

	return value
}

func (b *Parameters) waitReplies(submitter int) {
	var e state.Value
	var err error

	timeoutC := make(chan bool, 1)
	replyC := make(chan genericsmrproto.ProposeReplyTS, 1)

	go func() {
		time.Sleep(1 * time.Second)
		timeoutC <- true
	}()

	go func() {
		// FIXME handle b.Fast properly
		rep := new(genericsmrproto.ProposeReplyTS)
		if err = rep.Unmarshal(b.readers[submitter]); err != nil {
			log.Println("Error when reading:", err)
		}
		replyC <- *rep
	}()

	select {
	case reply := <-replyC:
		if reply.OK == TRUE {
			e = reply.Value
		}else{
			e = state.NIL()
			log.Println("Failed to receive a response")
			if !b.HasFailed {
				b.HasFailed = true
			} else {
				log.Fatal("cannot recover")
			}
		}
	case <-timeoutC:
		e = state.NIL()
		log.Println("Timeout!")
	}

	b.done <- e
}