package main

import (
	"bindings"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"log"
	"math/rand"
	"runtime"
	"state"
	"time"
)

var clientId string = *flag.String("id", "", "the id of the client. Default is RFC 4122 nodeID.")
var masterAddr *string = flag.String("maddr", "", "Master address. Defaults to localhost")
var masterPort *int = flag.Int("mport", 7087, "Master port. ")
var reqsNb *int = flag.Int("q", 1000, "Total number of requests. ")
var writes *int = flag.Int("w", 100, "Percentage of updates (writes). ")
var psize *int = flag.Int("psize", 100, "Payload size for writes.")
var noLeader *bool = flag.Bool("e", false, "Egalitarian (no leader). ")
var fast *bool = flag.Bool("f", false, "Fast Paxos: send message directly to all replicas. ")
var localReads *bool = flag.Bool("l", false, "Execute reads at the closest (local) replica. ")
var procs *int = flag.Int("p", 2, "GOMAXPROCS. ")
var conflicts *int = flag.Int("c", 0, "Percentage of conflicts. Defaults to 0%")
var verbose *bool = flag.Bool("v", false, "verbose mode. ")
var scan *bool = flag.Bool("s", false, "replace read with short scan (100 elements)")

func main() {

	flag.Parse()

	runtime.GOMAXPROCS(*procs)

	rand.Seed(time.Now().UnixNano())

	if *conflicts > 100 {
		log.Fatalf("Conflicts percentage must be between 0 and 100.\n")
	}

	var proxy *bindings.Parameters
	for {
		proxy = bindings.NewParameters(*masterAddr, *masterPort, *verbose, *noLeader, *fast, *localReads)
		err := proxy.Connect()
		if err == nil{
			break
		}
		proxy.Disconnect()
	}

	if clientId == "" {
		clientId = uuid.New().String()
	}

	log.Printf("client: %v (verbose=%v, psize=%v, conflicts=%v)", clientId, *verbose, *psize, *conflicts)

	karray := make([]state.Key, *reqsNb)
	put := make([]bool, *reqsNb)

	clientKey := state.Key(uint64(uuid.New().Time())) // a command id unique to this client.
	for i := 0; i < *reqsNb; i++ {
		put[i] = false
		if *writes > 0 {
			r := rand.Intn(100)
			if r <= *writes {
				put[i] = true
			}
		}
		karray[i] = clientKey
		if *conflicts > 0 {
			r := rand.Intn(100)
			if r <= *conflicts {
				karray[i] = 42
			}
		}
	}

	before_total := time.Now()

	for j := 0; j < *reqsNb; j++ {

		before := time.Now()

		key := int64(karray[j])

		if put[j] {
			value := make([]byte, *psize)
			rand.Read(value)
			proxy.Write(key, state.Value(value))
		} else {
			if *scan{
				proxy.Scan(key,int64(100))
			}else{
				proxy.Read(key)
			}
		}

		after := time.Now()

		duration := after.Sub(before)
		fmt.Printf("latency %d\n", to_ms(duration.Nanoseconds()))
		fmt.Printf("chain %d-1\n", to_ms(after.UnixNano()))
	}

	fmt.Printf(proxy.Stats() + "\n")

	proxy.Disconnect()

	after_total := time.Now()
	fmt.Printf("Test took %v\n", after_total.Sub(before_total))
}

// convert nanosecond to millisecond
func to_ms(nano int64) int64 {
  return nano / int64(time.Millisecond)
}
