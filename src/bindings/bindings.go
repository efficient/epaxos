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
)

const TRUE = uint8(1)

type Parameters struct {
	Leader       int
	IsLeaderless bool
	IsFast       bool
	N            int
	servers      []net.Conn
	readers      []*bufio.Reader
	writers      []*bufio.Writer
	id           int32
}

func NewParameters() *Parameters{ return &Parameters{ 0,false,false,0,nil,nil,nil,0 } }

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

	closest := 0
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
				closest = i
				minLatency = latency
			}
		} else {
			log.Fatal("cannot connect to " + rlReply.ReplicaList[i])
		}
	}

	b.N = len(rlReply.ReplicaList)

	b.servers = make([]net.Conn, b.N)
	b.readers = make([]*bufio.Reader, b.N)
	b.writers = make([]*bufio.Writer, b.N)

	for i := 0; i < b.N; i++ {
		var err error
		b.servers[i], err = net.Dial("tcp", rlReply.ReplicaList[i])
		if err != nil {
			log.Printf("Error connecting to replica %d\n", i)
		}
		b.readers[i] = bufio.NewReader(b.servers[i])
		b.writers[i] = bufio.NewWriter(b.servers[i])
	}
	log.Println("Connected")

	if leaderless == false {
		reply := new(masterproto.GetLeaderReply)
		if err = master.Call("Master.GetLeader", new(masterproto.GetLeaderArgs), reply); err != nil {
			log.Fatalf("Error making the GetLeader RPC\n")
		}
		b.Leader = reply.LeaderId
	} else {
		b.Leader = closest
	}
	log.Printf("The Leader is replica %d\n", b.Leader)
}

func (b *Parameters) Write(key int64, value string) {
	b.id++;
	args := genericsmrproto.Propose{b.id, state.Command{state.PUT, 0, state.NIL()}, 0}
	args.CommandId = b.id
	args.Command.K = state.Key(key)
	args.Command.V = []byte(value)
	args.Command.Op = state.PUT
	b.execute(args)
}

func (b *Parameters) Read(key int64) string {
	b.id++;
	args := genericsmrproto.Propose{b.id, state.Command{state.PUT, 0, state.NIL()}, 0}
	args.CommandId = b.id
	args.Command.K = state.Key(key)
	args.Command.Op = state.GET
	return b.execute(args)
}


// Helpers

func (b *Parameters) execute(args genericsmrproto.Propose) string {

	done := make(chan *state.Value, b.N)

	if b.IsFast {
		log.Fatal("NYIT")
	}

	go waitReplies(b.readers, b.Leader, 1, done)

	b.writers[b.Leader].WriteByte(genericsmrproto.PROPOSE)
	args.Marshal(b.writers[b.Leader])
	b.writers[b.Leader].Flush()
	err := false
	value := <-done
	err = value == nil || err

	return value.String()
}

func waitReplies(readers []*bufio.Reader, nnode int, n int, done chan *state.Value) {
	var e *state.Value
	e = nil
	reply := new(genericsmrproto.ProposeReplyTS)
	for i := 0; i < n; i++ {
		if err := reply.Unmarshal(readers[nnode]); err != nil {
			fmt.Println("Error when reading:", err)
			continue
		}
		if reply.OK == TRUE {
			e = &reply.Value
		} else {
			log.Fatal("Failed to receive response value")
		}
	}
	done <- e
}
