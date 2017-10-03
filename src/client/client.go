package main

import (
	"bufio"
	"dlog"
	"flag"
	"fmt"
	"genericsmrproto"
	"log"
	"masterproto"
	"math/rand"
	"net"
	"net/rpc"
	"runtime"
	"state"
	"time"
	"os/exec"
	"strings"
	"math"
	"strconv"
)

import "github.com/go-redis/redis"

var name string = *flag.String("name", "", "Name of this client. Defaults to closest replica IP + random int")
var masterAddr *string = flag.String("maddr", "", "Master address. Defaults to localhost")
var masterPort *int = flag.Int("mport", 7087, "Master port. ")
var reqsNb *int = flag.Int("q", 1000, "Total number of requests. ")
var writes *int = flag.Int("w", 100, "Percentage of updates (writes). ")
var noLeader *bool = flag.Bool("e", false, "Egalitarian (no leader). ")
var fast *bool = flag.Bool("f", false, "Fast Paxos: send message directly to all replicas. ")
var procs *int = flag.Int("p", 2, "GOMAXPROCS. ")
var check = flag.Bool("check", false, "Check that every expected reply was received exactly once.")
var conflicts *int = flag.Int("c", -1, "Percentage of conflicts. Defaults to 0%")
var s = flag.Float64("s", 2, "Zipfian s parameter")
var v = flag.Float64("v", 1, "Zipfian v parameter")
var redisAddr *string = flag.String("raddr", "", "Redis address. Disabled per default.")
var redisPort *int = flag.Int("rport", 6379, "Redis port.")

var N int

var successful []int

var rarray []int
var rsp []bool

var redisServer *redis.Client
var tarray []int64

func main() {
	flag.Parse()

	runtime.GOMAXPROCS(*procs)

	randObj := rand.New(rand.NewSource(42))
	zipf := rand.NewZipf(randObj, *s, *v, uint64(*reqsNb))

	if *conflicts > 100 {
		log.Fatalf("Conflicts percentage must be between 0 and 100.\n")
	}

	master, err := rpc.DialHTTP("tcp", fmt.Sprintf("%s:%d", *masterAddr, *masterPort))
	if err != nil {
		log.Fatalf("Error connecting to master\n")
	}

	if *redisAddr != "" {
		redisServer = redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", *redisAddr, *redisPort),
			Password: "", // no password set
			DB:       0,  // use default DB
		})
		if redisServer.Ping().Err() != nil {
			log.Fatalf("Error connecting to Redis (%v)\n %v\n",
				fmt.Sprintf("%s:%d", *redisAddr, *redisPort),
				redisServer.Ping().Err())
		}
	}

	rlReply := new(masterproto.GetReplicaListReply)
	err = master.Call("Master.GetReplicaList", new(masterproto.GetReplicaListArgs), rlReply)
	if err != nil {
		log.Fatalf("Error making the GetReplicaList RPC")
	}

	N = len(rlReply.ReplicaList)
	minLeader := 0
	minLatency := math.MaxFloat64
	for i := 0; i < N; i++ {
		addr := strings.Split(string(rlReply.ReplicaList[i]), ":")[0]
		if addr == "" {
			addr = "127.0.0.1"
		}
		out, err := exec.Command("ping", addr, "-c 3", "-q").Output()
		if err == nil {
			latency, _ := strconv.ParseFloat(strings.Split(string(out), "/")[4], 64)
			log.Printf("%v -> %v", i, latency)
			if minLatency > latency {
				minLeader = i
				minLatency = latency
			}
		}
	}

	log.Printf("node list %v, closest = (%v,%vms)",rlReply.ReplicaList,minLeader,minLatency)
	if name == "" {
		rand.Seed(time.Now().Unix())
		name = "client-"+strconv.FormatUint(rand.Uint64(),16)+"-"+rlReply.ReplicaList[minLeader]
	}
	log.Printf(name)

	servers := make([]net.Conn, N)
	readers := make([]*bufio.Reader, N)
	writers := make([]*bufio.Writer, N)

	rarray = make([]int, *reqsNb)
	tarray = make([]int64, *reqsNb)
	karray := make([]int64, *reqsNb)
	put := make([]bool, *reqsNb)
	test := make([]int, *reqsNb)

	for i := 0; i < len(rarray); i++ {
		rarray[i] = minLeader
		if *conflicts >= 0 {
			r = rand.Intn(100)
			if r < *conflicts {
				karray[i] = 42
			} else {
				karray[i] = int64(43 + i)
			}
			r = rand.Intn(100)
			if r < *writes {
				put[i] = true
			} else {
				put[i] = false
			}
		} else {
			karray[i] = int64(zipf.Uint64())
			test[karray[i]]++
		}
	}
	if *conflicts >= 0 {
		log.Println("Uniform distribution")
	} else {
		log.Println("Zipfian distribution:")
		//fmt.Println(test[0:100])
	}

	for i := 0; i < N; i++ {
		var err error
		servers[i], err = net.Dial("tcp", rlReply.ReplicaList[i])
		if err != nil {
			log.Printf("Error connecting to replica %d\n", i)
		}
		readers[i] = bufio.NewReader(servers[i])
		writers[i] = bufio.NewWriter(servers[i])
	}
	log.Println("Connected")

	successful = make([]int, N)
	leader := 0

	if *noLeader == false {
		reply := new(masterproto.GetLeaderReply)
		if err = master.Call("Master.GetLeader", new(masterproto.GetLeaderArgs), reply); err != nil {
			log.Fatalf("Error making the GetLeader RPC\n")
		}
		leader = reply.LeaderId
		log.Printf("The leader is replica %d\n", leader)
	}

	var id int32 = 0
	done := make(chan bool, N)
	args := genericsmrproto.Propose{id, state.Command{state.PUT, 0, 0}, 0}

	before_total := time.Now()

	for j := 0; j < *reqsNb; j++ {

		if *check {
			rsp = make([]bool, j)
			for j := 0; j < j; j++ {
				rsp[j] = false
			}
		}

		if *noLeader {
			leader = rarray[j]
		}

		go waitReplies(readers, leader, 1, done)

		before := time.Now()

		dlog.Printf("Sending proposal %d\n", id)
		args.CommandId = id
		if put[j] {
			args.Command.Op = state.PUT
		} else {
			args.Command.Op = state.GET
		}
		args.Command.K = state.Key(karray[j])
		args.Command.V = state.Value(j)
			//args.Timestamp = time.Now().UnixNano()
		if !*fast {
			writers[leader].WriteByte(genericsmrproto.PROPOSE)
			args.Marshal(writers[leader])
		} else {
			//send to everyone
			for rep := 0; rep < N; rep++ {
				writers[rep].WriteByte(genericsmrproto.PROPOSE)
				args.Marshal(writers[rep])
				writers[rep].Flush()
			}
		}

		id++
		for i := 0; i < N; i++ {
			writers[i].Flush()
		}

		err := false
		e := <-done
		err = e || err

		after := time.Now()

		fmt.Printf("Delivery took %v\n", after.Sub(before))
		tarray[j] = after.Sub(before).Nanoseconds()

		if *check {
			if !rsp[j] {
				fmt.Println("Didn't receive", j)
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
	fmt.Printf("Test took %v\n", after_total.Sub(before_total))

	s := 0
	for _, succ := range successful {
		s += succ
	}

	fmt.Printf("Successful: %d\n", s)

	if redisServer!=nil{
		for j := 0; j < *reqsNb; j++ {
			key := name+"-"
			if put[j] {
				key += "write"
			}else {
				key += "read"
			}
			cmd := redisServer.LPush(key,tarray[j])
			if cmd.Err()!=nil{
				log.Fatal("Error connecting to Redis.")
			}
		}
	}


	for _, client := range servers {
		if client != nil {
			client.Close()
		}
	}
	if redisServer!=nil{
		redisServer.Close()
	}
	master.Close()
}

func waitReplies(readers []*bufio.Reader, leader int, n int, done chan bool) {
	e := false

	reply := new(genericsmrproto.ProposeReplyTS)
	for i := 0; i < n; i++ {
		if err := reply.Unmarshal(readers[leader]); err != nil {
			fmt.Println("Error when reading:", err)
			e = true
			continue
		}
		//fmt.Println(reply.Value)
		if *check {
			if rsp[reply.CommandId] {
				fmt.Println("Duplicate reply", reply.CommandId)
			}
			rsp[reply.CommandId] = true
		}
		if reply.OK != 0 {
			successful[leader]++
		}
	}
	done <- e
}
