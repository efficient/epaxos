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

type Replica struct {
	*genericsmr.Replica // extends a generic Paxos replica
	prepareChan         chan fastrpc.Serializable
	acceptChan          chan fastrpc.Serializable
	commitChan          chan fastrpc.Serializable
	commitShortChan     chan fastrpc.Serializable
	prepareReplyChan chan fastrpc.Serializable
	acceptReplyChan  chan fastrpc.Serializable
	prepareRPC       uint8
	acceptRPC        uint8
	commitRPC        uint8
	commitShortRPC   uint8
	prepareReplyRPC  uint8
	acceptReplyRPC   uint8
	IsLeader         bool        // does this replica think it is the leader
	instanceSpace    []*Instance // the space of all instances (used and not yet used)
	crtInstance      int32       // highest active instance number that this replica knows about
	currentBallot    int32       // current ballot
	lastTriedBallot  int32       // highest ballot tried so far
	maxRecvBallot    int32
	Shutdown         bool
	counter          int
	flush            bool
	committedUpTo    int32
}

type InstanceStatus int

const (
	PREPARING InstanceStatus = iota
	PREPARED
	ACCEPTED
	COMMITTED
)

type Instance struct {
	cmds   []state.Command
	ballot int32
	status InstanceStatus
	lb     *LeaderBookkeeping
}

type LeaderBookkeeping struct {
	clientProposals []*genericsmr.Propose
	prepareOKs      int
	acceptOKs       int
	nacks           int
	ballot          int32 // highest ballot at which a command was accepted
	cmds            []state.Command // the accepted command
}

func NewReplica(id int, peerAddrList []string, Isleader bool, thrifty bool, exec bool, lread bool, dreply bool, durable bool) *Replica {
	r := &Replica{genericsmr.NewReplica(id, peerAddrList, thrifty, exec, lread, dreply),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, 3*genericsmr.CHAN_BUFFER_SIZE),
		0, 0, 0, 0, 0, 0,
		false,
		make([]*Instance, 15*1024*1024),
		0,
		-1,
		-1,
		-1,
		false,
		0,
		true,
		-1}

	r.Durable = durable

	if Isleader{
		r.BeTheLeader(nil,nil)
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
	binary.LittleEndian.PutUint32(b[0:4], uint32(inst.ballot))
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
	log.Println("I am the Paxos leader")
	r.makeBallot()
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
		}

	}
}

func (r *Replica) makeBallot() {
	n := int32(0)
	for n <= r.maxRecvBallot && n <= r.lastTriedBallot {
		n += int32(r.N)
	}
	r.Mutex.Lock()
	r.lastTriedBallot =  n + r.Id
	r.Mutex.Unlock()
	dlog.Printf("Last tried ballot is %d\n", r.lastTriedBallot)
}

func (r *Replica) updateCommittedUpTo() {
	for r.instanceSpace[r.committedUpTo+1] != nil &&
		r.instanceSpace[r.committedUpTo+1].status == COMMITTED {
		r.committedUpTo++
	}
	dlog.Printf("Committed up to %d",r.committedUpTo)
}

func (r *Replica) bcastPrepare(instance int32, ballot int32) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Prepare bcast failed:", err)
		}
	}()

	args := &paxosproto.Prepare{r.Id, instance, ballot}

	n := r.N - 1
	if r.Thrifty {
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

func (r *Replica) bcastAccept(instance int32, ballot int32, command []state.Command) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Accept bcast failed:", err)
		}
	}()
	pa.LeaderId = r.Id
	pa.Instance = instance
	pa.Ballot = ballot
	pa.Command = command
	args := &pa
	//args := &paxosproto.Accept{r.Id, instance, ballot, command}

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

	for r.instanceSpace[r.crtInstance] != nil {
		r.crtInstance++
	}

	instNo := r.crtInstance
	r.crtInstance++

	batchSize := len(r.ProposeChan) + 1

	if batchSize > MAX_BATCH {
		batchSize = MAX_BATCH
	}

	dlog.Printf("Batched %d\n", batchSize)

	cmds := make([]state.Command, batchSize)
	proposals := make([]*genericsmr.Propose, batchSize)
	cmds[0] = propose.Command
	proposals[0] = propose

	for i := 1; i < batchSize; i++ {
		prop := <-r.ProposeChan
		cmds[i] = prop.Command
		proposals[i] = prop
	}

	r.instanceSpace[instNo] = &Instance{
		cmds,
		r.lastTriedBallot,
		PREPARING,
		&LeaderBookkeeping{proposals, 0, 0, 0, r.lastTriedBallot, cmds}}

	if r.currentBallot != r.lastTriedBallot {
		dlog.Printf("Classic round for instance %d (%d,%d)\n", instNo, r.lastTriedBallot, r.currentBallot)
		r.bcastPrepare(instNo, r.lastTriedBallot)
	} else {
		dlog.Printf("Fast round for instance %d (%d)\n", instNo, r.lastTriedBallot)
		r.instanceSpace[instNo].status = PREPARED
		r.bcastAccept(instNo, r.lastTriedBallot, cmds)
		r.recordInstanceMetadata(r.instanceSpace[instNo])
		r.recordCommands(cmds)
		r.sync()
	}

}

func (r *Replica) handlePrepare(prepare *paxosproto.Prepare) {
	inst := r.instanceSpace[prepare.Instance]
	var preply *paxosproto.PrepareReply

	if prepare.Ballot> r.maxRecvBallot {
		r.maxRecvBallot = prepare.Ballot
	}

	if r.currentBallot > prepare.Ballot {
		dlog.Printf("Cannot join ballot %d < %d",prepare.Ballot, r.currentBallot)
	} else if r.currentBallot < prepare.Ballot {
		dlog.Printf("Joining ballot %d ",prepare.Ballot)
		r.currentBallot = prepare.Ballot
	}

	if inst == nil {
		preply = &paxosproto.PrepareReply{prepare.Instance, r.currentBallot, 0, nil}
	} else {
		preply = &paxosproto.PrepareReply{prepare.Instance,  r.currentBallot, inst.ballot, inst.cmds}
	}

	r.replyPrepare(prepare.LeaderId, preply)

}

func (r *Replica) handleAccept(accept *paxosproto.Accept) {
	inst := r.instanceSpace[accept.Instance]
	var areply *paxosproto.AcceptReply

	if accept.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = areply.Ballot
	}

	if accept.Ballot != r.currentBallot {
		dlog.Printf("Accept msg not for current ballot %d < %d\n", accept.Ballot, r.currentBallot)
		areply = &paxosproto.AcceptReply{accept.Instance, r.currentBallot}
	} else if inst != nil && inst.status == COMMITTED {
		dlog.Printf("Already committed instance %d\n", accept.Instance)
		areply = &paxosproto.AcceptReply{accept.Instance, r.currentBallot}
	} else {
		if inst == nil {
			r.instanceSpace[accept.Instance] = &Instance{
				accept.Command,
				r.currentBallot,
				ACCEPTED,
				nil}
			areply = &paxosproto.AcceptReply{accept.Instance, r.currentBallot}
		} else {
			inst.cmds = accept.Command
			inst.ballot = r.currentBallot
			inst.status = ACCEPTED
			areply = &paxosproto.AcceptReply{accept.Instance, r.currentBallot}
		}
		r.recordInstanceMetadata(r.instanceSpace[accept.Instance])
		r.recordCommands(accept.Command)
		r.sync()
	}

	r.replyAccept(accept.LeaderId, areply)

}

func (r *Replica) handleCommit(commit *paxosproto.Commit) {
	inst := r.instanceSpace[commit.Instance]

	if inst != nil && inst.status == COMMITTED {
		dlog.Printf("Already committed instance %d\n", commit.Instance)
		return
	}

	dlog.Printf("Committing instance %d\n", commit.Instance)
	r.instanceSpace[commit.Instance] = &Instance{
		commit.Command,
		commit.Ballot,
		COMMITTED,
		nil}

	r.updateCommittedUpTo()

	r.recordInstanceMetadata(r.instanceSpace[commit.Instance])
	r.recordCommands(commit.Command)

}

func (r *Replica) handleCommitShort(commit *paxosproto.CommitShort) {
	inst := r.instanceSpace[commit.Instance]

	if inst != nil && inst.status == COMMITTED {
		dlog.Printf("Already committed instance %d\n", commit.Instance)
		return
	}

	if inst == nil {
		dlog.Printf("Commit short received but nothing recorded %d\n", commit.Instance)
		return
	}

	dlog.Printf("Committing instance %d\n", commit.Instance)

	r.instanceSpace[commit.Instance].status = COMMITTED
	r.instanceSpace[commit.Instance].ballot = commit.Ballot

	r.updateCommittedUpTo()
	r.recordInstanceMetadata(r.instanceSpace[commit.Instance])
	r.recordCommands(r.instanceSpace[commit.Instance].cmds)
}

func (r *Replica) handlePrepareReply(preply *paxosproto.PrepareReply) {
	inst := r.instanceSpace[preply.Instance]

	if preply.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = preply.Ballot
	}

	if inst.status != PREPARING {
		dlog.Printf("Not preparing %d", preply.Instance)
		return
	}

	if preply.Ballot != r.lastTriedBallot{
		dlog.Printf("There is another active leader (%d,%d) \n",preply.Ballot, r.lastTriedBallot)
		inst.lb.nacks++
	}

	if preply.Command != nil && preply.LBallot > inst.lb.ballot {
		inst.lb.ballot = preply.LBallot
		inst.lb.cmds = preply.Command
	}

	inst.lb.prepareOKs++
	if inst.lb.prepareOKs+1 > r.N>>1 {
		if r.lastTriedBallot > r.currentBallot {
			r.currentBallot = r.lastTriedBallot
		}
		inst.status = PREPARED
		inst.lb.nacks = 0
		r.recordInstanceMetadata(r.instanceSpace[preply.Instance])
		r.sync()
		r.bcastAccept(preply.Instance, inst.ballot, inst.cmds)
	}

}

func (r *Replica) handleAcceptReply(areply *paxosproto.AcceptReply) {
	inst := r.instanceSpace[areply.Instance]

	if areply.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = areply.Ballot
	}

	if inst.status != PREPARED && inst.status != ACCEPTED {
		// we've move on, these are delayed replies, so just ignore
		return
	}

	if areply.Ballot != r.lastTriedBallot{
		dlog.Printf("There is another active leader (%d,%d) \n",areply.Ballot, r.lastTriedBallot)
		inst.lb.nacks++
	}

	inst.lb.acceptOKs++
	if inst.lb.acceptOKs+1 > r.N>>1 {
		inst = r.instanceSpace[areply.Instance]
		inst.status = COMMITTED
		r.bcastCommit(areply.Instance, inst.ballot, inst.cmds)
		if inst.lb.clientProposals != nil && !r.Dreply {
			// give client the all clear
			for i := 0; i < len(inst.cmds); i++ {
				propreply := &genericsmrproto.ProposeReplyTS{
					TRUE,
					inst.lb.clientProposals[i].CommandId,
					state.NIL(),
					inst.lb.clientProposals[i].Timestamp}
				r.ReplyProposeTS(propreply, inst.lb.clientProposals[i].Reply, inst.lb.clientProposals[i].Mutex)
			}
		}

		r.recordInstanceMetadata(r.instanceSpace[areply.Instance])
		r.sync() //is this necessary?

		r.updateCommittedUpTo()
	}

}

func (r *Replica) executeCommands() {
	i := int32(0)
	for !r.Shutdown {
		executed := false

		// FIXME idempotence
		for i <= r.committedUpTo {
			if r.instanceSpace[i].cmds != nil {
				inst := r.instanceSpace[i]
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
				i++
				executed = true
			 } else {
				dlog.Printf("Retrieving instance %d",i)
				r.bcastPrepare(i, r.lastTriedBallot)
				break
			}
		}

		if !executed {
			time.Sleep(1000 * 1000)
		}
	}

}
