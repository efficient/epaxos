package epaxos

import (
	"bloomfilter"
	"dlog"
	"encoding/binary"
	"epaxosproto"
	"fastrpc"
	"genericsmr"
	"genericsmrproto"
	"io"
	"log"
	"math"
	"state"
	"sync"
	"time"
)

const MAX_INSTANCE = 10 * 1024 * 1024

const MAX_DEPTH_DEP = 10
const TRUE = uint8(1)
const FALSE = uint8(0)
const ADAPT_TIME_SEC = 10

const MAX_BATCH = 1

const COMMIT_GRACE_PERIOD = 1 * 1e9 // 1 second(s)

const BF_K = 4
const BF_M_N = 32.0

var bf_PT uint32

const DO_CHECKPOINTING = false
const HT_INIT_SIZE = 200000
const CHECKPOINT_PERIOD = 10000

var cpMarker []state.Command
var cpcounter = 0

type Replica struct {
	*genericsmr.Replica
	prepareChan           chan fastrpc.Serializable
	preAcceptChan         chan fastrpc.Serializable
	acceptChan            chan fastrpc.Serializable
	commitChan            chan fastrpc.Serializable
	commitShortChan       chan fastrpc.Serializable
	prepareReplyChan      chan fastrpc.Serializable
	preAcceptReplyChan    chan fastrpc.Serializable
	preAcceptOKChan       chan fastrpc.Serializable
	acceptReplyChan       chan fastrpc.Serializable
	tryPreAcceptChan      chan fastrpc.Serializable
	tryPreAcceptReplyChan chan fastrpc.Serializable
	prepareRPC            uint8
	prepareReplyRPC       uint8
	preAcceptRPC          uint8
	preAcceptReplyRPC     uint8
	acceptRPC             uint8
	acceptReplyRPC        uint8
	commitRPC             uint8
	commitShortRPC        uint8
	tryPreAcceptRPC       uint8
	tryPreAcceptReplyRPC  uint8
	InstanceSpace         [][]*Instance // the space of all instances (used and not yet used)
	crtInstance           []int32       // highest active instance numbers that this replica knows about
	CommittedUpTo         []int32       // highest committed instance per replica that this replica knows about
	ExecedUpTo            []int32       // instance up to which all commands have been executed (including iteslf)
	exec                  *Exec
	conflicts             []map[state.Key]int32
	maxSeqPerKey          map[state.Key]int32
	maxSeq                int32
	latestCPReplica       int32
	latestCPInstance      int32
	clientMutex           *sync.Mutex // for synchronizing when sending replies to clients from multiple go-routines
	instancesToRecover    chan *instanceId
	IsLeader              bool // does this replica think it is the leader
}

type Instance struct {
	Coordinator    int32
	Cmds           []state.Command
	ballot         int32
	Status         int8
	Seq            int32
	Deps           []int32
	lb             *LeaderBookkeeping
	Index, Lowlink int
	bfilter        *bloomfilter.Bloomfilter
	proposeTime    int64
}

type instanceId struct {
	replica  int32
	instance int32
}

type RecoveryInstance struct {
	cmds            []state.Command
	status          int8
	seq             int32
	deps            []int32
	preAcceptCount  int
	leaderResponded bool
}

type LeaderBookkeeping struct {
	clientProposals   []*genericsmr.Propose
	maxRecvBallot     int32
	prepareOKs        int
	allEqual          bool
	preAcceptOKs      int
	acceptOKs         int
	nacks             int
	originalDeps      []int32
	committedDeps     []int32
	recoveryInst      *RecoveryInstance
	preparing         bool
	tryingToPreAccept bool
	possibleQuorum    []bool
	tpaOKs            int
}

func NewReplica(id int, peerAddrList []string, thrifty bool, exec bool, lread bool, dreply bool, beacon bool, durable bool) *Replica {
	r := &Replica{
		genericsmr.NewReplica(id, peerAddrList, thrifty, exec, lread, dreply),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE*3),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE*3),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE*2),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		make([][]*Instance, len(peerAddrList)),
		make([]int32, len(peerAddrList)),
		make([]int32, len(peerAddrList)),
		make([]int32, len(peerAddrList)),
		nil,
		make([]map[state.Key]int32, len(peerAddrList)),
		make(map[state.Key]int32),
		0,
		0,
		-1,
		new(sync.Mutex),
		make(chan *instanceId, genericsmr.CHAN_BUFFER_SIZE),
		false}

	r.Beacon = beacon
	r.Durable = durable

	for i := 0; i < r.N; i++ {
		r.InstanceSpace[i] = make([]*Instance, MAX_INSTANCE) // FIXME
		r.crtInstance[i] = 0
		r.ExecedUpTo[i] = -1
		r.CommittedUpTo[i] = -1
		r.conflicts[i] = make(map[state.Key]int32, HT_INIT_SIZE)
	}

	for bf_PT = 1; math.Pow(2, float64(bf_PT))/float64(MAX_BATCH) < BF_M_N; {
		bf_PT++
	}

	r.exec = &Exec{r}

	cpMarker = make([]state.Command, 0)

	//register RPCs
	r.prepareRPC = r.RegisterRPC(new(epaxosproto.Prepare), r.prepareChan)
	r.prepareReplyRPC = r.RegisterRPC(new(epaxosproto.PrepareReply), r.prepareReplyChan)
	r.preAcceptRPC = r.RegisterRPC(new(epaxosproto.PreAccept), r.preAcceptChan)
	r.preAcceptReplyRPC = r.RegisterRPC(new(epaxosproto.PreAcceptReply), r.preAcceptReplyChan)
	r.acceptRPC = r.RegisterRPC(new(epaxosproto.Accept), r.acceptChan)
	r.acceptReplyRPC = r.RegisterRPC(new(epaxosproto.AcceptReply), r.acceptReplyChan)
	r.commitRPC = r.RegisterRPC(new(epaxosproto.Commit), r.commitChan)
	r.commitShortRPC = r.RegisterRPC(new(epaxosproto.CommitShort), r.commitShortChan)
	r.tryPreAcceptRPC = r.RegisterRPC(new(epaxosproto.TryPreAccept), r.tryPreAcceptChan)
	r.tryPreAcceptReplyRPC = r.RegisterRPC(new(epaxosproto.TryPreAcceptReply), r.tryPreAcceptReplyChan)

	r.Stats.M["weird"], r.Stats.M["conflicted"], r.Stats.M["slow"], r.Stats.M["fast"], r.Stats.M["totalCommitTime"], r.Stats.M["totalBatching"], r.Stats.M["totalBatchingSize"] = 0, 0, 0, 0, 0, 0, 0

	go r.run()

	return r
}

//append a log entry to stable storage
func (r *Replica) recordInstanceMetadata(inst *Instance) {
	if !r.Durable {
		return
	}

	b := make([]byte, 9+r.N*4)
	binary.LittleEndian.PutUint32(b[0:4], uint32(inst.ballot))
	b[4] = byte(inst.Status)
	binary.LittleEndian.PutUint32(b[5:9], uint32(inst.Seq))
	l := 9
	for _, dep := range inst.Deps {
		binary.LittleEndian.PutUint32(b[l:l+4], uint32(dep))
		l += 4
	}
	r.StableStore.Write(b[:])
}

//write a sequence of commands to stable storage
func (r *Replica) recordCommands(cmds []state.Command) {
	if !r.Durable {
		return
	}

	if cmds == nil {
		return
	}
	for i := 0; i < len(cmds); i++ {
		cmds[i].Marshal(io.Writer(r.StableStore))
	}
}

//sync with the stable store
func (r *Replica) sync() {
	if !r.Durable {
		return
	}

	r.StableStore.Sync()
}

/* Clock goroutine */

var fastClockChan chan bool
var slowClockChan chan bool

func (r *Replica) fastClock() {
	for !r.Shutdown {
		time.Sleep(5 * 1e6) // 5 ms
		fastClockChan <- true
	}
}
func (r *Replica) slowClock() {
	for !r.Shutdown {
		time.Sleep(150 * 1e6) // 150 ms
		slowClockChan <- true
	}
}

func (r *Replica) stopAdapting() {
	time.Sleep(1000 * 1000 * 1000 * ADAPT_TIME_SEC)
	r.Beacon = false
	time.Sleep(1000 * 1000 * 1000)

	for i := 0; i < r.N-1; i++ {
		min := i
		for j := i + 1; j < r.N-1; j++ {
			if r.Ewma[r.PreferredPeerOrder[j]] < r.Ewma[r.PreferredPeerOrder[min]] {
				min = j
			}
		}
		aux := r.PreferredPeerOrder[i]
		r.PreferredPeerOrder[i] = r.PreferredPeerOrder[min]
		r.PreferredPeerOrder[min] = aux
	}

	log.Println(r.PreferredPeerOrder)
}

/* ============= */

/***********************************
   Main event processing loop      *
************************************/

func (r *Replica) run() {
	r.ConnectToPeers()

	r.ComputeClosestPeers()

	if r.Exec {
		go r.executeCommands()
	}

	slowClockChan = make(chan bool, 1)
	fastClockChan = make(chan bool, 1)
	go r.slowClock()

	//Enabled when batching for 5ms
	if MAX_BATCH > 100 {
		go r.fastClock()
	}

	if r.Beacon {
		go r.stopAdapting()
	}

	onOffProposeChan := r.ProposeChan

	go r.WaitForClientConnections()

	for !r.Shutdown {

		select {

		case propose := <-onOffProposeChan:
			//got a Propose from a client
			dlog.Printf("Proposal with op %d\n", propose.Command.Op)
			r.handlePropose(propose)
			//deactivate new proposals channel to prioritize the handling of other protocol messages,
			//and to allow commands to accumulate for batching
			if MAX_BATCH > 100 {
				onOffProposeChan = nil
			}
			break

		case <-fastClockChan:
			//activate new proposals channel
			onOffProposeChan = r.ProposeChan
			break

		case prepareS := <-r.prepareChan:
			prepare := prepareS.(*epaxosproto.Prepare)
			//got a Prepare message
			dlog.Printf("Received Prepare %d.%d w. ballot %d\n", prepare.Replica, prepare.Instance, prepare.Ballot)
			r.handlePrepare(prepare)
			break

		case preAcceptS := <-r.preAcceptChan:
			preAccept := preAcceptS.(*epaxosproto.PreAccept)
			//got a PreAccept message
			dlog.Printf("Received PreAccept %d.%d w. ballot %d\n", preAccept.LeaderId, preAccept.Instance, preAccept.Ballot)
			r.handlePreAccept(preAccept)
			break

		case acceptS := <-r.acceptChan:
			accept := acceptS.(*epaxosproto.Accept)
			//got an Accept message
			dlog.Printf("Received Accept %d.%d w. ballot %d\n", accept.LeaderId, accept.Instance, accept.Ballot)
			r.handleAccept(accept)
			break

		case commitS := <-r.commitChan:
			commit := commitS.(*epaxosproto.Commit)
			//got a Commit message
			dlog.Printf("Received Commit %d.%d\n", commit.LeaderId, commit.Instance)
			r.handleCommit(commit)
			break

		case commitS := <-r.commitShortChan:
			commit := commitS.(*epaxosproto.CommitShort)
			//got a Commit message
			dlog.Printf("Received (short) Commit %d.%d\n", commit.LeaderId, commit.Instance)
			r.handleCommitShort(commit)
			break

		case prepareReplyS := <-r.prepareReplyChan:
			prepareReply := prepareReplyS.(*epaxosproto.PrepareReply)
			//got a Prepare reply
			dlog.Printf("Received PrepareReply %d.%d w. ballot %d from %d\n", prepareReply.Replica, prepareReply.Instance, prepareReply.Ballot, prepareReply.AcceptorId)
			r.handlePrepareReply(prepareReply)
			break

		case preAcceptReplyS := <-r.preAcceptReplyChan:
			preAcceptReply := preAcceptReplyS.(*epaxosproto.PreAcceptReply)
			//got a PreAccept reply
			dlog.Printf("Received PreAcceptReply %d.%d w. ballot %d\n", preAcceptReply.Replica, preAcceptReply.Instance, preAcceptReply.Ballot)
			r.handlePreAcceptReply(preAcceptReply)
			break

		case acceptReplyS := <-r.acceptReplyChan:
			acceptReply := acceptReplyS.(*epaxosproto.AcceptReply)
			//got an Accept reply
			dlog.Printf("Received AcceptReply %d.%d w. ballot %d\n", acceptReply.Replica, acceptReply.Instance, acceptReply.Ballot)
			r.handleAcceptReply(acceptReply)
			break

		case tryPreAcceptS := <-r.tryPreAcceptChan:
			tryPreAccept := tryPreAcceptS.(*epaxosproto.TryPreAccept)
			dlog.Printf("Received TryPreAccept %d.%d w. ballot %d, deps %d\n", tryPreAccept.Replica, tryPreAccept.Instance, tryPreAccept.Ballot, tryPreAccept.Deps)
			r.handleTryPreAccept(tryPreAccept)
			break

		case tryPreAcceptReplyS := <-r.tryPreAcceptReplyChan:
			tryPreAcceptReply := tryPreAcceptReplyS.(*epaxosproto.TryPreAcceptReply)
			dlog.Printf("Received TryPreAcceptReply %d.%d\n", tryPreAcceptReply.Replica, tryPreAcceptReply.Instance)
			r.handleTryPreAcceptReply(tryPreAcceptReply)
			break

		case beacon := <-r.BeaconChan:
			dlog.Printf("Received Beacon from replica %d with timestamp %d\n", beacon.Rid, beacon.Timestamp)
			r.ReplyBeacon(beacon)
			break

		case <-slowClockChan:
			if r.Beacon {
				log.Printf("weird %d; conflicted %d; slow %d; fast %d\n", r.Stats.M["weird"], r.Stats.M["conflicted"], r.Stats.M["slow"], r.Stats.M["fast"])
				for q := int32(0); q < int32(r.N); q++ {
					if q == r.Id {
						continue
					}
					r.SendBeacon(q)
				}
			}
			break

		case iid := <-r.instancesToRecover:
			r.startRecoveryForInstance(iid.replica, iid.instance)
		}
	}
}

/***********************************
   Command execution thread        *
************************************/

func (r *Replica) executeCommands() {
	const SLEEP_TIME_NS = 1e6
	problemInstance := make([]int32, r.N)
	timeout := make([]uint64, r.N)
	for q := 0; q < r.N; q++ {
		problemInstance[q] = -1
		timeout[q] = 0
	}

	for !r.Shutdown {
		executed := false
		for q := 0; q < r.N; q++ {
			inst := int32(0)
			for inst = r.ExecedUpTo[q] + 1; inst < r.crtInstance[q]; inst++ {
				if r.InstanceSpace[q][inst] != nil && r.InstanceSpace[q][inst].Status == epaxosproto.EXECUTED {
					if inst == r.ExecedUpTo[q]+1 {
						r.ExecedUpTo[q] = inst
					}
					continue
				}
				if r.InstanceSpace[q][inst] == nil || r.InstanceSpace[q][inst].Status != epaxosproto.COMMITTED {
					if inst == problemInstance[q] {
						timeout[q] += SLEEP_TIME_NS
						if timeout[q] >= COMMIT_GRACE_PERIOD {
							if r.IsLeader || timeout[q] >= COMMIT_GRACE_PERIOD*uint64(r.Id+1) {
								dlog.Printf("Recovery for %d.%d", q, inst)
								r.instancesToRecover <- &instanceId{int32(q), inst}
								timeout[q] = 0
							}
						}
					} else {
						problemInstance[q] = inst
						timeout[q] = 0
					}
					//if r.InstanceSpace[q][inst] == nil {
					//	continue
					//}
					break // stop at the first problematic instance
				}
				if ok := r.exec.executeCommand(int32(q), inst); ok {
					executed = true
					if inst == r.ExecedUpTo[q]+1 {
						r.ExecedUpTo[q] = inst
					}
				}
			}
		}
		if !executed {
			r.Mutex.Lock()
			r.Mutex.Unlock() // FIXME for cache coherence
			time.Sleep(SLEEP_TIME_NS)
		}
		//log.Println(r.ExecedUpTo, " ", r.crtInstance)
	}
}

/* Ballot helper functions */

func (r *Replica) makeUniqueBallot(ballot int32) int32 {
	return (ballot << 4) | r.Id
}

func (r *Replica) makeBallotLargerThan(ballot int32) int32 {
	return r.makeUniqueBallot((ballot >> 4) + 1)
}

func isInitialBallot(ballot int32) bool {
	return (ballot >> 4) == 0
}

func replicaIdFromBallot(ballot int32) int32 {
	return ballot & 15
}

/**********************************************************************
                    inter-replica communication
***********************************************************************/

func (r *Replica) replyPrepare(replicaId int32, reply *epaxosproto.PrepareReply) {
	dlog.Printf("Sending PrepareReply %d.%d w. ballot=%d, status=%d to %d\n", reply.Replica, reply.Instance, reply.Ballot, reply.Status, replicaId)
	r.SendMsg(replicaId, r.prepareReplyRPC, reply)
}

func (r *Replica) replyPreAccept(replicaId int32, reply *epaxosproto.PreAcceptReply) {
	dlog.Printf("Sending ReplyPreAccept %d.%d w. ballot=%d, deps=%d, committedDeps=%d to %d\n", reply.Replica, reply.Instance, reply.Ballot, reply.Deps, reply.CommittedDeps, replicaId)
	r.SendMsg(replicaId, r.preAcceptReplyRPC, reply)
}

func (r *Replica) replyAccept(replicaId int32, reply *epaxosproto.AcceptReply) {
	dlog.Printf("Sending AcceptReply %d.%d w. ballot=%d to %d\n", reply.Replica, reply.Instance, reply.Ballot, replicaId)
	r.SendMsg(replicaId, r.acceptReplyRPC, reply)
}

func (r *Replica) replyTryPreAccept(replicaId int32, reply *epaxosproto.TryPreAcceptReply) {
	dlog.Printf("Sending TryPreAcceptReply %d.%d w. ballot=%d to %d\n", reply.Replica, reply.Instance, reply.Ballot, replicaId)
	r.SendMsg(replicaId, r.tryPreAcceptReplyRPC, reply)
}

func (r *Replica) bcastPrepare(replica int32, instance int32, ballot int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Prepare bcast failed:", err)
		}
	}()
	args := &epaxosproto.Prepare{r.Id, replica, instance, ballot}

	n := r.N - 1
	if r.Thrifty {
		n = r.N / 2
	}
	q := r.Id
	for sent := 0; sent < n; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			log.Fatal("Not enough replicas alive!")
		}
		if !r.Alive[q] {
			continue
		}
		dlog.Printf("Sending Prepare %d.%d w. ballot %d to %d\n", replica, instance, ballot, q)
		r.SendMsg(q, r.prepareRPC, args)
		sent++
	}
}

func (r *Replica) bcastPreAccept(replica int32, instance int32, ballot int32, cmds []state.Command, seq int32, deps []int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("PreAccept bcast failed:", err)
		}
	}()
	pa := new(epaxosproto.PreAccept)
	pa.LeaderId = r.Id
	pa.Replica = replica
	pa.Instance = instance
	pa.Ballot = ballot
	pa.Command = cmds
	pa.Seq = seq
	pa.Deps = deps

	n := r.N - 1
	if r.Thrifty {
		n = r.fastQuorumSize() - 1
	}

	sent := 0
	for q := 0; q < r.N-1; q++ {
		if !r.Alive[r.PreferredPeerOrder[q]] {
			continue
		}
		dlog.Printf("Sending PreAccept %d.%d w. ballot %d and deps %d to %d\n", replica, instance, ballot, deps, q)
		r.SendMsg(r.PreferredPeerOrder[q], r.preAcceptRPC, pa)
		sent++
		if sent >= n {
			break
		}
	}
}

func (r *Replica) bcastTryPreAccept(replica int32, instance int32, ballot int32, cmds []state.Command, seq int32, deps []int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("PreAccept bcast failed:", err)
		}
	}()
	tpa := new(epaxosproto.TryPreAccept)
	tpa.LeaderId = r.Id
	tpa.Replica = replica
	tpa.Instance = instance
	tpa.Ballot = ballot
	tpa.Command = cmds
	tpa.Seq = seq
	tpa.Deps = deps

	for q := int32(0); q < int32(r.N); q++ {
		if q == r.Id {
			continue
		}
		if !r.Alive[q] {
			continue
		}
		dlog.Printf("Sending TryPreAccept %d.%d w. ballot %d and deps %d to %d\n", replica, instance, ballot, deps, q)
		r.SendMsg(q, r.tryPreAcceptRPC, tpa)
	}
}

func (r *Replica) bcastAccept(replica int32, instance int32, ballot int32, count int32, seq int32, deps []int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Accept bcast failed:", err)
		}
	}()

	ea := new(epaxosproto.Accept)
	ea.LeaderId = r.Id
	ea.Replica = replica
	ea.Instance = instance
	ea.Ballot = ballot
	ea.Count = count
	ea.Seq = seq
	ea.Deps = deps

	n := r.N - 1
	if r.Thrifty {
		n = r.N / 2
	}

	sent := 0
	for q := 0; q < r.N-1; q++ {
		if !r.Alive[r.PreferredPeerOrder[q]] {
			continue
		}
		dlog.Printf("Sending Accept %d.%d w. ballot %d to %d\n", replica, instance, ballot, q)
		r.SendMsg(r.PreferredPeerOrder[q], r.acceptRPC, ea)
		sent++
		if sent >= n {
			break
		}
	}
}

func (r *Replica) bcastCommit(replica int32, instance int32, cmds []state.Command, seq int32, deps []int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Commit bcast failed:", err)
		}
	}()
	ec := new(epaxosproto.Commit)
	ec.LeaderId = r.Id
	ec.Replica = replica
	ec.Instance = instance
	ec.Command = cmds
	ec.Seq = seq
	ec.Deps = deps

	for q := 0; q < r.N-1; q++ {
		if !r.Alive[r.PreferredPeerOrder[q]] {
			continue
		}
		dlog.Printf("Sending Commit %d.%d to %d\n", replica, instance, r.PreferredPeerOrder[q])
		r.SendMsg(r.PreferredPeerOrder[q], r.commitRPC, ec)
	}
}

/******************************************************************
               Helper functions
*******************************************************************/

func (r *Replica) clearHashtables() {
	for q := 0; q < r.N; q++ {
		r.conflicts[q] = make(map[state.Key]int32, HT_INIT_SIZE)
	}
}

func (r *Replica) updateCommitted(replica int32) {
	r.Mutex.Lock()
	for r.InstanceSpace[replica][r.CommittedUpTo[replica]+1] != nil &&
		(r.InstanceSpace[replica][r.CommittedUpTo[replica]+1].Status == epaxosproto.COMMITTED ||
			r.InstanceSpace[replica][r.CommittedUpTo[replica]+1].Status == epaxosproto.EXECUTED) {
		r.CommittedUpTo[replica] = r.CommittedUpTo[replica] + 1
	}
	r.Mutex.Unlock()
}

func (r *Replica) updateConflicts(cmds []state.Command, replica int32, instance int32, seq int32) {
	for i := 0; i < len(cmds); i++ {
		if d, present := r.conflicts[replica][cmds[i].K]; present {
			if d < instance {
				r.conflicts[replica][cmds[i].K] = instance
			}
		} else {
			r.conflicts[replica][cmds[i].K] = instance
		}
		if s, present := r.maxSeqPerKey[cmds[i].K]; present {
			if s < seq {
				r.maxSeqPerKey[cmds[i].K] = seq
			}
		} else {
			r.maxSeqPerKey[cmds[i].K] = seq
		}
	}
}

func (r *Replica) updateAttributes(cmds []state.Command, seq int32, deps []int32, replica int32, instance int32) (int32, []int32, bool) {
	changed := false
	for q := 0; q < r.N; q++ {
		if r.Id != replica && int32(q) == replica {
			continue
		}
		for i := 0; i < len(cmds); i++ {
			if d, present := (r.conflicts[q])[cmds[i].K]; present {
				if d > deps[q] {
					deps[q] = d
					if seq <= r.InstanceSpace[q][d].Seq {
						seq = r.InstanceSpace[q][d].Seq + 1
					}
					changed = true
					break
				}
			}
		}
	}
	for i := 0; i < len(cmds); i++ {
		if s, present := r.maxSeqPerKey[cmds[i].K]; present {
			if seq <= s {
				changed = true
				seq = s + 1
			}
		}
	}

	return seq, deps, changed
}

func (r *Replica) mergeAttributes(seq1 int32, deps1 []int32, seq2 int32, deps2 []int32) (int32, []int32, bool) {
	equal := true
	if seq1 != seq2 {
		equal = false
		if seq2 > seq1 {
			seq1 = seq2
		}
	}
	for q := 0; q < r.N; q++ {
		if int32(q) == r.Id {
			continue
		}
		if deps1[q] != deps2[q] {
			equal = false
			if deps2[q] > deps1[q] {
				deps1[q] = deps2[q]
			}
		}
	}
	return seq1, deps1, equal
}

func equal(deps1 []int32, deps2 []int32) bool {
	for i := 0; i < len(deps1); i++ {
		if deps1[i] != deps2[i] {
			return false
		}
	}
	return true
}

func bfFromCommands(cmds []state.Command) *bloomfilter.Bloomfilter {
	if cmds == nil {
		return nil
	}

	bf := bloomfilter.NewPowTwo(bf_PT, BF_K)

	for i := 0; i < len(cmds); i++ {
		bf.AddUint64(uint64(cmds[i].K))
	}

	return bf
}

/**********************************************************************

                            PHASE 1

***********************************************************************/

func (r *Replica) handlePropose(propose *genericsmr.Propose) {
	//TODO!! Handle client retries

	batchSize := len(r.ProposeChan) + 1
	if batchSize > MAX_BATCH {
		batchSize = MAX_BATCH
	}
	r.Mutex.Lock()
	r.Stats.M["totalBatching"]++
	r.Stats.M["totalBatchingSize"] += batchSize
	r.Mutex.Unlock()

	instNo := r.crtInstance[r.Id]
	r.crtInstance[r.Id]++

	dlog.Printf("Starting %d.%d w. batching %d\n", r.Id, instNo, batchSize)

	cmds := make([]state.Command, batchSize)
	proposals := make([]*genericsmr.Propose, batchSize)
	cmds[0] = propose.Command
	proposals[0] = propose
	for i := 1; i < batchSize; i++ {
		prop := <-r.ProposeChan
		cmds[i] = prop.Command
		proposals[i] = prop
	}

	r.startPhase1(r.Id, instNo, 0, proposals, cmds, batchSize)
}

func (r *Replica) startPhase1(replica int32, instance int32, ballot int32, proposals []*genericsmr.Propose, cmds []state.Command, batchSize int) {
	//init command attributes

	seq := int32(0)
	deps := make([]int32, r.N)
	for q := 0; q < r.N; q++ {
		deps[q] = -1
	}

	seq, deps, _ = r.updateAttributes(cmds, seq, deps, replica, instance)

	comDeps := make([]int32, r.N)
	for i := 0; i < r.N; i++ {
		comDeps[i] = -1
	}

	r.InstanceSpace[r.Id][instance] = &Instance{
		replica,
		cmds,
		ballot,
		epaxosproto.PREACCEPTED,
		seq,
		deps,
		&LeaderBookkeeping{proposals, 0, 0, true, 0, 0, 0, deps, comDeps, nil, false, false, nil, 0}, 0, 0,
		nil,
		time.Now().UnixNano()}

	r.updateConflicts(cmds, r.Id, instance, seq)

	if seq >= r.maxSeq {
		r.maxSeq = seq
	}

	r.recordInstanceMetadata(r.InstanceSpace[r.Id][instance])
	r.recordCommands(cmds)
	r.sync()

	r.bcastPreAccept(r.Id, instance, ballot, cmds, seq, deps)

	cpcounter += batchSize

	if r.Id == 0 && DO_CHECKPOINTING && cpcounter >= CHECKPOINT_PERIOD {
		cpcounter = 0

		//Propose a checkpoint command to act like a barrier.
		//This allows replicas to discard their dependency hashtables.
		r.crtInstance[r.Id]++
		instance++

		r.maxSeq++
		for q := 0; q < r.N; q++ {
			deps[q] = r.crtInstance[q] - 1
		}

		r.InstanceSpace[r.Id][instance] = &Instance{
			replica,
			cpMarker,
			0,
			epaxosproto.PREACCEPTED,
			r.maxSeq,
			deps,
			&LeaderBookkeeping{nil, 0, 0, true, 0, 0, 0, deps, nil, nil, false, false, nil, 0},
			0,
			0,
			nil,
			0}

		r.latestCPReplica = r.Id
		r.latestCPInstance = instance

		//discard dependency hashtables
		r.clearHashtables()

		r.recordInstanceMetadata(r.InstanceSpace[r.Id][instance])
		r.sync()

		r.bcastPreAccept(r.Id, instance, 0, cpMarker, r.maxSeq, deps)
	}
}

func (r *Replica) handlePreAccept(preAccept *epaxosproto.PreAccept) {
	inst := r.InstanceSpace[preAccept.LeaderId][preAccept.Instance]

	if preAccept.Seq >= r.maxSeq {
		r.maxSeq = preAccept.Seq + 1
	}

	if inst != nil && (inst.Status == epaxosproto.COMMITTED || inst.Status == epaxosproto.ACCEPTED) {
		//reordered handling of commit/accept and pre-accept
		if inst.Cmds == nil {
			r.InstanceSpace[preAccept.LeaderId][preAccept.Instance].Cmds = preAccept.Command
			r.updateConflicts(preAccept.Command, preAccept.Replica, preAccept.Instance, preAccept.Seq)
			//r.InstanceSpace[preAccept.LeaderId][preAccept.Instance].bfilter = bfFromCommands(preAccept.Command)
		}
		r.recordCommands(preAccept.Command)
		r.sync()
		dlog.Println("Warping here")
		return
	}

	if preAccept.Instance >= r.crtInstance[preAccept.Replica] {
		r.crtInstance[preAccept.Replica] = preAccept.Instance + 1
	}

	//update attributes for command
	seq, deps, changed := r.updateAttributes(preAccept.Command, preAccept.Seq, preAccept.Deps, preAccept.Replica, preAccept.Instance)
	status := epaxosproto.PREACCEPTED_EQ
	if changed {
		status = epaxosproto.PREACCEPTED
	}

	if inst != nil {
		if preAccept.Ballot < inst.ballot {
			dlog.Printf("I say no w. seq=%d, deps=%d, committedUpTo=%d\n", inst.Seq, inst.Deps, r.CommittedUpTo)
			//r.replyPreAccept(preAccept.LeaderId,
			//	&epaxosproto.PreAcceptReply{
			//		preAccept.Replica,
			//		preAccept.Instance,
			//		FALSE,
			//		inst.ballot,
			//		inst.Seq,
			//		inst.Deps,
			//		r.CommittedUpTo}) FIXME
			return
		} else {
			inst.Cmds = preAccept.Command
			inst.Seq = seq
			inst.Deps = deps
			inst.ballot = preAccept.Ballot
			inst.Status = status
		}
	} else {
		dlog.Printf("Never heard of %d.%d\n", preAccept.Replica, preAccept.Instance)
		r.InstanceSpace[preAccept.Replica][preAccept.Instance] = &Instance{
			preAccept.Replica,
			preAccept.Command,
			preAccept.Ballot,
			status,
			seq,
			deps,
			nil, 0, 0,
			nil,
			0}
	}

	r.updateConflicts(preAccept.Command, preAccept.Replica, preAccept.Instance, preAccept.Seq)

	r.recordInstanceMetadata(r.InstanceSpace[preAccept.Replica][preAccept.Instance])
	r.recordCommands(preAccept.Command)
	r.sync()

	if len(preAccept.Command) == 0 {
		//checkpoint
		//update latest checkpoint info
		r.latestCPReplica = preAccept.Replica
		r.latestCPInstance = preAccept.Instance

		//discard dependency hashtables
		r.clearHashtables()
	}

	r.replyPreAccept(preAccept.LeaderId,
		&epaxosproto.PreAcceptReply{
			preAccept.Replica,
			preAccept.Instance,
			TRUE,
			preAccept.Ballot,
			seq,
			deps,
			r.CommittedUpTo})

	dlog.Printf("I've replied to the PreAccept\n")
}

func (r *Replica) handlePreAcceptReply(pareply *epaxosproto.PreAcceptReply) {
	inst := r.InstanceSpace[pareply.Replica][pareply.Instance]

	if inst.ballot != pareply.Ballot {
		dlog.Printf("Wrong ballot\n")
		return
	}

	if inst.Status != epaxosproto.PREACCEPTED {
		dlog.Printf("Delayed PreAcceptReply\n")
		// we've moved on, this is a delayed reply
		return
	}

	if pareply.OK == FALSE {
		// TODO: there is probably another active leader
		inst.lb.nacks++
		if pareply.Ballot > inst.lb.maxRecvBallot {
			inst.lb.maxRecvBallot = pareply.Ballot
		}
		if inst.lb.nacks >= r.N/2 {
			// TODO
		}
		dlog.Printf("Another active leader\n")
		return
	}

	inst.lb.preAcceptOKs++

	var equal bool
	inst.Seq, inst.Deps, equal = r.mergeAttributes(inst.Seq, inst.Deps, pareply.Seq, pareply.Deps)
	if (r.N <= 3 && !r.Thrifty) || inst.lb.preAcceptOKs > 1 {
		inst.lb.allEqual = inst.lb.allEqual && equal
		if !equal {
			r.Mutex.Lock()
			r.Stats.M["conflicted"]++
			r.Mutex.Unlock()
		}
	}

	allCommitted := true
	//// FIXME r.N \leq 7 (following section 4.4 in SOSP)
	//for q := 0; q < r.N; q++ {
	//	if inst.lb.committedDeps[q] < pareply.CommittedDeps[q] {
	//		inst.lb.committedDeps[q] = pareply.CommittedDeps[q]
	//	}
	//	if inst.lb.committedDeps[q] < r.CommittedUpTo[q] {
	//		inst.lb.committedDeps[q] = r.CommittedUpTo[q]
	//	}
	//	if inst.lb.committedDeps[q] < inst.Deps[q] {
	//		allCommitted = false
	//	}
	//}

	//can we commit on the fast path?
	if inst.lb.preAcceptOKs >= (r.fastQuorumSize()-1) && inst.lb.allEqual && allCommitted && isInitialBallot(inst.ballot) {
		r.Mutex.Lock()
		r.Stats.M["fast"]++
		r.Mutex.Unlock()
		dlog.Printf("Fast path %d.%d, w. deps %d\n", pareply.Replica, pareply.Instance, pareply.Deps)
		r.InstanceSpace[pareply.Replica][pareply.Instance].Status = epaxosproto.COMMITTED
		r.updateCommitted(pareply.Replica)
		if inst.lb.clientProposals != nil && !r.Dreply {
			// give clients the all clear
			for i := 0; i < len(inst.lb.clientProposals); i++ {
				r.ReplyProposeTS(
					&genericsmrproto.ProposeReplyTS{
						TRUE,
						inst.lb.clientProposals[i].CommandId,
						state.NIL(),
						inst.lb.clientProposals[i].Timestamp},
					inst.lb.clientProposals[i].Reply,
					inst.lb.clientProposals[i].Mutex)
			}
		}

		r.recordInstanceMetadata(inst)
		r.sync() //is this necessary here?

		r.bcastCommit(pareply.Replica, pareply.Instance, inst.Cmds, inst.Seq, inst.Deps)
		r.Mutex.Lock()
		r.Stats.M["totalCommitTime"] += int(time.Now().UnixNano() - inst.proposeTime)
		r.Mutex.Unlock()
	} else if inst.lb.preAcceptOKs >= r.fastQuorumSize()-1 {
		if !allCommitted {
			r.Mutex.Lock()
			r.Stats.M["weird"]++
			r.Mutex.Unlock()
		}
		dlog.Printf("Slow path %d.%d\n", pareply.Replica, pareply.Instance)
		r.Mutex.Lock()
		r.Stats.M["slow"]++
		r.Mutex.Unlock()
		inst.Status = epaxosproto.ACCEPTED
		r.bcastAccept(pareply.Replica, pareply.Instance, inst.ballot, int32(len(inst.Cmds)), inst.Seq, inst.Deps)
	} else {
		dlog.Printf("Nothing to do\n")
	}
	//TODO: take the slow path if messages are slow to arrive
}

/**********************************************************************

                        PHASE 2

***********************************************************************/

func (r *Replica) handleAccept(accept *epaxosproto.Accept) {
	inst := r.InstanceSpace[accept.Replica][accept.Instance]

	if accept.Seq >= r.maxSeq {
		r.maxSeq = accept.Seq + 1
	}

	if inst != nil && (inst.Status == epaxosproto.COMMITTED || inst.Status == epaxosproto.EXECUTED) {
		dlog.Printf("Already committed %d\n", inst.Status)
		return
	}

	if accept.Instance >= r.crtInstance[accept.LeaderId] {
		r.crtInstance[accept.LeaderId] = accept.Instance + 1
	}

	if inst != nil {
		if accept.Ballot < inst.ballot {
			r.replyAccept(accept.LeaderId, &epaxosproto.AcceptReply{accept.Replica, accept.Instance, FALSE, inst.ballot})
			dlog.Println("Lower ballot")
			return
		}
		inst.Status = epaxosproto.ACCEPTED
		inst.Seq = accept.Seq
		inst.Deps = accept.Deps
	} else {
		r.InstanceSpace[accept.LeaderId][accept.Instance] = &Instance{
			accept.LeaderId,
			nil,
			accept.Ballot,
			epaxosproto.ACCEPTED,
			accept.Seq,
			accept.Deps,
			nil, 0, 0, nil, 0}

		if accept.Count == 0 {
			//checkpoint
			//update latest checkpoint info
			r.latestCPReplica = accept.Replica
			r.latestCPInstance = accept.Instance

			//discard dependency hashtables
			r.clearHashtables()
		}
	}

	r.recordInstanceMetadata(r.InstanceSpace[accept.Replica][accept.Instance])
	r.sync()

	r.replyAccept(accept.LeaderId,
		&epaxosproto.AcceptReply{
			accept.Replica,
			accept.Instance,
			TRUE,
			accept.Ballot})
}

func (r *Replica) handleAcceptReply(areply *epaxosproto.AcceptReply) {
	inst := r.InstanceSpace[areply.Replica][areply.Instance]

	if inst.Status != epaxosproto.ACCEPTED {
		// we've move on, these are delayed replies, so just ignore
		dlog.Println("Delayed")
		return
	}

	if inst.ballot != areply.Ballot {
		dlog.Println("Wrong ballot")
		return
	}

	if areply.OK == FALSE {
		// TODO: there is probably another active leader
		inst.lb.nacks++
		if areply.Ballot > inst.lb.maxRecvBallot {
			inst.lb.maxRecvBallot = areply.Ballot
		}
		if inst.lb.nacks >= r.N/2 {
			// TODO
		}
		dlog.Println("Another leader")
		return
	}

	inst.lb.acceptOKs++

	if inst.lb.acceptOKs+1 > r.N/2 {
		r.InstanceSpace[areply.Replica][areply.Instance].Status = epaxosproto.COMMITTED
		r.updateCommitted(areply.Replica)
		if inst.lb.clientProposals != nil && !r.Dreply {
			// give clients the all clear
			for i := 0; i < len(inst.lb.clientProposals); i++ {
				r.ReplyProposeTS(
					&genericsmrproto.ProposeReplyTS{
						TRUE,
						inst.lb.clientProposals[i].CommandId,
						state.NIL(),
						inst.lb.clientProposals[i].Timestamp},
					inst.lb.clientProposals[i].Reply,
					inst.lb.clientProposals[i].Mutex)
			}
		}

		r.recordInstanceMetadata(inst)
		r.sync() //is this necessary here?

		r.bcastCommit(areply.Replica, areply.Instance, inst.Cmds, inst.Seq, inst.Deps)
		r.Mutex.Lock()
		r.Stats.M["totalCommitTime"] += int(time.Now().UnixNano() - inst.proposeTime)
		r.Mutex.Unlock()
	} else {
		dlog.Println("Not enough")
	}
}

/**********************************************************************

                            COMMIT

***********************************************************************/

func (r *Replica) handleCommit(commit *epaxosproto.Commit) {
	inst := r.InstanceSpace[commit.Replica][commit.Instance]

	if commit.Seq >= r.maxSeq {
		r.maxSeq = commit.Seq + 1
	}

	if commit.Instance >= r.crtInstance[commit.Replica] {
		r.crtInstance[commit.Replica] = commit.Instance + 1
	}

	if inst != nil {
		if inst.lb != nil && inst.lb.clientProposals != nil && len(commit.Command) == 0 {
			//someone committed a NO-OP, but we have proposals for this instance
			//try in a different instance
			for _, p := range inst.lb.clientProposals {
				dlog.Printf("Re-trying %d\n", p.Command)
				r.ProposeChan <- p
			}
			inst.lb = nil
		}
		inst.Cmds = commit.Command
		inst.Seq = commit.Seq
		inst.Deps = commit.Deps
		inst.Status = epaxosproto.COMMITTED
	} else {
		r.InstanceSpace[commit.Replica][int(commit.Instance)] = &Instance{
			commit.Replica,
			commit.Command,
			0,
			epaxosproto.COMMITTED,
			commit.Seq,
			commit.Deps,
			nil,
			0,
			0,
			nil, 0}
		r.updateConflicts(commit.Command, commit.Replica, commit.Instance, commit.Seq)

		if len(commit.Command) == 0 {
			//checkpoint
			//update latest checkpoint info
			r.latestCPReplica = commit.Replica
			r.latestCPInstance = commit.Instance

			//discard dependency hashtables
			r.clearHashtables()
		}
	}
	r.updateCommitted(commit.Replica)

	r.recordInstanceMetadata(r.InstanceSpace[commit.Replica][commit.Instance])
	r.recordCommands(commit.Command)
}

func (r *Replica) handleCommitShort(commit *epaxosproto.CommitShort) {
	inst := r.InstanceSpace[commit.Replica][commit.Instance]

	if commit.Instance >= r.crtInstance[commit.Replica] {
		r.crtInstance[commit.Replica] = commit.Instance + 1
	}

	if inst != nil {
		if inst.lb != nil && inst.lb.clientProposals != nil {
			//try command in a different instance
			for _, p := range inst.lb.clientProposals {
				r.ProposeChan <- p
			}
			inst.lb = nil
		}
		inst.Seq = commit.Seq
		inst.Deps = commit.Deps
		inst.Status = epaxosproto.COMMITTED
	} else {
		r.InstanceSpace[commit.Replica][commit.Instance] = &Instance{
			commit.Replica,
			nil,
			0,
			epaxosproto.COMMITTED,
			commit.Seq,
			commit.Deps,
			nil, 0, 0, nil, 0}

		if commit.Count == 0 {
			//checkpoint
			//update latest checkpoint info
			r.latestCPReplica = commit.Replica
			r.latestCPInstance = commit.Instance

			//discard dependency hashtables
			r.clearHashtables()
		}
	}
	r.updateCommitted(commit.Replica)

	r.recordInstanceMetadata(r.InstanceSpace[commit.Replica][commit.Instance])
}

/**********************************************************************

                      RECOVERY ACTIONS

***********************************************************************/

func (r *Replica) BeTheLeader(args *genericsmrproto.BeTheLeaderArgs, reply *genericsmrproto.BeTheLeaderReply) error {
	r.Mutex.Lock()
	r.IsLeader = true
	log.Println("I am the leader")
	time.Sleep(5 * time.Second) // wait that the connection is actually lost
	r.Mutex.Unlock()
	return nil
}

func (r *Replica) startRecoveryForInstance(replica int32, instance int32) {
	var nildeps []int32

	dlog.Printf("Recovering %d.%d", replica, instance)

	if r.InstanceSpace[replica][instance] == nil {
		dlog.Println("Creating instance")
		r.InstanceSpace[replica][instance] = &Instance{replica, nil, 0, epaxosproto.NONE, 0, nildeps, nil, 0, 0, nil, 0}
	}

	inst := r.InstanceSpace[replica][instance]

	if inst.lb == nil {
		inst.lb = &LeaderBookkeeping{nil, -1, 0, false, 0, 0, 0, nildeps, nil, nil, true, false, nil, 0}

	} else {
		inst.lb = &LeaderBookkeeping{inst.lb.clientProposals, -1, 0, false, 0, 0, 0, nildeps, nil, nil, true, false, nil, 0}
	}

	if inst.Status >= epaxosproto.PREACCEPTED { // FIXME
		inst.lb.recoveryInst = &RecoveryInstance{inst.Cmds, inst.Status, inst.Seq, inst.Deps, 1, r.Id == replica}
	}

	//compute larger ballot
	inst.ballot = r.makeBallotLargerThan(inst.ballot)

	r.bcastPrepare(replica, instance, inst.ballot)
}

func (r *Replica) handlePrepare(prepare *epaxosproto.Prepare) {
	inst := r.InstanceSpace[prepare.Replica][prepare.Instance]
	var preply *epaxosproto.PrepareReply
	var nildeps []int32

	if inst == nil {
		dlog.Println("Never heard of")
		r.InstanceSpace[prepare.Replica][prepare.Instance] = &Instance{
			prepare.Replica,
			nil,
			prepare.Ballot,
			epaxosproto.NONE,
			0,
			nildeps,
			nil, 0, 0, nil, 0}
		preply = &epaxosproto.PrepareReply{
			r.Id,
			prepare.Replica,
			prepare.Instance,
			TRUE,
			-1, // FIXME not inline w. TLA spec. (0)
			epaxosproto.NONE,
			nil,
			-1,
			nildeps}
		inst = r.InstanceSpace[prepare.Replica][prepare.Instance]
	} else {
		ok := TRUE
		if prepare.Ballot < inst.ballot {
			ok = FALSE
		} else {
			inst.ballot = prepare.Ballot
		}
		preply = &epaxosproto.PrepareReply{ // FIXME not inline w. TLA spec. (prev_ballot is missing)
			r.Id,
			prepare.Replica,
			prepare.Instance,
			ok,
			inst.ballot,
			inst.Status,
			inst.Cmds,
			inst.Seq,
			inst.Deps}
	}
	r.replyPrepare(prepare.LeaderId, preply)
}

func (r *Replica) handlePrepareReply(preply *epaxosproto.PrepareReply) {
	inst := r.InstanceSpace[preply.Replica][preply.Instance]

	if inst.lb == nil || !inst.lb.preparing {
		dlog.Println("Delayed reply")
		// we've moved on -- these are delayed replies, so just ignore
		// TODO: should replies for non-current ballots be ignored?
		return
	}

	if preply.OK == FALSE {
		dlog.Println("Another active leader")
		// TODO: there is probably another active leader, back off and retry later
		inst.lb.nacks++
		return
	}

	//Got an ACK (preply.OK == TRUE)

	inst.lb.prepareOKs++

	if preply.Status == epaxosproto.COMMITTED || preply.Status == epaxosproto.EXECUTED {
		r.InstanceSpace[preply.Replica][preply.Instance] = &Instance{
			preply.Replica,
			preply.Command,
			inst.ballot,
			epaxosproto.COMMITTED,
			preply.Seq,
			preply.Deps,
			nil, 0, 0, nil, 0}
		r.bcastCommit(preply.Replica, preply.Instance, inst.Cmds, preply.Seq, preply.Deps)
		r.Mutex.Lock()
		r.Stats.M["totalCommitTime"] += int(time.Now().UnixNano() - inst.proposeTime)
		r.Mutex.Unlock()
		//TODO: check if we should send notifications to clients
		dlog.Println("Already committed")
		return
	}

	if preply.Status == epaxosproto.ACCEPTED {
		if inst.lb.recoveryInst == nil || inst.lb.maxRecvBallot < preply.Ballot {
			inst.lb.recoveryInst = &RecoveryInstance{preply.Command, preply.Status, preply.Seq, preply.Deps, 0, false}
			inst.lb.maxRecvBallot = preply.Ballot
		}
	}

	if (preply.Status == epaxosproto.PREACCEPTED || preply.Status == epaxosproto.PREACCEPTED_EQ) &&
		(inst.lb.recoveryInst == nil || inst.lb.recoveryInst.status < epaxosproto.ACCEPTED) {
		if inst.lb.recoveryInst == nil {
			inst.lb.recoveryInst = &RecoveryInstance{preply.Command, preply.Status, preply.Seq, preply.Deps, 1, false}
		} else if preply.Seq == inst.Seq && equal(preply.Deps, inst.Deps) {
			inst.lb.recoveryInst.preAcceptCount++
		} else if preply.Status == epaxosproto.PREACCEPTED_EQ {
			// If we get different ordering attributes from pre-acceptors, we must go with the ones
			// that agreed with the initial command leader (in case we do not use Thrifty).
			// This is safe if we use thrifty, although we can also safely start phase 1 in that case.
			inst.lb.recoveryInst = &RecoveryInstance{preply.Command, preply.Status, preply.Seq, preply.Deps, 1, false}
		}
		if preply.AcceptorId == preply.Replica {
			//if the reply is from the initial command leader, then it's safe to restart phase 1
			inst.lb.recoveryInst.leaderResponded = true
		}
	}

	if inst.lb.prepareOKs < r.N/2 {
		dlog.Println("Not enough")
		return
	}

	//Received Prepare replies from a majority

	ir := inst.lb.recoveryInst
	// inst.Cmds = ir.cmds // FIXME

	if ir != nil {
		//at least one replica has (pre-)accepted this instance
		if ir.status == epaxosproto.ACCEPTED ||
			(!ir.leaderResponded && ir.preAcceptCount >= r.N/2 && (r.Thrifty || ir.status == epaxosproto.PREACCEPTED_EQ)) {
			//safe to go to Accept phase
			inst.Cmds = ir.cmds
			inst.Seq = ir.seq
			inst.Deps = ir.deps
			inst.Status = epaxosproto.ACCEPTED
			inst.lb.preparing = false
			r.bcastAccept(preply.Replica, preply.Instance, inst.ballot, int32(len(inst.Cmds)), inst.Seq, inst.Deps)
		} else if !ir.leaderResponded && ir.preAcceptCount >= (r.N/2+1)/2 {
			//send TryPreAccepts
			//but first try to pre-accept on the local replica
			inst.lb.preAcceptOKs = 0
			inst.lb.nacks = 0
			inst.lb.possibleQuorum = make([]bool, r.N)
			for q := 0; q < r.N; q++ {
				inst.lb.possibleQuorum[q] = true
			}
			if conf, q, i := r.findPreAcceptConflicts(ir.cmds, preply.Replica, preply.Instance, ir.seq, ir.deps); conf {
				if r.InstanceSpace[q][i].Status >= epaxosproto.COMMITTED {
					//start Phase1 in the initial leader's instance
					dlog.Println("Starting phase 1")
					r.startPhase1(preply.Replica, preply.Instance, inst.ballot, inst.lb.clientProposals, ir.cmds, len(ir.cmds))
					return
				} else {
					inst.lb.nacks = 1
					inst.lb.possibleQuorum[r.Id] = false
				}
			} else {
				inst.Cmds = ir.cmds
				inst.Seq = ir.seq
				inst.Deps = ir.deps
				inst.Status = epaxosproto.PREACCEPTED
				inst.lb.preAcceptOKs = 1
			}
			inst.lb.preparing = false
			inst.lb.tryingToPreAccept = true
			r.bcastTryPreAccept(preply.Replica, preply.Instance, inst.ballot, inst.Cmds, inst.Seq, inst.Deps)
		} else {
			//start Phase1 in the initial leader's instance
			inst.lb.preparing = false
			r.startPhase1(preply.Replica, preply.Instance, inst.ballot, inst.lb.clientProposals, ir.cmds, len(ir.cmds))
		}
	} else {
		//try to finalize instance by proposing NO-OP
		noop_deps := make([]int32, r.N)
		// commands that depended on this instance must look at all previous instances
		noop_deps[preply.Replica] = preply.Instance - 1
		inst.lb.preparing = false
		r.InstanceSpace[preply.Replica][preply.Instance] = &Instance{
			preply.Replica,
			state.NOOP(),
			inst.ballot,
			epaxosproto.ACCEPTED,
			0,
			noop_deps,
			inst.lb, 0, 0, nil, 0}
		dlog.Println("Bcasting accept")
		r.bcastAccept(preply.Replica, preply.Instance, inst.ballot, 0, 0, noop_deps)
	}
}

func (r *Replica) handleTryPreAccept(tpa *epaxosproto.TryPreAccept) {
	inst := r.InstanceSpace[tpa.Replica][tpa.Instance]
	if inst != nil && inst.ballot > tpa.Ballot {
		// ballot number too small
		r.replyTryPreAccept(tpa.LeaderId, &epaxosproto.TryPreAcceptReply{
			r.Id,
			tpa.Replica,
			tpa.Instance,
			FALSE,
			inst.ballot,
			tpa.Replica,
			tpa.Instance,
			inst.Status})
	}
	if conflict, confRep, confInst := r.findPreAcceptConflicts(tpa.Command, tpa.Replica, tpa.Instance, tpa.Seq, tpa.Deps); conflict {
		// there is a conflict, can't pre-accept
		r.replyTryPreAccept(tpa.LeaderId, &epaxosproto.TryPreAcceptReply{
			r.Id,
			tpa.Replica,
			tpa.Instance,
			FALSE,
			inst.ballot,
			confRep,
			confInst,
			r.InstanceSpace[confRep][confInst].Status})
	} else {
		// can pre-accept
		if tpa.Instance >= r.crtInstance[tpa.Replica] {
			r.crtInstance[tpa.Replica] = tpa.Instance + 1
		}
		if inst != nil {
			inst.Cmds = tpa.Command
			inst.Deps = tpa.Deps
			inst.Seq = tpa.Seq
			inst.Status = epaxosproto.PREACCEPTED
			inst.ballot = tpa.Ballot
		} else {
			r.InstanceSpace[tpa.Replica][tpa.Instance] = &Instance{
				tpa.Replica,
				tpa.Command,
				tpa.Ballot,
				epaxosproto.PREACCEPTED,
				tpa.Seq,
				tpa.Deps,
				nil, 0, 0,
				nil, 0}
		}
		r.replyTryPreAccept(tpa.LeaderId, &epaxosproto.TryPreAcceptReply{r.Id, tpa.Replica, tpa.Instance, TRUE, inst.ballot, 0, 0, 0})
	}
}

func (r *Replica) findPreAcceptConflicts(cmds []state.Command, replica int32, instance int32, seq int32, deps []int32) (bool, int32, int32) {
	inst := r.InstanceSpace[replica][instance]
	if inst != nil && len(inst.Cmds) > 0 {
		if inst.Status >= epaxosproto.ACCEPTED {
			// already ACCEPTED or COMMITTED
			// we consider this a conflict because we shouldn't regress to PRE-ACCEPTED
			return true, replica, instance
		}
		if inst.Seq == seq && equal(inst.Deps, deps) {
			// already PRE-ACCEPTED, no point looking for conflicts again
			return false, replica, instance
		}
	}
	for q := int32(0); q < int32(r.N); q++ {
		for i := r.ExecedUpTo[q]; i < r.crtInstance[q]; i++ { // FIXME this is not enough imho.
			// dlog.Printf("Checking %d.%d \n",q,i)
			if i == -1 {
				//do not check placeholder
				continue
			}
			if replica == q && instance == i {
				// no point checking past instance in replica's row, since replica would have
				// set the dependencies correctly for anything started after instance
				break
			}
			if i == deps[q] {
				//the instance cannot be a dependency for itself
				continue
			}
			inst := r.InstanceSpace[q][i]
			if inst == nil || inst.Cmds == nil || len(inst.Cmds) == 0 {
				continue
			}
			if inst.Deps[replica] >= instance {
				// instance q.i depends on instance replica.instance, it is not a conflict
				continue
			}
			if r.LRead || state.ConflictBatch(inst.Cmds, cmds) {
				if i > deps[q] ||
					(i < deps[q] && inst.Seq >= seq && (q != replica || inst.Status > epaxosproto.PREACCEPTED_EQ)) {
					// this is a conflict
					return true, q, i
				}
			}
		}
	}
	return false, -1, -1
}

func (r *Replica) handleTryPreAcceptReply(tpar *epaxosproto.TryPreAcceptReply) {
	inst := r.InstanceSpace[tpar.Replica][tpar.Instance]
	if inst == nil || inst.lb == nil || !inst.lb.tryingToPreAccept || inst.lb.recoveryInst == nil {
		dlog.Printf("Nothing to do %d.%d\n", tpar.Replica, tpar.Instance)
		return
	}

	ir := inst.lb.recoveryInst

	if tpar.OK == TRUE {
		inst.lb.preAcceptOKs++
		inst.lb.tpaOKs++
		if inst.lb.preAcceptOKs >= r.N/2 {
			//it's safe to start Accept phase
			inst.Cmds = ir.cmds
			inst.Seq = ir.seq
			inst.Deps = ir.deps
			inst.Status = epaxosproto.ACCEPTED
			inst.lb.tryingToPreAccept = false
			inst.lb.acceptOKs = 0
			r.bcastAccept(tpar.Replica, tpar.Instance, inst.ballot, int32(len(inst.Cmds)), inst.Seq, inst.Deps)
			return
		}
	} else {
		inst.lb.nacks++
		if tpar.Ballot > inst.ballot {
			//TODO: retry with higher ballot
			dlog.Printf("Retry %d.%d\n", tpar.Replica, tpar.Instance)
			return
		}
		inst.lb.tpaOKs++
		if tpar.ConflictReplica == tpar.Replica && tpar.ConflictInstance == tpar.Instance {
			//TODO: re-run prepare
			inst.lb.tryingToPreAccept = false
			dlog.Printf("Re-run %d.%d\n", tpar.Replica, tpar.Instance)
			return
		}
		inst.lb.possibleQuorum[tpar.AcceptorId] = false
		inst.lb.possibleQuorum[tpar.ConflictReplica] = false
		notInQuorum := 0
		for q := 0; q < r.N; q++ {
			if !inst.lb.possibleQuorum[tpar.AcceptorId] {
				notInQuorum++
			}
		}
		if tpar.ConflictStatus >= epaxosproto.COMMITTED || notInQuorum > r.N/2 {
			//abandon recovery, restart from phase 1
			inst.lb.tryingToPreAccept = false
			r.startPhase1(tpar.Replica, tpar.Instance, inst.ballot, inst.lb.clientProposals, ir.cmds, len(ir.cmds))
		}
		if notInQuorum == r.N/2 {
			//this is to prevent defer cycles
			if present, dq, _ := deferredByInstance(tpar.Replica, tpar.Instance); present {
				if inst.lb.possibleQuorum[dq] {
					//an instance whose leader must have been in this instance's quorum has been deferred for this instance => contradiction
					//abandon recovery, restart from phase 1
					inst.lb.tryingToPreAccept = false
					r.startPhase1(tpar.Replica, tpar.Instance, inst.ballot, inst.lb.clientProposals, ir.cmds, len(ir.cmds))
				}
			}
		}
		if inst.lb.tpaOKs >= r.N/2 {
			//defer recovery and update deferred information
			updateDeferred(tpar.Replica, tpar.Instance, tpar.ConflictReplica, tpar.ConflictInstance)
			inst.lb.tryingToPreAccept = false
		}
	}
}

//helper functions and structures to prevent defer cycles while recovering

var deferMap map[uint64]uint64 = make(map[uint64]uint64)

func updateDeferred(dr int32, di int32, r int32, i int32) {
	daux := (uint64(dr) << 32) | uint64(di)
	aux := (uint64(r) << 32) | uint64(i)
	deferMap[aux] = daux
}

func deferredByInstance(q int32, i int32) (bool, int32, int32) {
	aux := (uint64(q) << 32) | uint64(i)
	daux, present := deferMap[aux]
	if !present {
		return false, 0, 0
	}
	dq := int32(daux >> 32)
	di := int32(daux)
	return true, dq, di
}

func (r *Replica) fastQuorumSize() int {
	return r.N/2 + (r.N/2+1)/2
}
