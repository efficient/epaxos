package main

import (
	"flag"
	"fmt"
	"genericsmrproto"
	"log"
	"masterproto"
	"net"
	"net/http"
	"net/rpc"
	"sync"
	"time"
	"math"
	"strconv"
	"strings"
	"os/exec"
)

var portnum *int = flag.Int("port", 7087, "Port # to listen on. Defaults to 7087")
var numNodes *int = flag.Int("N", 3, "Number of replicas. Defaults to 3.")

type Master struct {
	N        int
	nodeList []string
	addrList []string
	portList []int
	lock     *sync.Mutex
	nodes    []*rpc.Client
	leader   []bool
	alive    []bool
	latencies [] float64
}

func main() {
	flag.Parse()

	log.Printf("Master starting on port %d\n", *portnum)
	log.Printf("...waiting for %d replicas\n", *numNodes)

	master := &Master{*numNodes,
		make([]string, 0, *numNodes),
		make([]string, 0, *numNodes),
		make([]int, 0, *numNodes),
		new(sync.Mutex),
		make([]*rpc.Client, *numNodes),
		make([]bool, *numNodes),
		make([]bool, *numNodes),
		make([]float64, *numNodes)}

	rpc.Register(master)
	rpc.HandleHTTP()
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", *portnum))
	if err != nil {
		log.Fatal("Master listen error:", err)
	}

	go master.run()

	http.Serve(l, nil)
}

func (master *Master) run() {
	for true {
		master.lock.Lock()
		if len(master.nodeList) == master.N {
			master.lock.Unlock()
			break
		}
		master.lock.Unlock()
		time.Sleep(100000000)
	}
	time.Sleep(2000000000) // wait servers are up and rolling

	// connect to SMR servers
	for i := 0; i < master.N; {
		var err error
		addr := fmt.Sprintf("%s:%d", master.addrList[i], master.portList[i]+1000)
		master.nodes[i], err = rpc.DialHTTP("tcp", addr)
		if err != nil {
			log.Printf("Error connecting to replica %d (%v), retrying .. \n", i,addr)
			time.Sleep(1000000000) // retry
		} else {
			i++
		}
	}

	for true {
		time.Sleep(1000 * 1000 * 1000)
		new_leader := false
		for i, node := range master.nodes {
			err := node.Call("Replica.Ping", new(genericsmrproto.PingArgs), new(genericsmrproto.PingReply))
			master.lock.Lock()
			if err != nil {
				// log.Printf("Replica %d has failed to reply\n", i)
				master.alive[i] = false
				if master.leader[i] {
					// need to choose a new leader
					new_leader = true
					master.leader[i] = false
				}
			} else {
				master.alive[i] = true
			}
			master.lock.Unlock()
		}
		if !new_leader {
			continue
		}
		for i, new_master := range master.nodes {
			if master.alive[i] {
				err := new_master.Call("Replica.BeTheLeader", new(genericsmrproto.BeTheLeaderArgs), new(genericsmrproto.BeTheLeaderReply))
				if err == nil {
					master.lock.Lock()
					master.leader[i] = true
					master.lock.Unlock()
					log.Printf("Replica %d is the new leader.", i)
					break
				}
			}
		}

	}
}

func (master *Master) Register(args *masterproto.RegisterArgs, reply *masterproto.RegisterReply) error {

	master.lock.Lock()
	defer master.lock.Unlock()

	nlen := len(master.nodeList)
	index := nlen

	addrPort := fmt.Sprintf("%s:%d", args.Addr, args.Port)

	for i, ap := range master.nodeList {
		if addrPort == ap {
			index = i
			break
		}
	}

	if index == nlen {
		master.nodeList = master.nodeList[0 : nlen+1]
		master.nodeList[nlen] = addrPort
		master.addrList = master.addrList[0 : nlen+1]
		master.addrList[nlen] = args.Addr
		master.portList = master.portList[0 : nlen+1]
		master.portList[nlen] = args.Port
		master.leader[index] = false
		nlen++

		addr := args.Addr
		if addr == "" {
			addr = "127.0.0.1"
		}
		out, err := exec.Command("ping", addr, "-c 2", "-q").Output()
		if err == nil {
			master.latencies[index], _ = strconv.ParseFloat(strings.Split(string(out), "/")[4], 64)
			log.Printf(" node %v [%v] -> %v", index, master.nodeList[index],master.latencies[index])
		}else{
			log.Fatal("cannot connect to "+addr)
		}
	}

	if nlen == master.N {
		reply.Ready = true
		reply.ReplicaId = index
		reply.NodeList = master.nodeList
		reply.IsLeader = false

		minLatency := math.MaxFloat64
		leader := 0
		for i := 0; i < len(master.leader); i++ {
			if master.latencies[i] < minLatency {
				minLatency = master.latencies[i]
				leader = i
			}
		}

		if leader == index {
			log.Printf("Replica %d is the new leader.", index)
			master.leader[index] = true
			reply.IsLeader = true
		}

	} else {
		reply.Ready = false
	}

	return nil
}

func (master *Master) GetLeader(args *masterproto.GetLeaderArgs, reply *masterproto.GetLeaderReply) error {
	master.lock.Lock()
	defer master.lock.Unlock()

	for i, l := range master.leader {
		if l {
			*reply = masterproto.GetLeaderReply{i}
			break
		}
	}
	return nil
}

func (master *Master) GetReplicaList(args *masterproto.GetReplicaListArgs, reply *masterproto.GetReplicaListReply) error {
	master.lock.Lock()
	defer master.lock.Unlock()

	if len(master.nodeList) == master.N {
		reply.Ready = true
	} else {
		reply.Ready = false
	}

	reply.ReplicaList = make([]string, 0)
	reply.AliveList = make([]bool, 0)
	for i,node := range master.nodeList {
		reply.ReplicaList = append(reply.ReplicaList,node)
		reply.AliveList = append(reply.AliveList,master.alive[i])
	}

	log.Printf("nodes list %v", reply.ReplicaList)
	return nil
}
