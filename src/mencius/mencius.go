package mencius

import (
	"dlog"
	"encoding/binary"
	"fastrpc"
	"genericsmr"
	"genericsmrproto"
	"io"
	"log"
	"menciusproto"
	"state"
	"time"
)

const CHAN_BUFFER_SIZE = 200000
const WAIT_BEFORE_SKIP_MS = 50
const NB_INST_TO_SKIP = 100000
const MAX_SKIPS_WAITING = 20
const TRUE = uint8(1)
const FALSE = uint8(0)

type Replica struct {
	*genericsmr.Replica      // extends a generic Paxos replica
	skipChan                 chan fastrpc.Serializable
	prepareChan              chan fastrpc.Serializable
	acceptChan               chan fastrpc.Serializable
	commitChan               chan fastrpc.Serializable
	prepareReplyChan         chan fastrpc.Serializable
	acceptReplyChan          chan fastrpc.Serializable
	delayedSkipChan          chan *DelayedSkip
	skipRPC                  uint8
	prepareRPC               uint8
	acceptRPC                uint8
	commitRPC                uint8
	prepareReplyRPC          uint8
	acceptReplyRPC           uint8
	clockChan                chan bool   // clock
	instanceSpace            []*Instance // the space of all instances (used and not yet used)
	crtInstance              int32       // highest active instance number that this replica knows about
	latestInstReady          int32       // highest instance number that is in the READY state (ready to commit)
	latestInstCommitted      int32       // highest instance number (owned by the current replica) that was committed
	blockingInstance         int32       // the lowest instance that could block commits
	noCommitFor              int
	waitingToCommitSomething bool
	Shutdown                 bool
	skipsWaiting             int
	counter                  int
	skippedTo                []int32
}

type DelayedSkip struct {
	skipEnd int32
}

type InstanceStatus int

const (
	PREPARING InstanceStatus = iota
	ACCEPTED
	READY
	COMMITTED
	EXECUTED
)

type Instance struct {
	skipped       bool
	nbInstSkipped int
	command       *state.Command
	ballot        int32
	status        InstanceStatus
	lb            *LeaderBookkeeping
}

type LeaderBookkeeping struct {
	clientProposal *genericsmr.Propose
	maxRecvBallot  int32
	prepareOKs     int
	acceptOKs      int
	nacks          int
}

func NewReplica(id int, peerAddrList []string, thrifty bool, exec bool, dreply bool, durable bool) *Replica {
	skippedTo := make([]int32, len(peerAddrList))
	for i := 0; i < len(skippedTo); i++ {
		skippedTo[i] = -1
	}
	r := &Replica{genericsmr.NewReplica(id, peerAddrList, thrifty, exec, dreply),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE*4),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE),
		make(chan fastrpc.Serializable, genericsmr.CHAN_BUFFER_SIZE*4),
		make(chan *DelayedSkip, genericsmr.CHAN_BUFFER_SIZE),
		0, 0, 0, 0, 0, 0,
		make(chan bool, 10),
		make([]*Instance, 10*1024*1024),
		int32(id),
		int32(-1),
		int32(0),
		int32(0),
		0,
		false,
		false,
		0,
		0,
		skippedTo}

	r.Durable = durable

	r.skipRPC = r.RegisterRPC(new(menciusproto.Skip), r.skipChan)
	r.prepareRPC = r.RegisterRPC(new(menciusproto.Prepare), r.prepareChan)
	r.acceptRPC = r.RegisterRPC(new(menciusproto.Accept), r.acceptChan)
	r.commitRPC = r.RegisterRPC(new(menciusproto.Commit), r.commitChan)
	r.prepareReplyRPC = r.RegisterRPC(new(menciusproto.PrepareReply), r.prepareReplyChan)
	r.acceptReplyRPC = r.RegisterRPC(new(menciusproto.AcceptReply), r.acceptReplyChan)

	go r.run()

	return r
}

//append a log entry to stable storage
func (r *Replica) recordInstanceMetadata(inst *Instance) {
	if !r.Durable {
		return
	}

	var b [10]byte
	if inst.skipped {
		b[0] = 1
	} else {
		b[0] = 0
	}
	binary.LittleEndian.PutUint32(b[1:5], uint32(inst.nbInstSkipped))
	binary.LittleEndian.PutUint32(b[5:9], uint32(inst.ballot))
	b[9] = byte(inst.status)
	r.StableStore.Write(b[:])
}

//write a sequence of commands to stable storage
func (r *Replica) recordCommand(cmd *state.Command) {
	if !r.Durable {
		return
	}

	if cmd == nil {
		return
	}
	cmd.Marshal(io.Writer(r.StableStore))
}

//sync with the stable store
func (r *Replica) sync() {
	if !r.Durable {
		return
	}

	r.StableStore.Sync()
}

func (r *Replica) replyPrepare(replicaId int32, reply *menciusproto.PrepareReply) {
	r.SendMsg(replicaId, r.prepareReplyRPC, reply)
}

func (r *Replica) replyAccept(replicaId int32, reply *menciusproto.AcceptReply) {
	r.SendMsg(replicaId, r.acceptReplyRPC, reply)
}

/* ============= */

/* Main event processing loop */
var lastSeenInstance int32

func (r *Replica) run() {
	r.ConnectToPeers()

	dlog.Println("Waiting for client connections")

	go r.WaitForClientConnections()

	if r.Exec {
		go r.executeCommands()
	}

	go r.clock()

	for !r.Shutdown {

		select {

		case propose := <-r.ProposeChan:
			//got a Propose from a client
			dlog.Printf("Proposal with id %d\n", propose.CommandId)
			r.handlePropose(propose)
			break

		case skipS := <-r.skipChan:
			skip := skipS.(*menciusproto.Skip)
			//got a Skip from another replica
			dlog.Printf("Skip for instances %d-%d\n", skip.StartInstance, skip.EndInstance)
			r.handleSkip(skip)

		case prepareS := <-r.prepareChan:
			prepare := prepareS.(*menciusproto.Prepare)
			//got a Prepare message
			dlog.Printf("Received Prepare from replica %d, for instance %d\n", prepare.LeaderId, prepare.Instance)
			r.handlePrepare(prepare)
			break

		case acceptS := <-r.acceptChan:
			accept := acceptS.(*menciusproto.Accept)
			//got an Accept message
			dlog.Printf("Received Accept from replica %d, for instance %d\n", accept.LeaderId, accept.Instance)
			r.handleAccept(accept)
			break

		case commitS := <-r.commitChan:
			commit := commitS.(*menciusproto.Commit)
			//got a Commit message
			dlog.Printf("Received Commit from replica %d, for instance %d\n", commit.LeaderId, commit.Instance)
			r.handleCommit(commit)
			break

		case prepareReplyS := <-r.prepareReplyChan:
			prepareReply := prepareReplyS.(*menciusproto.PrepareReply)
			//got a Prepare reply
			dlog.Printf("Received PrepareReply for instance %d\n", prepareReply.Instance)
			r.handlePrepareReply(prepareReply)
			break

		case acceptReplyS := <-r.acceptReplyChan:
			acceptReply := acceptReplyS.(*menciusproto.AcceptReply)
			//got an Accept reply
			dlog.Printf("Received AcceptReply for instance %d\n", acceptReply.Instance)
			r.handleAcceptReply(acceptReply)
			break

		case delayedSkip := <-r.delayedSkipChan:
			r.handleDelayedSkip(delayedSkip)
			break

		case <-r.clockChan:
			if lastSeenInstance == r.blockingInstance {
				r.noCommitFor++
			} else {
				r.noCommitFor = 0
				lastSeenInstance = r.blockingInstance
			}
			if r.noCommitFor >= 50+int(r.Id) && r.crtInstance >= r.blockingInstance+int32(r.N) {
				r.noCommitFor = 0
				dlog.Printf("Doing force commit\n")
				r.forceCommit()
			}
			break
		}
	}
}

func (r *Replica) clock() {
	for !r.Shutdown {
		time.Sleep(100 * 1000 * 1000)
		r.clockChan <- true
	}
}

func (r *Replica) makeUniqueBallot(ballot int32) int32 {
	return (ballot << 4) | r.Id
}

func (r *Replica) makeBallotLargerThan(ballot int32) int32 {
	return r.makeUniqueBallot((ballot >> 4) + 1)
}

var sk menciusproto.Skip

func (r *Replica) bcastSkip(startInstance int32, endInstance int32, exceptReplica int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Skip bcast failed:", err)
		}
	}()
	sk.LeaderId = r.Id
	sk.StartInstance = startInstance
	sk.EndInstance = endInstance
	args := &sk
	//args := &menciusproto.Skip{r.Id, startInstance, endInstance}

	n := r.N - 1
	q := r.Id

	for sent := 0; sent < n; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] || q == exceptReplica {
			continue
		}
		sent++
		r.SendMsgNoFlush(q, r.skipRPC, args)
	}
}

func (r *Replica) bcastPrepare(instance int32, ballot int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Prepare bcast failed:", err)
		}
	}()
	args := &menciusproto.Prepare{r.Id, instance, ballot}

	n := r.N - 1
	if r.Thrifty {
		n = r.N >> 1
	}
	q := r.Id

	for sent := 0; sent < n; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		sent++
		r.SendMsg(q, r.prepareRPC, args)
	}
}

var ma menciusproto.Accept

func (r *Replica) bcastAccept(instance int32, ballot int32, skip uint8, nbInstToSkip int32, command state.Command) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Accept bcast failed:", err)
		}
	}()
	ma.LeaderId = r.Id
	ma.Instance = instance
	ma.Ballot = ballot
	ma.Skip = skip
	ma.NbInstancesToSkip = nbInstToSkip
	ma.Command = command
	args := &ma
	//args := &menciusproto.Accept{r.Id, instance, ballot, skip, nbInstToSkip, command}

	n := r.N - 1
	q := r.Id

	sent := 0
	for sent < n {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		if r.Thrifty {
			inst := (instance/int32(r.N))*int32(r.N) + q
			if inst > instance {
				inst -= int32(r.N)
			}
			if inst < 0 || r.instanceSpace[inst] != nil {
				continue
			}
		}
		sent++
		r.SendMsg(q, r.acceptRPC, args)
	}

	for sent < r.N>>1 {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		if r.Thrifty {
			inst := (instance/int32(r.N))*int32(r.N) + q
			if inst > instance {
				inst -= int32(r.N)
			}
			if inst >= 0 && r.instanceSpace[inst] == nil {
				continue
			}
		}
		sent++
		r.SendMsg(q, r.acceptRPC, args)
	}
}

var mc menciusproto.Commit

func (r *Replica) bcastCommit(instance int32, skip uint8, nbInstToSkip int32, command state.Command) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Commit bcast failed:", err)
		}
	}()
	mc.LeaderId = r.Id
	mc.Instance = instance
	mc.Skip = skip
	mc.NbInstancesToSkip = nbInstToSkip
	//mc.Command = command
	//args := &menciusproto.Commit{r.Id, instance, skip, nbInstToSkip, command}
	args := &mc

	n := r.N - 1
	q := r.Id

	for sent := 0; sent < n; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		sent++
		r.SendMsg(q, r.commitRPC, args)
	}
}

func (r *Replica) handlePropose(propose *genericsmr.Propose) {

	instNo := r.crtInstance
	r.crtInstance += int32(r.N)

	r.instanceSpace[instNo] = &Instance{false,
		0,
		&propose.Command,
		r.makeBallotLargerThan(0),
		ACCEPTED,
		&LeaderBookkeeping{propose, 0, 0, 0, 0}}

	r.recordInstanceMetadata(r.instanceSpace[instNo])
	r.recordCommand(&propose.Command)
	r.sync()

	r.bcastAccept(instNo, r.instanceSpace[instNo].ballot, FALSE, 0, propose.Command)
	dlog.Printf("Choosing req. %d in instance %d\n", propose.CommandId, instNo)
}

func (r *Replica) handleSkip(skip *menciusproto.Skip) {
	r.instanceSpace[skip.StartInstance] = &Instance{true,
		int(skip.EndInstance-skip.StartInstance)/r.N + 1,
		nil,
		0,
		COMMITTED,
		nil}
	r.updateBlocking(skip.StartInstance)
}

func (r *Replica) handlePrepare(prepare *menciusproto.Prepare) {
	inst := r.instanceSpace[prepare.Instance]

	if inst == nil {
		dlog.Println("Replying OK to null-instance Prepare")
		r.replyPrepare(prepare.LeaderId, &menciusproto.PrepareReply{prepare.Instance,
			TRUE,
			-1,
			FALSE,
			0,
			state.Command{state.NONE, 0, 0}})

		r.instanceSpace[prepare.Instance] = &Instance{false,
			0,
			nil,
			prepare.Ballot,
			PREPARING,
			nil}
	} else {
		ok := TRUE
		if prepare.Ballot < inst.ballot {
			ok = FALSE
		}
		if inst.command == nil {
			inst.command = &state.Command{state.NONE, 0, 0}
		}
		skipped := FALSE
		if inst.skipped {
			skipped = TRUE
		}
		r.replyPrepare(prepare.LeaderId, &menciusproto.PrepareReply{prepare.Instance,
			ok,
			inst.ballot,
			skipped,
			int32(inst.nbInstSkipped),
			*inst.command})
	}
}

func (r *Replica) timerHelper(ds *DelayedSkip) {
	time.Sleep(WAIT_BEFORE_SKIP_MS * 1000 * 1000)
	r.delayedSkipChan <- ds
}

func (r *Replica) handleAccept(accept *menciusproto.Accept) {
	flush := true
	inst := r.instanceSpace[accept.Instance]

	if inst != nil && inst.ballot > accept.Ballot {
		r.replyAccept(accept.LeaderId, &menciusproto.AcceptReply{accept.Instance, FALSE, inst.ballot, -1, -1})
		return
	}

	skipStart := int32(-1)
	skipEnd := int32(-1)
	if accept.Skip == FALSE && r.crtInstance < accept.Instance {
		skipStart = r.crtInstance
		skipEnd = accept.Instance/int32(r.N)*int32(r.N) + r.Id
		if skipEnd > accept.Instance {
			skipEnd -= int32(r.N)
		}
		if r.skipsWaiting < MAX_SKIPS_WAITING {
			//start a timer, waiting for a propose to arrive and fill this hole
			go r.timerHelper(&DelayedSkip{skipEnd})
			//r.delayedSkipChan <- &DelayedSkip{accept, skipStart}
			r.skipsWaiting++
			flush = false
		}
		r.instanceSpace[r.crtInstance] = &Instance{true,
			int(skipEnd-r.crtInstance)/r.N + 1,
			nil,
			-1,
			COMMITTED,
			nil}

		r.recordInstanceMetadata(r.instanceSpace[r.crtInstance])
		r.sync()

		r.crtInstance = skipEnd + int32(r.N)
	}
	if inst == nil {
		skip := false
		if accept.Skip == TRUE {
			skip = true
		}
		r.instanceSpace[accept.Instance] = &Instance{skip,
			int(accept.NbInstancesToSkip),
			&accept.Command,
			accept.Ballot,
			ACCEPTED,
			nil}
		r.recordInstanceMetadata(r.instanceSpace[accept.Instance])
		r.recordCommand(&accept.Command)
		r.sync()

		r.replyAccept(accept.LeaderId, &menciusproto.AcceptReply{accept.Instance, TRUE, -1, skipStart, skipEnd})
	} else {
		if inst.status == COMMITTED || inst.status == EXECUTED {
			if inst.command == nil {
				inst.command = &accept.Command
			}
			dlog.Printf("ATTENTION! Reordered Commit\n")
		} else {
			inst.command = &accept.Command
			inst.ballot = accept.Ballot
			inst.status = ACCEPTED
			inst.skipped = false
			if accept.Skip == TRUE {
				inst.skipped = true
			}
			inst.nbInstSkipped = int(accept.NbInstancesToSkip)

			r.recordInstanceMetadata(inst)

			r.replyAccept(accept.LeaderId, &menciusproto.AcceptReply{accept.Instance, TRUE, inst.ballot, skipStart, skipEnd})
		}
	}
	if skipStart >= 0 {
		dlog.Printf("Skipping!!\n")
		r.bcastSkip(skipStart, skipEnd, accept.LeaderId)
		r.updateBlocking(skipStart)
		if flush {
			for _, w := range r.PeerWriters {
				if w != nil {
					w.Flush()
				}
			}
		}
	} else {
		r.updateBlocking(accept.Instance)
	}
}

func (r *Replica) handleDelayedSkip(delayedSkip *DelayedSkip) {
	r.skipsWaiting--
	for _, w := range r.PeerWriters {
		if w != nil {
			w.Flush()
		}
	}
}

func (r *Replica) handleCommit(commit *menciusproto.Commit) {
	inst := r.instanceSpace[commit.Instance]

	dlog.Printf("Committing instance %d\n", commit.Instance)

	if inst == nil {
		skip := false
		if commit.Skip == TRUE {
			skip = true
		}
		r.instanceSpace[commit.Instance] = &Instance{skip,
			int(commit.NbInstancesToSkip),
			nil, //&commit.Command,
			0,
			COMMITTED,
			nil}
	} else {
		//inst.command = &commit.Command
		inst.status = COMMITTED
		inst.skipped = false
		if commit.Skip == TRUE {
			inst.skipped = true
		}
		inst.nbInstSkipped = int(commit.NbInstancesToSkip)
		if inst.lb != nil && inst.lb.clientProposal != nil {
			// try command in the next available instance
			r.ProposeChan <- inst.lb.clientProposal
			inst.lb.clientProposal = nil
		}
	}

	r.recordInstanceMetadata(r.instanceSpace[commit.Instance])

	if commit.Instance%int32(r.N) == r.Id%int32(r.N) {
		if r.crtInstance < commit.Instance+commit.NbInstancesToSkip*int32(r.N) {
			r.crtInstance = commit.Instance + commit.NbInstancesToSkip*int32(r.N)
		}
	}

	// Try to commit instances waiting for this one
	r.updateBlocking(commit.Instance)
}

func (r *Replica) handlePrepareReply(preply *menciusproto.PrepareReply) {
	dlog.Printf("PrepareReply for instance %d\n", preply.Instance)

	inst := r.instanceSpace[preply.Instance]

	if inst.status != PREPARING {
		// we've moved on -- these are delayed replies, so just ignore
		return
	}

	if preply.OK == TRUE {
		inst.lb.prepareOKs++

		if preply.Ballot > inst.lb.maxRecvBallot {
			inst.command = &preply.Command
			inst.skipped = false
			if preply.Skip == TRUE {
				inst.skipped = true
			}
			inst.nbInstSkipped = int(preply.NbInstancesToSkip)
			inst.lb.maxRecvBallot = preply.Ballot
		}

		if inst.lb.prepareOKs+1 > r.N>>1 {
			inst.status = ACCEPTED
			inst.lb.nacks = 0
			skip := FALSE
			if inst.skipped {
				skip = TRUE
			}
			r.bcastAccept(preply.Instance, inst.ballot, skip, int32(inst.nbInstSkipped), *inst.command)
		}
	} else {
		// TODO: there is probably another active leader
		inst.lb.nacks++
		if preply.Ballot > inst.lb.maxRecvBallot {
			inst.lb.maxRecvBallot = preply.Ballot
		}
		if inst.lb.nacks >= r.N>>1 && inst.lb != nil {
			// TODO: better to wait a while
			// some other replica is trying to commit skips for our instance
			// increase ballot number and try again
			inst.ballot = r.makeBallotLargerThan(inst.lb.maxRecvBallot)
			r.bcastPrepare(preply.Instance, inst.ballot)
		}
	}
}

func (r *Replica) handleAcceptReply(areply *menciusproto.AcceptReply) {
	dlog.Printf("AcceptReply for instance %d\n", areply.Instance)

	inst := r.instanceSpace[areply.Instance]

	if areply.OK == TRUE {
		inst.lb.acceptOKs++
		if areply.SkippedStartInstance > -1 {
			r.instanceSpace[areply.SkippedStartInstance] = &Instance{true,
				int(areply.SkippedEndInstance-areply.SkippedStartInstance)/r.N + 1,
				nil,
				0,
				COMMITTED,
				nil}
			r.updateBlocking(areply.SkippedStartInstance)
		}

		if inst.status == COMMITTED || inst.status == EXECUTED { //TODO || aargs.Ballot != inst.ballot {
			// we've moved on, these are delayed replies, so just ignore
			return
		}

		if inst.lb.acceptOKs+1 > r.N>>1 {
			if inst.skipped {
				//TODO what if
			}
			inst.status = READY
			if !inst.skipped && areply.Instance > r.latestInstReady {
				r.latestInstReady = areply.Instance
			}
			r.updateBlocking(areply.Instance)
		}
	} else {
		// TODO: there is probably another active leader
		inst.lb.nacks++
		if areply.Ballot > inst.lb.maxRecvBallot {
			inst.lb.maxRecvBallot = areply.Ballot
		}
		if (areply.Ballot&0x0F)%int32(r.N) == areply.Instance%int32(r.N) {
			// the owner of the instance is trying to commit something, I should give up
		}
		if inst.lb.nacks >= r.N>>1 {
			// TODO
			if inst.lb.clientProposal != nil {
				// I'm the owner of the instance, I'll try again with a higher ballot number
				inst.ballot = r.makeBallotLargerThan(inst.lb.maxRecvBallot)
				r.bcastPrepare(areply.Instance, inst.ballot)
			}
		}
	}
}

func (r *Replica) updateBlocking(instance int32) {
	if instance != r.blockingInstance {
		return
	}

	for r.blockingInstance = r.blockingInstance; true; r.blockingInstance++ {
		if r.blockingInstance <= r.skippedTo[int(r.blockingInstance)%r.N] {
			continue
		}
		if r.instanceSpace[r.blockingInstance] == nil {
			return
		}
		inst := r.instanceSpace[r.blockingInstance]
		if inst.status == COMMITTED && inst.skipped {
			r.skippedTo[int(r.blockingInstance)%r.N] = r.blockingInstance + int32((inst.nbInstSkipped-1)*r.N)
			continue
		}
		if inst.status == ACCEPTED && inst.skipped {
			return
		}
		if r.blockingInstance%int32(r.N) == r.Id || inst.lb != nil {
			if inst.status == READY {
				//commit my instance
				dlog.Printf("Am about to commit instance %d\n", r.blockingInstance)

				inst.status = COMMITTED
				if inst.lb.clientProposal != nil && !r.Dreply {
					// give client the all clear
					dlog.Printf("Sending ACK for req. %d\n", inst.lb.clientProposal.CommandId)
					r.ReplyProposeTS(&genericsmrproto.ProposeReplyTS{TRUE, inst.lb.clientProposal.CommandId, state.NIL, inst.lb.clientProposal.Timestamp},
						inst.lb.clientProposal.Reply)
				}
				skip := FALSE
				if inst.skipped {
					skip = TRUE
				}

				r.recordInstanceMetadata(inst)
				r.sync()

				r.bcastCommit(r.blockingInstance, skip, int32(inst.nbInstSkipped), *inst.command)
			} else if inst.status != COMMITTED && inst.status != EXECUTED {
				return
			}
			if inst.skipped {
				r.skippedTo[int(r.blockingInstance)%r.N] = r.blockingInstance + int32((inst.nbInstSkipped-1)*r.N)
			}
		} else {
			if inst.status == PREPARING || (inst.status == ACCEPTED && inst.skipped) {
				return
			}
		}
	}
}

func (r *Replica) executeCommands() {
	execedUpTo := int32(-1)
	skippedTo := make([]int32, r.N)
	skippedToOrig := make([]int32, r.N)
	conflicts := make(map[state.Key]int32, 60000)

	for q := 0; q < r.N; q++ {
		skippedToOrig[q] = -1
	}

	for !r.Shutdown {
		executed := false
		jump := false
		copy(skippedTo, skippedToOrig)
		for i := execedUpTo + 1; i < r.crtInstance; i++ {
			if i < skippedTo[i%int32(r.N)] {
				continue
			}

			if r.instanceSpace[i] == nil {
				break
			}

			if r.instanceSpace[i].status == EXECUTED {
				continue
			}

			if r.instanceSpace[i].status != COMMITTED {
				if !r.instanceSpace[i].skipped {
					confInst, present := conflicts[r.instanceSpace[i].command.K]
					if present && r.instanceSpace[confInst].status != EXECUTED {
						break
					}
					conflicts[r.instanceSpace[i].command.K] = i
					jump = true
					continue
				} else {
					break
				}
			}

			if r.instanceSpace[i].skipped {
				skippedTo[i%int32(r.N)] = i + int32(r.instanceSpace[i].nbInstSkipped*r.N)
				if !jump {
					skippedToOrig[i%int32(r.N)] = skippedTo[i%int32(r.N)]
				}
				continue
			}

			inst := r.instanceSpace[i]
			for inst.command == nil {
				time.Sleep(1000 * 1000)
			}
			confInst, present := conflicts[inst.command.K]
			if present && confInst < i && r.instanceSpace[confInst].status != EXECUTED && state.Conflict(r.instanceSpace[confInst].command, inst.command) {
				break
			}

			inst.command.Execute(r.State)

			if r.Dreply && inst.lb != nil && inst.lb.clientProposal != nil {
				dlog.Printf("Sending ACK for req. %d\n", inst.lb.clientProposal.CommandId)
				r.ReplyProposeTS(&genericsmrproto.ProposeReplyTS{TRUE, inst.lb.clientProposal.CommandId, state.NIL, inst.lb.clientProposal.Timestamp},
					inst.lb.clientProposal.Reply)
			}
			inst.status = EXECUTED

			executed = true

			if !jump {
				execedUpTo = i
			}
		}
		if !executed {
			time.Sleep(1000 * 1000)
		}
	}
}

func (r *Replica) forceCommit() {
	//find what is the oldest un-initialized instance and try to take over
	problemInstance := r.blockingInstance

	//try to take over the problem instance
	if int(problemInstance)%r.N == int(r.Id+1)%r.N {
		log.Println("Replica", r.Id, "Trying to take over instance", problemInstance)
		if r.instanceSpace[problemInstance] == nil {
			r.instanceSpace[problemInstance] = &Instance{true,
				NB_INST_TO_SKIP,
				&state.Command{state.NONE, 0, 0},
				r.makeUniqueBallot(1),
				PREPARING,
				&LeaderBookkeeping{nil, 0, 0, 0, 0}}
			r.bcastPrepare(problemInstance, r.instanceSpace[problemInstance].ballot)
		} else {
			log.Println("Not nil")
		}
	}
}
