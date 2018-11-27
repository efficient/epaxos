package paxos

import (
	"dlog"
	"encoding/binary"
	"fastrpc"
	"genericsmr"
	"genericsmrproto"
	"io"
	"log"
	"paxosproto"
	"state"
	"time"
)

const CHAN_BUFFER_SIZE = 200000
const TRUE = uint8(1)
const FALSE = uint8(0)

const MAX_BATCH = 1

const COMMIT_GRACE_PERIOD = 100 * 1e9 // 1 second(s)
const SLEEP_TIME_NS = 1e6

type Replica struct {
	*genericsmr.Replica // extends a generic Paxos replica
	prepareChan         chan fastrpc.Serializable
	acceptChan          chan fastrpc.Serializable
	commitChan          chan fastrpc.Serializable
	commitShortChan     chan fastrpc.Serializable
	prepareReplyChan chan fastrpc.Serializable
	acceptReplyChan  chan fastrpc.Serializable
	instancesToRecover   chan int32
	prepareRPC      uint8
	acceptRPC       uint8
	commitRPC       uint8
	commitShortRPC  uint8
	prepareReplyRPC uint8
	acceptReplyRPC  uint8
	IsLeader        bool        // does this replica think it is the leader
	instanceSpace   []*Instance // the space of all instances (used and not yet used)
	crtInstance     int32       // highest active instance number that this replica knows about
	maxRecvBallot   int32
	defaultBallot   []int32
	smallestDefaultBallot int32
	Shutdown        bool
	counter         int
	flush           bool
	executedUpTo    int32
}

type InstanceStatus int

const (
	PREPARING InstanceStatus = iota
	PREPARED
	ACCEPTED
	COMMITTED
)

type Instance struct {
	cmds    []state.Command
	cballot int32
	lballot int32
	status  InstanceStatus
	lb      *LeaderBookkeeping
}

type LeaderBookkeeping struct {
	clientProposals []*genericsmr.Propose
	prepareOKs      int
	acceptOKs       int
	nacks           int
	ballot          int32 // highest ballot at which a command was accepted
	cmds            []state.Command // the accepted command
	lastTriedBallot int32         // highest ballot tried so far
}

func NewReplica(id int, peerAddrList []string, Isleader bool, thrifty bool, exec bool, lread bool, dreply bool, durable bool) *Replica {
	r := &Replica{genericsmr.NewReplica(id, peerAddrList, thrifty, exec, lread, dreply),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, 3*genericsmr.CHAN_BUFFER_SIZE),
		make(chan int32, 3*genericsmr.CHAN_BUFFER_SIZE),
		0, 0, 0, 0, 0, 0,
		false,
		make([]*Instance, 15*1024*1024),
		-1,
		-1,
		make([]int32, len(peerAddrList)),
		-1,
		false,
		0,
		true,
		-1}

	r.Durable = durable

	if Isleader{
		r.BeTheLeader(nil,nil)
	}

	for i:=0; i<len(r.defaultBallot); i++{
		r.defaultBallot[i]=-1
	}

	r.prepareRPC = r.RegisterRPC(new(paxosproto.Prepare), r.prepareChan)
	r.acceptRPC = r.RegisterRPC(new(paxosproto.Accept), r.acceptChan)
	r.commitRPC = r.RegisterRPC(new(paxosproto.Commit), r.commitChan)
	r.commitShortRPC = r.RegisterRPC(new(paxosproto.CommitShort), r.commitShortChan)
	r.prepareReplyRPC = r.RegisterRPC(new(paxosproto.PrepareReply), r.prepareReplyChan)
	r.acceptReplyRPC = r.RegisterRPC(new(paxosproto.AcceptReply), r.acceptReplyChan)

	go r.run()

	return r
}

//append a log entry to stable storage
func (r *Replica) recordInstanceMetadata(inst *Instance) {
	if !r.Durable {
		return
	}

	var b [5]byte
	binary.LittleEndian.PutUint32(b[0:4], uint32(inst.cballot))
	b[4] = byte(inst.status)
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

/* RPC to be called by master */

func (r *Replica) BeTheLeader(args *genericsmrproto.BeTheLeaderArgs, reply *genericsmrproto.BeTheLeaderReply) error {
	r.IsLeader = true
	log.Println("I am the leader")
	return nil
}

func (r *Replica) replyPrepare(replicaId int32, reply *paxosproto.PrepareReply) {
	r.SendMsg(replicaId, r.prepareReplyRPC, reply)
}

func (r *Replica) replyAccept(replicaId int32, reply *paxosproto.AcceptReply) {
	r.SendMsg(replicaId, r.acceptReplyRPC, reply)
}

/* ============= */

var clockChan chan bool

func (r *Replica) clock() {
	for !r.Shutdown {
		time.Sleep(1000 * 1000 * 5)
		clockChan <- true
	}
}

/* Main event processing loop */

func (r *Replica) run() {

	r.ConnectToPeers()

	r.ComputeClosestPeers()

	if r.Exec {
		go r.executeCommands()
	}

	clockChan = make(chan bool, 1)
	go r.clock()
	onOffProposeChan := r.ProposeChan

	go r.WaitForClientConnections()

	for !r.Shutdown {

		select {

		case <-clockChan:
			//activate the new proposals channel
			onOffProposeChan = r.ProposeChan
			break

		case propose := <- onOffProposeChan:
			//got a Propose from a client
			dlog.Printf("Received proposal with type=%d\n", propose.Command.Op)
			r.handlePropose(propose)
			//deactivate the new proposals channel to prioritize the handling of protocol messages
			if MAX_BATCH > 100 {
				onOffProposeChan = nil
			}
			break

		case prepareS := <-r.prepareChan:
			prepare := prepareS.(*paxosproto.Prepare)
			//got a Prepare message
			dlog.Printf("Received Prepare from replica %d, for instance %d\n", prepare.LeaderId, prepare.Instance)
			r.handlePrepare(prepare)
			break

		case acceptS := <-r.acceptChan:
			accept := acceptS.(*paxosproto.Accept)
			//got an Accept message
			dlog.Printf("Received Accept from replica %d, for instance %d\n", accept.LeaderId, accept.Instance)
			r.handleAccept(accept)
			break

		case commitS := <-r.commitChan:
			commit := commitS.(*paxosproto.Commit)
			//got a Commit message
			dlog.Printf("Received Commit from replica %d, for instance %d\n", commit.LeaderId, commit.Instance)
			r.handleCommit(commit)
			break

		case commitS := <-r.commitShortChan:
			commit := commitS.(*paxosproto.CommitShort)
			//got a Commit message
			dlog.Printf("Received Commit from replica %d, for instance %d\n", commit.LeaderId, commit.Instance)
			r.handleCommitShort(commit)
			break

		case prepareReplyS := <-r.prepareReplyChan:
			prepareReply := prepareReplyS.(*paxosproto.PrepareReply)
			//got a Prepare reply
			dlog.Printf("Received PrepareReply for instance %d\n", prepareReply.Instance)
			r.handlePrepareReply(prepareReply)
			break

		case acceptReplyS := <-r.acceptReplyChan:
			acceptReply := acceptReplyS.(*paxosproto.AcceptReply)
			//got an Accept reply
			dlog.Printf("Received AcceptReply for instance %d\n", acceptReply.Instance)
			r.handleAcceptReply(acceptReply)
			break

		case iid := <-r.instancesToRecover:
			r.recover(iid)
			break
		}

	}
}

func (r *Replica) makeBallot(instance int32) {
	n := int32(0)
	r.Mutex.Lock()
	lb := r.instanceSpace[instance].lb
	for n <= r.maxRecvBallot && n <= lb.lastTriedBallot {
		n += int32(r.N)
	}
	lb.lastTriedBallot =  n + r.Id
	r.Mutex.Unlock()
	dlog.Printf("Last tried ballot is %d in %d\n", lb.lastTriedBallot, instance)
}

func (r *Replica) bcastPrepare(instance int32) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Prepare bcast failed:", err)
		}
	}()

	args := &paxosproto.Prepare{r.Id, instance, r.instanceSpace[instance].lb.lastTriedBallot}

	n := r.N - 1
	if r.Thrifty  {
		n = r.N >> 1
	}

	sent := 0
	for q := 0; q < r.N-1; q++ {
		if !r.Alive[r.PreferredPeerOrder[q]] {
			continue
		}
		r.SendMsg(r.PreferredPeerOrder[q], r.prepareRPC, args)
		sent++
		if sent >= n {
			break
		}
	}

}

var pa paxosproto.Accept

func (r *Replica) bcastAccept(instance int32) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Accept bcast failed:", err)
		}
	}()
	pa.LeaderId = r.Id
	pa.Instance = instance
	pa.Ballot = r.instanceSpace[instance].lb.lastTriedBallot
	pa.Command = r.instanceSpace[instance].lb.cmds
	args := &pa

	n := r.N - 1
	if r.Thrifty {
		n = r.N >> 1
	}

	sent := 0
	for q := 0; q < r.N-1; q++ {
		if !r.Alive[r.PreferredPeerOrder[q]] {
			continue
		}
		r.SendMsg(r.PreferredPeerOrder[q], r.acceptRPC, args)
		sent++
		if sent >= n {
			break
		}
	}

}

var pc paxosproto.Commit
var pcs paxosproto.CommitShort

func (r *Replica) bcastCommit(instance int32, ballot int32, command []state.Command) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Commit bcast failed:", err)
		}
	}()
	pc.LeaderId = r.Id
	pc.Instance = instance
	pc.Ballot = ballot
	pc.Command = command
	args := &pc
	pcs.LeaderId = r.Id
	pcs.Instance = instance
	pcs.Ballot = ballot
	pcs.Count = int32(len(command))
	argsShort := &pcs

	sent := 0
	for q := 0; q < r.N-1; q++ {
		if !r.Alive[r.PreferredPeerOrder[q]] {
			continue
		}
		if sent < (r.N >> 1) {
			r.SendMsg(r.PreferredPeerOrder[q], r.commitShortRPC, argsShort)
		}else{
			r.SendMsg(r.PreferredPeerOrder[q], r.commitRPC, args)
		}
		sent++
	}

}

func (r *Replica) handlePropose(propose *genericsmr.Propose) {
	if !r.IsLeader {
		dlog.Printf("Not the leader, cannot propose %v\n", propose.CommandId)
		preply := &genericsmrproto.ProposeReplyTS{FALSE, -1, state.NIL(), 0}
		r.ReplyProposeTS(preply, propose.Reply, propose.Mutex)
		return
	}

	batchSize := len(r.ProposeChan) + 1

	if batchSize > MAX_BATCH {
		batchSize = MAX_BATCH
	}

	dlog.Printf("Batched %d\n", batchSize)

	proposals := make([]*genericsmr.Propose, batchSize)
	proposals[0] = propose
	for i := 1; i < batchSize; i++ {
		prop := <-r.ProposeChan
		proposals[i] = prop
	}

	r.crtInstance++
	r.instanceSpace[r.crtInstance] = &Instance{
		nil,
		r.defaultBallot[r.Id],
		r.defaultBallot[r.Id],
		PREPARING,
		&LeaderBookkeeping{proposals, 0, 0, 0, -1, nil, -1}}
	r.makeBallot(r.crtInstance)

	r.defaultBallot[r.Id] = r.instanceSpace[r.crtInstance].lb.lastTriedBallot

	if r.instanceSpace[r.crtInstance].lb.lastTriedBallot != r.smallestDefaultBallot {
		dlog.Printf("Classic round for instance %d\n", r.crtInstance)
		r.bcastPrepare(r.crtInstance)
	}else{
		dlog.Printf("Fast round for instance %d\n", r.crtInstance)
		r.bcastAccept(r.crtInstance)
	}

}

func (r *Replica) handlePrepare(prepare *paxosproto.Prepare) {

	if prepare.Ballot> r.maxRecvBallot {
		r.maxRecvBallot = prepare.Ballot
	}

	inst := r.instanceSpace[prepare.Instance]
	if inst == nil {
		if prepare.Instance > r.crtInstance{
			r.crtInstance = prepare.Instance
		}
		r.instanceSpace[prepare.Instance] = &Instance{
			nil,
			r.defaultBallot[r.Id],
			r.defaultBallot[r.Id],
			PREPARING,
			nil}
		inst = r.instanceSpace[prepare.Instance]
	}

	if inst.status == COMMITTED {
		dlog.Printf("Already committed \n")
		var pc paxosproto.Commit
		pc.LeaderId = prepare.LeaderId
		pc.Instance = prepare.Instance
		pc.Ballot = inst.lballot
		pc.Command = inst.cmds
		args := &pc
		r.SendMsg(pc.LeaderId, r.commitRPC, args)
		return
	}

	if inst.cballot > prepare.Ballot {
		dlog.Printf("Joined higher ballot %d < %d", prepare.Ballot, inst.cballot)
	}else if inst.cballot < prepare.Ballot {
			dlog.Printf("Joining ballot %d ", prepare.Ballot)
			inst.cballot = prepare.Ballot
			inst.status = PREPARED
			if r.crtInstance == prepare.Instance{
				r.defaultBallot[r.Id] = prepare.Ballot
			}
	}else{
		// msg reordering
		dlog.Printf("Ballot %d already joined", prepare.Ballot)
	}

	preply := &paxosproto.PrepareReply{prepare.Instance,  inst.cballot, inst.lballot, r.defaultBallot[r.Id], r.Id, inst.cmds}
	r.replyPrepare(prepare.LeaderId, preply)

}

func (r *Replica) handleAccept(accept *paxosproto.Accept) {
	inst := r.instanceSpace[accept.Instance]

	if accept.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = accept.Ballot
	}

	if inst == nil {
		if accept.Instance > r.crtInstance{
			r.crtInstance = accept.Instance
		}
		r.instanceSpace[accept.Instance] = &Instance{
			accept.Command,
			accept.Ballot,
			accept.Ballot,
			ACCEPTED,
			nil}
		inst = r.instanceSpace[accept.Instance]
		r.recordInstanceMetadata(r.instanceSpace[accept.Instance])
		r.recordCommands(accept.Command)
		r.sync()
	} else if accept.Ballot < inst.cballot {
		dlog.Printf("Smaller ballot %d < %d\n", accept.Ballot, inst.cballot)
	} else if inst.status == COMMITTED {
		dlog.Printf("Already committed \n")
	} else {
		inst.cmds = accept.Command
		inst.cballot = accept.Ballot
		inst.lballot = accept.Ballot
		inst.status = ACCEPTED
		r.recordInstanceMetadata(r.instanceSpace[accept.Instance])
		r.recordCommands(accept.Command)
		r.sync()
	}

	areply := &paxosproto.AcceptReply{accept.Instance, inst.cballot}
	r.replyAccept(accept.LeaderId, areply)

}

func (r *Replica) handleCommit(commit *paxosproto.Commit) {
	inst := r.instanceSpace[commit.Instance]
	if inst == nil{
		if commit.Instance > r.crtInstance{
			r.crtInstance = commit.Instance
		}
		r.instanceSpace[commit.Instance] = &Instance{
			nil,
			r.defaultBallot[r.Id],
			r.defaultBallot[r.Id],
			PREPARING,
			nil}
		inst = r.instanceSpace[commit.Instance]
	}

	if inst != nil && inst.status == COMMITTED {
		dlog.Printf("Already committed \n")
		return
	}

	if commit.Ballot < inst.cballot {
		dlog.Printf("Smaller ballot %d < %d\n", commit.Ballot, inst.cballot)
		return
	}

	dlog.Printf("Committing \n")
	inst.cmds = commit.Command
	inst.cballot = commit.Ballot
	inst.lballot = commit.Ballot
	inst.status = COMMITTED
	r.recordInstanceMetadata(r.instanceSpace[commit.Instance])
	r.recordCommands(commit.Command)
}

func (r *Replica) handleCommitShort(commit *paxosproto.CommitShort) {
	inst := r.instanceSpace[commit.Instance]
	if inst == nil {
		dlog.Printf("Commit short received but nothing recorded \n")
		return
	}

	if inst != nil && inst.status == COMMITTED {
		dlog.Printf("Already committed \n")
		return
	}

	if commit.Ballot < inst.cballot {
		dlog.Printf("Smaller ballot %d < %d\n", commit.Ballot, inst.cballot)
		return
	}

	dlog.Printf("Committing \n")
	r.instanceSpace[commit.Instance].status = COMMITTED
	r.instanceSpace[commit.Instance].cballot = commit.Ballot
	r.recordInstanceMetadata(r.instanceSpace[commit.Instance])
	r.recordCommands(r.instanceSpace[commit.Instance].cmds)
}

func (r *Replica) handlePrepareReply(preply *paxosproto.PrepareReply) {
	inst := r.instanceSpace[preply.Instance]
	lb := r.instanceSpace[preply.Instance].lb

	if preply.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = preply.Ballot
	}

	if preply.Ballot < lb.lastTriedBallot {
		dlog.Printf("Message in late \n")
		return
	}

	if preply.LBallot > -1 {
		dlog.Printf("Command(s) found \n",)
		lb.ballot = preply.LBallot
		lb.cmds = preply.Command
	}

	if preply.Ballot > lb.lastTriedBallot{
		dlog.Printf("Another active leader using ballot %d \n",preply.Ballot)
		lb.nacks++
		if lb.nacks+1 > r.N>>1{
			if r.IsLeader {
				r.makeBallot(preply.Instance)
				dlog.Printf("Retrying with ballot %d \n",lb.lastTriedBallot)
				r.bcastPrepare(preply.Instance)
			}
		}
		return
	}

	lb.prepareOKs++
	if r.defaultBallot[preply.AcceptorId] < preply.DefaultBallot{
		r.defaultBallot[preply.AcceptorId] = preply.DefaultBallot
		m := int32(-1)
		for _, e := range r.defaultBallot {
			if e < m {
				m = e
			}
		}
		if m > r.smallestDefaultBallot{
			r.smallestDefaultBallot = m
		}
	}

	if lb.prepareOKs+1 > r.N>>1 {
		if lb.clientProposals != nil {
			dlog.Printf("Pushing client proposals")
			cmds := make([]state.Command, len(lb.clientProposals))
			for i := 0; i < len(lb.clientProposals); i++ {
				cmds[i] = lb.clientProposals[i].Command
			}
			lb.cmds = cmds
		} else {
			dlog.Printf("Pushing no-op")
			lb.cmds = state.NOOP()
		}
		inst.cmds = lb.cmds
		inst.cballot = lb.lastTriedBallot
		inst.status = ACCEPTED

		r.recordInstanceMetadata(r.instanceSpace[preply.Instance])
		r.sync()
		r.bcastAccept(preply.Instance)
	}

}

func (r *Replica) handleAcceptReply(areply *paxosproto.AcceptReply) {
	inst := r.instanceSpace[areply.Instance]
	lb := r.instanceSpace[areply.Instance].lb

	if areply.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = areply.Ballot
	}

	if inst.status != PREPARED && inst.status != ACCEPTED {
		// we've move on, these are delayed replies, so just ignore
		return
	}

	if areply.Ballot < lb.lastTriedBallot {
		dlog.Printf("Message in late ")
		return
	}

	if areply.Ballot > lb.lastTriedBallot{
		dlog.Printf("Another active leader using ballot %d \n",areply.Ballot)
		lb.nacks++
		if lb.nacks+1 > r.N>>1{
			if r.IsLeader {
				r.makeBallot(areply.Ballot)
				dlog.Printf("Retrying with ballot %d \n",lb.lastTriedBallot)
				r.bcastPrepare(areply.Instance)
			}
		}
		return
	}

	lb.acceptOKs++
	if lb.acceptOKs+1 > r.N>>1 {
		inst = r.instanceSpace[areply.Instance]
		inst.status = COMMITTED
		r.bcastCommit(areply.Instance, inst.cballot, inst.cmds)
		if lb.clientProposals != nil && !r.Dreply {
			// give client the all clear
			for i := 0; i < len(inst.cmds); i++ {
				propreply := &genericsmrproto.ProposeReplyTS{
					TRUE,
					lb.clientProposals[i].CommandId,
					state.NIL(),
					lb.clientProposals[i].Timestamp}
				r.ReplyProposeTS(propreply, lb.clientProposals[i].Reply, lb.clientProposals[i].Mutex)
			}
		}

		r.recordInstanceMetadata(r.instanceSpace[areply.Instance])
		r.sync() //is this necessary?
	}

}

func (r *Replica) recover(instance int32) {
	if r.instanceSpace[instance].lb == nil{
		r.instanceSpace[instance].lb = &LeaderBookkeeping{nil, 0, 0, 0, -1, nil, -1}
	}
	r.makeBallot(instance)
	r.bcastPrepare(instance)
}

func (r *Replica) executeCommands() {

	timeout := int64(0)
	problemInstance := int32(0)

	for !r.Shutdown {
		executed := false

		// FIXME idempotence
		for i := r.executedUpTo + 1; i <= r.crtInstance; i++ {
			inst := r.instanceSpace[i]
			if inst!= nil && inst.cmds != nil && inst.status == COMMITTED {
				for j := 0; j < len(inst.cmds); j++ {
					dlog.Printf("Executing "+inst.cmds[j].String())
					if r.Dreply && inst.lb != nil && inst.lb.clientProposals != nil {
						val := inst.cmds[j].Execute(r.State)
						propreply := &genericsmrproto.ProposeReplyTS{
							TRUE,
							inst.lb.clientProposals[j].CommandId,
							val,
							inst.lb.clientProposals[j].Timestamp}
						r.ReplyProposeTS(propreply, inst.lb.clientProposals[j].Reply, inst.lb.clientProposals[j].Mutex)
					} else if inst.cmds[j].Op == state.PUT {
						inst.cmds[j].Execute(r.State)
					}
				}
				executed = true
				r.executedUpTo++
				dlog.Printf("Executed up to %d (crtInstance=%d)",r.executedUpTo,r.crtInstance)
			} else {
				if i == problemInstance {
					timeout += SLEEP_TIME_NS
					if (r.IsLeader && timeout >= COMMIT_GRACE_PERIOD) || timeout >= 10*COMMIT_GRACE_PERIOD {
						dlog.Printf("Recovering instance %d \n",i)
						r.instancesToRecover <- problemInstance
						problemInstance = 0
						timeout = 0
					}
				} else {
					problemInstance = i
					timeout = 0
				}
				break
			}
		}

		if !executed {
			time.Sleep(SLEEP_TIME_NS)
		}
	}

}
