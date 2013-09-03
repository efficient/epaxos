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
	"sync"
	"time"
)

var masterAddr *string = flag.String("maddr", "", "Master address. Defaults to localhost")
var masterPort *int = flag.Int("mport", 7087, "Master port.  Defaults to 7077.")
var reqsNb *int = flag.Int("q", 5000, "Total number of requests. Defaults to 5000.")
var noLeader *bool = flag.Bool("e", false, "Egalitarian (no leader). Defaults to false.")
var rounds *int = flag.Int("r", 1, "Split the total number of requests into this many rounds, and do rounds sequentially. Defaults to 1.")
var procs *int = flag.Int("p", 2, "GOMAXPROCS. Defaults to 2")
var check = flag.Bool("check", false, "Check that every expected reply was received exactly once.")
var eps *int = flag.Int("eps", 0, "Send eps more messages per round than the client will wait for (to discount stragglers). Defaults to 0.")
var conflicts *int = flag.Int("c", -1, "Percentage of conflicts. Defaults to 0%")
var s = flag.Float64("s", 2, "Zipfian s parameter")
var v = flag.Float64("v", 1, "Zipfian v parameter")
var barOne = flag.Bool("barOne", false, "Sent commands to all replicas except the last one.")
var waitLess = flag.Bool("waitLess", false, "Wait for only N - 1 replicas to finish.")

var N int

var successful []int
var succ int
var succLock = new(sync.Mutex)

var rarray []int
var rsp []bool

func main() {
	flag.Parse()

	runtime.GOMAXPROCS(*procs)

	randObj := rand.New(rand.NewSource(42))
	zipf := rand.NewZipf(randObj, *s, *v, uint64(*reqsNb)) //uint64(*reqsNb / *rounds + *eps))

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

	N = len(rlReply.ReplicaList)
	servers := make([]net.Conn, N)
	readers := make([]*bufio.Reader, N)
	writers := make([]*bufio.Writer, N)

	rarray = make([]int, *reqsNb / *rounds + *eps)
	karray := make([]int64, *reqsNb / *rounds + *eps)
	perReplicaCount := make([]int, N)
	//test := make([]int, *reqsNb / *rounds + *eps)
	M := N
	if *barOne {
		M = N - 1
	}
	for i := 0; i < len(rarray); i++ {
		r := rand.Intn(M)

		rarray[i] = r
		if i < *reqsNb / *rounds {
			perReplicaCount[r]++
		}

		if *conflicts >= 0 {
			r = rand.Intn(100)
			if r < *conflicts {
				karray[i] = 42
			} else {
				karray[i] = int64(43 + i)
			}
		} else {
			karray[i] = int64(zipf.Uint64())
			//test[karray[i]]++
		}
	}
	if *conflicts >= 0 {
		//fmt.Println("Uniform distribution")
	} else {
		/*fmt.Println("Zipfian distribution:")
		  sum := 0
		  for _, val := range test[0:2000] {
		      sum += val
		  }
		  fmt.Println(test[0:100])
		  fmt.Println(sum)*/
	}

	for i := 0; i < N; i++ {
		var err error
		servers[i], err = net.Dial("tcp", rlReply.ReplicaList[i])
		if err != nil {
			log.Printf("Error connecting to replica %d\n", i)
			N = N - 1
		}
		readers[i] = bufio.NewReader(servers[i])
		writers[i] = bufio.NewWriter(servers[i])
	}

	successful = make([]int, N)
	leader := 0

	if *noLeader == false {
		reply := new(masterproto.GetLeaderReply)
		if err = master.Call("Master.GetLeader", new(masterproto.GetLeaderArgs), reply); err != nil {
			log.Fatalf("Error making the GetLeader RPC\n")
		}
		leader = reply.LeaderId
		//log.Printf("The leader is replica %d\n", leader)
	}

	var id int32 = 0
	done := make(chan bool, N)
	args := genericsmrproto.Propose{id, state.Command{state.PUT, 0, 0}} //make([]int64, state.VALUE_SIZE)}}

	pdone := make(chan bool)
	go printer(pdone)

	before_total := time.Now()

	for j := 0; j < *rounds; j++ {

		n := *reqsNb / *rounds

		if *check {
			rsp = make([]bool, n)
			for j := 0; j < n; j++ {
				rsp[j] = false
			}
		}

		if *noLeader {
			for i := 0; i < N; i++ {
				go waitReplies(readers, i, perReplicaCount[i], done)
			}
		} else {
			go waitReplies(readers, leader, n, done)
			//    go waitReplies(readers, 2, n, done)
		}

		//    before := time.Now()

		for i := 0; i < n+*eps; i++ {
			//dlog.Printf("Sending proposal %d\n", id)
			if *noLeader {
				leader = rarray[i]
				if leader >= N {
					continue
				}
			}
			args.ClientId = id
			args.Command.K = state.Key(karray[i])
			writers[leader].WriteByte(genericsmrproto.PROPOSE)
			args.Marshal(writers[leader])
			writers[leader].Flush()
			//fmt.Println("Sent", id)
			id++
			if i%100 == 0 {
				for i := 0; i < N; i++ {
					writers[i].Flush()
				}
			}
		}
		for i := 0; i < N; i++ {
			writers[i].Flush()
		}

		err := false
		if *noLeader {
			W := N
			if *waitLess {
				W = N - 1
			}
			for i := 0; i < W; i++ {
				e := <-done
				err = e || err
			}
		} else {
			err = <-done
		}

		// after := time.Now()

		//  fmt.Printf("Round took %v\n", after.Sub(before))

		if *check {
			for j := 0; j < n; j++ {
				if !rsp[j] {
					fmt.Println("Didn't receive", j)
				}
			}
		}

		if err {
			if *noLeader {
				N = N - 1
			} else {
				reply := new(masterproto.GetLeaderReply)
				master.Call("Master.GetLeader", new(masterproto.GetLeaderArgs), reply)
				leader = reply.LeaderId
				log.Printf("New leader is replica %d\n", leader)
			}
		}
	}

	after_total := time.Now()
	//fmt.Printf("Test took %v\n", after_total.Sub(before_total))
	//fmt.Printf("%v\n", (after_total.Sub(before_total)).Seconds())

	s := 0
	for _, succ := range successful {
		s += succ
	}

	fmt.Printf("Successful: %d\n", s)
	fmt.Printf("%v\n", float64(s)/(after_total.Sub(before_total)).Seconds())

	for _, client := range servers {
		if client != nil {
			client.Close()
		}
	}
	master.Close()
}

func waitReplies(readers []*bufio.Reader, leader int, n int, done chan bool) {
	e := false

	reply := new(genericsmrproto.ProposeReply)
	for i := 0; i < n; i++ {
		/*if *noLeader {
		    leader = rarray[i]
		}*/
		if err := reply.Unmarshal(readers[leader]); err != nil {
			fmt.Println("Error when reading:", err)
			e = true
			//continue
			break
		}
		if *check {
			if rsp[reply.Instance] {
				fmt.Println("Duplicate reply", reply.Instance)
			}
			rsp[reply.Instance] = true
		}
		if reply.OK != 0 {
			successful[leader]++
			succLock.Lock()
			succ++
			succLock.Unlock()
		}
	}
	done <- e
}

func printer(done chan bool) {
	//i := 0
	//var ts int
	var smooth [50]float64
	i := 0
	mt := 0.0
	for true {
		time.Sleep(10 * 1000 * 1000)
		var ls int
		succLock.Lock()
		ls = succ
		succ = 0
		succLock.Unlock()
		j := i % len(smooth)
		mt -= smooth[j]
		smooth[j] = float64(ls * 100)
		mt += smooth[j]
		i++
		if i >= len(smooth) {
			fmt.Println(mt / float64(len(smooth)))
		}
	}
}
