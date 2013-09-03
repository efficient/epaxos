package main

import (
	"log"
	//	"dlog"
	"bufio"
	"flag"
	"fmt"
	"genericsmrproto"
	"masterproto"
	"math/rand"
	"net"
	"net/rpc"
	"runtime"
	"state"
	"time"
)

var masterAddr *string = flag.String("maddr", "", "Master address. Defaults to localhost")
var masterPort *int = flag.Int("mport", 7087, "Master port.  Defaults to 7077.")
var reqsNb *int = flag.Int("q", 5000, "Total number of requests. Defaults to 5000.")
var noLeader *bool = flag.Bool("e", false, "Egalitarian (no leader). Defaults to false.")
var procs *int = flag.Int("p", 2, "GOMAXPROCS. Defaults to 2")
var check = flag.Bool("check", false, "Check that every expected reply was received exactly once.")
var conflicts *int = flag.Int("c", -1, "Percentage of conflicts. Defaults to 0%")
var s = flag.Float64("s", 2, "Zipfian s parameter")
var v = flag.Float64("v", 1, "Zipfian v parameter")
var barOne = flag.Bool("barOne", false, "Sent commands to all replicas except the last one.")
var forceLeader = flag.Int("l", -1, "Force client to talk to a certain replica.")
var startRange = flag.Int("sr", 0, "Key range start")
var sleep = flag.Int("sleep", 0, "Sleep")
var T = flag.Int("T", 1, "Number of threads (simulated clients).")
var fast *bool = flag.Bool("f", false, "Fast Paxos: send message directly to all replicas. Defaults to false.")
var idStart = flag.Int("ids", 0, "Command ID range start.")

var rarray []int
var rsp []bool

func main() {
	flag.Parse()

	runtime.GOMAXPROCS(*procs)

	if *conflicts > 100 {
		log.Fatalf("Conflicts percentage must be between 0 and 100.\n")
	}

	master, err := rpc.DialHTTP("tcp", fmt.Sprintf("%s:%d", *masterAddr, *masterPort))
	if err != nil {
		log.Fatalf("Error connecting to master\n")
	}

	rlReply := new(masterproto.GetReplicaListReply)
	err = master.Call("Master.GetReplicaList", new(masterproto.GetReplicaListArgs), rlReply)
	if err != nil {
		log.Fatalf("Error making the GetReplicaList RPC")
	}

	leader := 0
	if *noLeader == false && *forceLeader < 0 {
		reply := new(masterproto.GetLeaderReply)
		if err = master.Call("Master.GetLeader", new(masterproto.GetLeaderArgs), reply); err != nil {
			log.Fatalf("Error making the GetLeader RPC\n")
		}
		leader = reply.LeaderId
		//log.Printf("The leader is replica %d\n", leader)
	} else if *forceLeader > 0 {
		leader = *forceLeader
	}

	done := make(chan bool, *T)

	readings := make(chan float64, 2**reqsNb)

	go printer(readings, done)

	for i := 0; i < *T; i++ {
		go simulatedClient(rlReply, leader, readings, done)
	}

	for i := 0; i < *T+1; i++ {
		<-done
	}
	master.Close()
}

func simulatedClient(rlReply *masterproto.GetReplicaListReply, leader int, readings chan float64, done chan bool) {
	N := len(rlReply.ReplicaList)
	servers := make([]net.Conn, N)
	readers := make([]*bufio.Reader, N)
	writers := make([]*bufio.Writer, N)

	rarray := make([]int, *reqsNb)
	karray := make([]int64, *reqsNb)
	perReplicaCount := make([]int, N)
	M := N
	if *barOne {
		M = N - 1
	}
	randObj := rand.New(rand.NewSource(42))
	zipf := rand.NewZipf(randObj, *s, *v, uint64(*reqsNb))
	for i := 0; i < len(rarray); i++ {
		r := rand.Intn(M)

		rarray[i] = r
		perReplicaCount[r]++

		if *conflicts >= 0 {
			r = rand.Intn(100)
			if r < *conflicts {
				karray[i] = 42
			} else {
				karray[i] = int64(*startRange + 43 + i)
			}
		} else {
			karray[i] = int64(zipf.Uint64())
		}
	}

	repliesChan := make(chan int32, *reqsNb*N)

	for i := 0; i < N; i++ {
		var err error
		servers[i], err = net.Dial("tcp", rlReply.ReplicaList[i])
		if err != nil {
			log.Printf("Error connecting to replica %d\n", i)
		}
		readers[i] = bufio.NewReader(servers[i])
		if *fast {
			//wait for replies from every replica
			go waitForReplies(readers[i], repliesChan)
		}
		writers[i] = bufio.NewWriter(servers[i])
	}

	id := int32(*idStart)
	args := genericsmrproto.Propose{id, state.Command{state.PUT, 0, 0}}

	n := *reqsNb

	for i := 0; i < n; i++ {

		before := time.Now()

		args.ClientId = id
		args.Command.K = state.Key(karray[i])

		if !*fast {
			if *noLeader {
				leader = rarray[i]
			}
			writers[leader].WriteByte(genericsmrproto.PROPOSE)
			args.Marshal(writers[leader])
			writers[leader].Flush()
		} else {
			//send to everyone
			for rep := 0; rep < N; rep++ {
				writers[rep].WriteByte(genericsmrproto.PROPOSE)
				args.Marshal(writers[rep])
				writers[rep].Flush()
			}
		}

		for true {
			rid := <-repliesChan
			if rid == id {
				break
			}
		}

		after := time.Now()

		id++

		readings <- (after.Sub(before)).Seconds() * 1000

		if *sleep > 0 {
			time.Sleep(100 * 1000 * 1000)
		}
	}

	for _, client := range servers {
		if client != nil {
			client.Close()
		}
	}
	done <- true
}

func waitForReplies(reader *bufio.Reader, repliesChan chan int32) {
	var reply genericsmrproto.ProposeReply
	for true {
		if err := reply.Unmarshal(reader); err != nil || reply.OK == 0 {
			return
			//fmt.Println("Error when reading from replica:", err)
			//continue
		}
		repliesChan <- reply.Instance
	}
}

func printer(readings chan float64, done chan bool) {
	n := *T * *reqsNb
	for i := 0; i < n; i++ {
		lat := <-readings
		fmt.Printf("%v\n", lat)
	}
	done <- true
}
