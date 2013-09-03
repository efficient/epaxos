package gpaxos

import (
	"bufio"
	"dlog"
	"genericsmr"
	"genericsmrproto"
	"gpaxosproto"
	"log"
	"state"
	"sync"
	"time"
)

const CHAN_BUFFER_SIZE = 200000
const TRUE = uint8(1)
const FALSE = uint8(0)
const CMDS_PER_BALLOT = 40

const ALL_TO_ALL = true

type Replica struct {
	*genericsmr.Replica // extends a generic Paxos replica

	prepareChan chan *gpaxosproto.Prepare // channel for Prepare's

	m1aChan    chan *gpaxosproto.M_1a
	m1bChan    chan *gpaxosproto.M_1b
	m2aChan    chan *gpaxosproto.M_2a
	m2bChan    chan *gpaxosproto.M_2b
	commitChan chan *gpaxosproto.Commit // channel for Commit's

	isLeader bool
	leaderId int32

	fastQSize int

	ballotArray    []*Ballot
	commands       map[int32]*state.Command
	commandsMutex  *sync.Mutex
	committed      map[int32]bool
	commandReplies map[int32]*genericsmr.Propose
	crtBalnum      int32 // highest active balnum
	fastRound      bool  // is this a fast round?
	shutdown       bool
	execedUpTo     int32 // balnum up to which all commands have been executed (including iteslf)
	//conflicts []map[state.Key]int32
	Shutdown bool
}

type Propose struct {
	*genericsmrproto.Propose
	reply *bufio.Writer
}

const (
	PHASE1 uint8 = iota
	PHASE2
	COMMIT
)

type Ballot struct {
	cstruct    []int32
	ballot     int32
	status     uint8
	received2a bool
	lb         *LeaderBookkeeping
}

type LeaderBookkeeping struct {
	maxRecvBallot int32
	prepareOKs    int
	committed     int
	cstructs      [][]int32
}

func NewReplica(id int, peerAddrList []string, thrifty bool, exec bool, dreply bool) *Replica {
	r := &Replica{genericsmr.NewReplica(id, peerAddrList, thrifty, exec, dreply),
		make(chan *gpaxosproto.Prepare, CHAN_BUFFER_SIZE),
		make(chan *gpaxosproto.M_1a, CHAN_BUFFER_SIZE),
		make(chan *gpaxosproto.M_1b, CHAN_BUFFER_SIZE),
		make(chan *gpaxosproto.M_2a, CHAN_BUFFER_SIZE),
		make(chan *gpaxosproto.M_2b, CHAN_BUFFER_SIZE),
		make(chan *gpaxosproto.Commit, CHAN_BUFFER_SIZE),
		false,
		0,
		3,
		make([]*Ballot, 2*1024*1024),
		make(map[int32]*state.Command, 100000),
		new(sync.Mutex),
		make(map[int32]bool, 100000),
		make(map[int32]*genericsmr.Propose, 200),
		-1,
		false,
		false,
		0,
		false}

	r.fastQSize = 3 * r.N / 4
	if r.fastQSize*4 < 3*r.N {
		r.fastQSize++
	}

	go r.run()

	return r
}

/* Inter-replica communication */

func (r *Replica) handleReplicaConnection(rid int, reader *bufio.Reader) error {
	var msgType byte
	var err error
	for !r.Shutdown && err == nil {

		if msgType, err = reader.ReadByte(); err != nil {
			break
		}

		switch uint8(msgType) {

		case gpaxosproto.PREPARE:
			prep := new(gpaxosproto.Prepare)
			if err = prep.Unmarshal(reader); err != nil {
				break
			}
			r.prepareChan <- prep
			break

			/*        case gpaxosproto.PREPARE_REPLY:
			          preply := new(gpaxosproto.PrepareReply)
			          if err = preply.Unmarshal(reader); err != nil {
			             break
			          }
			          r.prepareReplyChan <- preply
			          break*/

		case gpaxosproto.M1A:
			msg := new(gpaxosproto.M_1a)
			if err = msg.Unmarshal(reader); err != nil {
				break
			}
			r.m1aChan <- msg
			break

		case gpaxosproto.M1B:
			msg := new(gpaxosproto.M_1b)
			if err = msg.Unmarshal(reader); err != nil {
				break
			}

			//HACK
			for _, cid := range msg.Cstruct {
				cmd := new(state.Command)
				cmd.Unmarshal(reader)
				r.commandsMutex.Lock()
				if _, present := r.commands[cid]; !present {
					if cmd.Op != 0 || cmd.K != 0 || cmd.V != 0 {
						r.commands[cid] = cmd
					}
				}
				r.commandsMutex.Unlock()
			}

			r.m1bChan <- msg
			break

		case gpaxosproto.M2A:
			msg := new(gpaxosproto.M_2a)
			if err = msg.Unmarshal(reader); err != nil {
				break
			}
			r.m2aChan <- msg
			break

		case gpaxosproto.M2B:
			msg := new(gpaxosproto.M_2b)
			if err = msg.Unmarshal(reader); err != nil {
				break
			}
			//HACK
			for _, cid := range msg.Cids {
				cmd := new(state.Command)
				cmd.Unmarshal(reader)
				r.commandsMutex.Lock()
				if _, present := r.commands[cid]; !present {
					r.commands[cid] = cmd
				}
				r.commandsMutex.Unlock()
			}
			r.m2bChan <- msg
			break

		case gpaxosproto.COMMIT:
			commit := new(gpaxosproto.Commit)
			if err = commit.Unmarshal(reader); err != nil {
				break
			}
			r.commitChan <- commit
			break

		}
	}
	if err != nil {
		log.Println("Error when reading from the connection with replica", rid, err)
		r.Alive[rid] = false
		return err
	}
	return nil
}

func (r *Replica) replyPrepare(reply *gpaxosproto.PrepareReply, w *bufio.Writer) {
	w.WriteByte(gpaxosproto.PREPARE_REPLY)
	reply.Marshal(w)
	w.Flush()
}

func (r *Replica) send1b(msg *gpaxosproto.M_1b, w *bufio.Writer) {
	w.WriteByte(gpaxosproto.M1B)
	msg.Marshal(w)
	dummy := state.Command{0, 0, 0}
	for _, cid := range msg.Cstruct {
		if cmd, present := r.commands[cid]; present {
			cmd.Marshal(w)
		} else {
			dummy.Marshal(w)
		}
	}
	w.Flush()
}

func (r *Replica) send2b(msg *gpaxosproto.M_2b, w *bufio.Writer) {
	w.WriteByte(gpaxosproto.M2B)
	msg.Marshal(w)
	for _, cid := range msg.Cids {
		cmd := r.commands[cid]
		cmd.Marshal(w)
	}
	w.Flush()
}

func (r *Replica) bcast2b(msg *gpaxosproto.M_2b) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Commit bcast failed:", err)
		}
	}()

	for rid, w := range r.PeerWriters {
		if int32(rid) == r.Id {
			continue
		}
		w.WriteByte(gpaxosproto.M2B)
		msg.Marshal(w)
		for _, cid := range msg.Cids {
			cmd := r.commands[cid]
			cmd.Marshal(w)
		}
		w.Flush()
	}
}

var clockChan chan bool

func (r *Replica) clock() {
	for !r.Shutdown {
		time.Sleep(1000 * 1000 * 2)
		clockChan <- true
	}
}

/* ============= */

/* Main event processing loop */

func (r *Replica) run() {
	if r.Id == 0 {
		r.isLeader = true
	}

	r.ConnectToPeersNoListeners()

	for rid, peerReader := range r.PeerReaders {
		if int32(rid) == r.Id {
			continue
		}
		go r.handleReplicaConnection(rid, peerReader)
	}

	dlog.Println("Waiting for client connections")

	go r.WaitForClientConnections()

	/*if r.Exec {
	    go r.executeCommands()
	}*/

	clockChan = make(chan bool, 1)
	go r.clock()

	if r.isLeader {
		r.crtBalnum = 0
		r.fastRound = true
		r.ballotArray[0] = &Ballot{nil, 0, PHASE1, false, &LeaderBookkeeping{cstructs: make([][]int32, 0)}}
		r.ballotArray[0].lb.cstructs = append(r.ballotArray[0].lb.cstructs, make([]int32, 0))
		r.bcast1a(0, true)
	}

	for !r.Shutdown {

		if r.crtBalnum >= 0 && len(r.ballotArray[r.crtBalnum].cstruct) >= CMDS_PER_BALLOT {

			select {

			case prepare := <-r.prepareChan:
				//got a Prepare message
				dlog.Printf("Received Prepare for balnum %d\n", prepare.Balnum)
				r.commandsMutex.Lock()
				r.handlePrepare(prepare)
				r.commandsMutex.Unlock()
				break

			case msg := <-r.m1aChan:
				dlog.Printf("Received 1a for balnum %d @ replica %d\n", msg.Balnum, r.Id)
				r.commandsMutex.Lock()
				r.handle1a(msg)
				r.commandsMutex.Unlock()
				break

			case msg := <-r.m1bChan:
				dlog.Printf("Received 1b for balnum %d @ replica %d\n", msg.Balnum, r.Id)
				r.commandsMutex.Lock()
				r.handle1b(msg)
				r.commandsMutex.Unlock()
				break

			case msg := <-r.m2aChan:
				dlog.Printf("Received 2a for balnum %d @ replica %d\n", msg.Balnum, r.Id)
				r.commandsMutex.Lock()
				r.handle2a(msg)
				r.commandsMutex.Unlock()
				break

			case msg := <-r.m2bChan:
				dlog.Printf("Received 2b for balnum %d @ replica %d\n", msg.Balnum, r.Id)
				r.commandsMutex.Lock()
				r.handle2b(msg)
				r.commandsMutex.Unlock()
				break

			case <-clockChan:
				//way out of deadlock

				select {

				case propose := <-r.ProposeChan:
					//got a Propose from a client
					dlog.Printf("Proposal with id %d @ replica %d\n", propose.CommandId, r.Id)
					r.commandsMutex.Lock()
					r.handlePropose(propose)
					r.commandsMutex.Unlock()
					break

				default:
					break
				}
			}

		} else {

			select {

			case prepare := <-r.prepareChan:
				//got a Prepare message
				dlog.Printf("Received Prepare for balnum %d\n", prepare.Balnum)
				r.commandsMutex.Lock()
				r.handlePrepare(prepare)
				r.commandsMutex.Unlock()
				break

			case msg := <-r.m1aChan:
				dlog.Printf("Received 1a for balnum %d @ replica %d\n", msg.Balnum, r.Id)
				r.commandsMutex.Lock()
				r.handle1a(msg)
				r.commandsMutex.Unlock()
				break

			case msg := <-r.m1bChan:
				dlog.Printf("Received 1b for balnum %d @ replica %d\n", msg.Balnum, r.Id)
				r.commandsMutex.Lock()
				r.handle1b(msg)
				r.commandsMutex.Unlock()
				break

			case msg := <-r.m2aChan:
				dlog.Printf("Received 2a for balnum %d @ replica %d\n", msg.Balnum, r.Id)
				r.commandsMutex.Lock()
				r.handle2a(msg)
				r.commandsMutex.Unlock()
				break

			case msg := <-r.m2bChan:
				dlog.Printf("Received 2b for balnum %d @ replica %d\n", msg.Balnum, r.Id)
				r.commandsMutex.Lock()
				r.handle2b(msg)
				r.commandsMutex.Unlock()
				break

			case propose := <-r.ProposeChan:
				//got a Propose from a client
				dlog.Printf("Proposal with id %d @ replica %d\n", propose.CommandId, r.Id)
				r.commandsMutex.Lock()
				r.handlePropose(propose)
				r.commandsMutex.Unlock()
				break
			}
		}
	}
}

func (r *Replica) makeUniqueBallot(ballot int32) int32 {
	return (ballot << 4) | r.Id
}

func (r *Replica) makeBallotLargerThan(ballot int32) int32 {
	return r.makeUniqueBallot((ballot >> 4) + 1)
}

func (r *Replica) bcastPrepare(replica int32, instance int32, ballot int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Prepare bcast failed:", err)
		}
	}()
	args := &gpaxosproto.Prepare{r.Id, instance, ballot}

	n := r.N - 1
	//TODO: fix quorum size
	if r.Thrifty {
		n = r.N >> 1
	}
	q := r.Id
	var w *bufio.Writer
	for sent := 0; sent < n; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			dlog.Println("Not enough replicas alive!")
			break
		}
		if !r.Alive[q] {
			continue
		}
		sent++
		w = r.PeerWriters[q]
		w.WriteByte(gpaxosproto.PREPARE)
		args.Marshal(w)
		w.Flush()
	}
}

func (r *Replica) bcast1a(balnum int32, fast bool) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("1a bcast failed:", err)
		}
	}()
	args := &gpaxosproto.M_1a{r.Id, balnum, TRUE}
	if !fast {
		args.Fast = FALSE
	}

	n := r.N - 1
	if r.Thrifty {
		if fast {
			n = r.fastQSize - 1
		} else {
			n = r.N >> 1
		}
	}
	q := r.Id
	var w *bufio.Writer
	for sent := 0; sent < n; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		sent++
		w = r.PeerWriters[q]
		w.WriteByte(gpaxosproto.M1A)
		args.Marshal(w)
		w.Flush()
	}
}

func (r *Replica) bcast2a(balnum int32, cstruct []int32, fast bool) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("1a bcast failed:", err)
		}
	}()
	args := &gpaxosproto.M_2a{r.Id, balnum, cstruct}

	n := r.N - 1
	if r.Thrifty {
		if fast {
			n = r.fastQSize - 1
		} else {
			n = r.N >> 1
		}
	}
	q := r.Id
	var w *bufio.Writer
	for sent := 0; sent < n; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		sent++
		w = r.PeerWriters[q]
		w.WriteByte(gpaxosproto.M2A)
		args.Marshal(w)
		w.Flush()
	}
}

func (r *Replica) bcastCommit(cstruct []int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Commit bcast failed:", err)
		}
	}()
	args := &gpaxosproto.Commit{cstruct}

	n := r.N - 1
	q := r.Id
	var w *bufio.Writer
	for sent := 0; sent < n; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		sent++
		w = r.PeerWriters[q]
		w.WriteByte(gpaxosproto.COMMIT)
		args.Marshal(w)
		w.Flush()
	}
}

func (r *Replica) handlePropose(propose *genericsmr.Propose) {
	//TODO!! Handle client retries

	/*    if _, duplicate := r.commands[propose.CommandId]; duplicate {
	      log.Println("Duplicate command from client")
	      return
	  }*/

	if !r.isLeader && r.crtBalnum < 0 {
		log.Println("Received request before leader 1a message")
		r.ReplyProposeTS(&genericsmrproto.ProposeReplyTS{FALSE, -1, state.NIL, propose.Timestamp}, propose.Reply)
		return
	}

	r.commands[propose.CommandId] = &propose.Command

	if _, present := r.committed[propose.CommandId]; present {
		if r.isLeader || ALL_TO_ALL {
			r.ReplyProposeTS(&genericsmrproto.ProposeReplyTS{TRUE, propose.CommandId, state.NIL, propose.Timestamp}, propose.Reply)
		}
		return
	} else {
		r.commandReplies[propose.CommandId] = propose
	}

	crtBallot := r.ballotArray[r.crtBalnum]

	for _, cid := range crtBallot.cstruct {
		if cid == propose.CommandId {
			return
		}
	}

	crtBallot.cstruct = append(crtBallot.cstruct, propose.CommandId)
	crtBallot.lb.cstructs[r.Id] = crtBallot.cstruct

	b := [...]int32{propose.CommandId}

	if r.isLeader {
		if !r.fastRound {
			r.bcast2a(r.crtBalnum, crtBallot.cstruct, false)
		} else {
			if ALL_TO_ALL {
				r.bcast2b(&gpaxosproto.M_2b{r.Id, r.crtBalnum, crtBallot.cstruct, b[:1]})
			}
			r.tryToLearn()
		}
	} else {
		if r.fastRound {
			if crtBallot.received2a {
				if ALL_TO_ALL {
					r.bcast2b(&gpaxosproto.M_2b{r.Id, r.crtBalnum, crtBallot.cstruct, b[:1]})
					r.tryToLearn()
				} else {
					r.send2b(&gpaxosproto.M_2b{r.Id, r.crtBalnum, crtBallot.cstruct, b[:1]}, r.PeerWriters[r.leaderId])
				}
			}
		}
	}
}

func (r *Replica) handlePrepare(prep *gpaxosproto.Prepare) {
}

func (r *Replica) handle1a(msg *gpaxosproto.M_1a) {
	if msg.LeaderId != r.leaderId {
		log.Println("Incorrect leader")
		return
	}

	if msg.Balnum <= r.crtBalnum {
		return
	}

	if r.crtBalnum >= 0 {
		r.send1b(&gpaxosproto.M_1b{r.Id, msg.Balnum, r.ballotArray[r.crtBalnum].cstruct}, r.PeerWriters[r.leaderId])
	} else {
		r.send1b(&gpaxosproto.M_1b{r.Id, msg.Balnum, make([]int32, 0)}, r.PeerWriters[r.leaderId])
	}

	r.crtBalnum = msg.Balnum
	r.ballotArray[r.crtBalnum] = &Ballot{nil, 0, PHASE1, false, &LeaderBookkeeping{cstructs: make([][]int32, r.N)}}
	if msg.Fast == TRUE {
		r.fastRound = true
	} else {
		r.fastRound = false
	}
}

func (r *Replica) handle1b(msg *gpaxosproto.M_1b) {
	if msg.Balnum != r.crtBalnum {
		log.Println("1b from a different ballot")
		return
	}

	crtbal := r.ballotArray[r.crtBalnum]

	if crtbal.status != PHASE1 {
		//delayed 1b
		return
	}

	dlog.Println("msg.Cstruct: ", msg.Cstruct)
	crtbal.lb.cstructs = append(crtbal.lb.cstructs, msg.Cstruct)
	count := len(crtbal.lb.cstructs)

	//is it sufficient to have the same initial quorum size for both fast and slow rounds?
	if (r.fastRound && count == r.fastQSize) ||
		(!r.fastRound && count == r.N/2+1) {
		_, _, crtbal.cstruct = r.learn(true)
		dlog.Println("LUB:", crtbal.cstruct)
		r.bcast2a(r.crtBalnum, crtbal.cstruct, r.fastRound)
		crtbal.lb.cstructs = make([][]int32, r.N)
		crtbal.status = PHASE2
		if r.fastRound && ALL_TO_ALL {
			r.bcast2b(&gpaxosproto.M_2b{r.Id, r.crtBalnum, crtbal.cstruct, crtbal.cstruct})
		}
	}
}

func (r *Replica) handle2a(msg *gpaxosproto.M_2a) {
	if r.isLeader {
		log.Println("Received 2a even though I am the leader")
		return
	}

	if r.leaderId != msg.LeaderId {
		log.Println("Received 2a from unrecognized leader")
		return
	}

	if r.crtBalnum != msg.Balnum {
		log.Println("Received 2a for different ballot: ", msg.Balnum)
		return
	}

	crtbal := r.ballotArray[r.crtBalnum]
	crtbal.received2a = true
	crtbal.status = PHASE2

	dlog.Println("old cstruct", crtbal.cstruct)

	cids := make([]int32, 0)

	if crtbal.cstruct == nil || len(crtbal.cstruct) == 0 {
		crtbal.cstruct = msg.Cstruct
	} else {
		for _, ocid := range crtbal.cstruct {
			present := false
			for _, cid := range msg.Cstruct {
				if cid == ocid {
					present = true
					break
				}
			}
			if !present {
				msg.Cstruct = append(msg.Cstruct, ocid)
				cids = append(cids, ocid)
			}
		}
		crtbal.cstruct = msg.Cstruct
	}

	if ALL_TO_ALL {
		r.bcast2b(&gpaxosproto.M_2b{r.Id, r.crtBalnum, crtbal.cstruct, cids})
		r.tryToLearn()
	} else {
		r.send2b(&gpaxosproto.M_2b{r.Id, r.crtBalnum, crtbal.cstruct, cids}, r.PeerWriters[r.leaderId])
	}
}

func (r *Replica) handle2b(msg *gpaxosproto.M_2b) {
	if msg.Balnum != r.crtBalnum {
		dlog.Println("2b from a different ballot")
		return
	}

	crtbal := r.ballotArray[r.crtBalnum]

	if r.isLeader && crtbal.status != PHASE2 {
		log.Println("2b before its time")
		return
	}

	crtbal.lb.cstructs[msg.ReplicaId] = msg.Cstruct
	dlog.Printf("Replica %d 2b msg.Cstruct: ", msg.ReplicaId)
	dlog.Println(msg.Cstruct)
	dlog.Println("my cstruct:", crtbal.cstruct)

	crtbal.lb.cstructs[r.Id] = crtbal.cstruct

	r.tryToLearn()
}

func (r *Replica) tryToLearn() {
	var glb []int32
	var conflict bool
	crtbal := r.ballotArray[r.crtBalnum]

	if conflict, glb, _ = r.learn(false); conflict {
		log.Println("Conflict")
		if r.isLeader {
			r.startHigherBallot()
		}
	} else if glb != nil {
		dlog.Println("Got GLB:", glb)
		for _, cid := range glb {
			dlog.Println("Committing command ", cid)
			r.committed[cid] = true
			crtbal.lb.committed++
			if prop, present := r.commandReplies[cid]; present {
				r.ReplyProposeTS(&genericsmrproto.ProposeReplyTS{TRUE, cid, state.NIL, prop.Timestamp}, prop.Reply)
				delete(r.commandReplies, cid)
			}
		}
		if r.isLeader && crtbal.lb.committed >= CMDS_PER_BALLOT {
			r.startHigherBallot()
		}
	}
}

func (r *Replica) startHigherBallot() {
	//TODO: can Phase 1 be bypassed in this situation, as an optimization?
	r.crtBalnum++
	r.fastRound = true
	r.ballotArray[r.crtBalnum] = &Ballot{nil, 0, PHASE1, false, &LeaderBookkeeping{cstructs: make([][]int32, 0)}}
	r.ballotArray[r.crtBalnum].cstruct = r.ballotArray[r.crtBalnum-1].cstruct
	r.ballotArray[r.crtBalnum].lb.cstructs = append(r.ballotArray[r.crtBalnum].lb.cstructs, r.ballotArray[r.crtBalnum-1].cstruct)
	r.bcast1a(r.crtBalnum, true)
}

const (
	WHITE uint8 = iota
	GRAY
	BLACK
)

type node struct {
	count    int
	outEdges map[int32]int
	color    uint8
}

func (r *Replica) learn(getLub bool) (conflict bool, glb []int32, lub []int32) {
	if r.crtBalnum < 0 {
		return false, nil, nil
	}

	crtbal := r.ballotArray[r.crtBalnum]

	if len(crtbal.lb.cstructs) == 0 {
		return false, nil, nil
	}

	idToNode := make(map[int32]*node, 2*len(crtbal.cstruct))

	// build directed graph

	dlog.Println(crtbal.lb.cstructs)
	for i := 0; i < len(crtbal.lb.cstructs); i++ {
		cs := crtbal.lb.cstructs[i]
		for idx, cid := range cs {
			var n *node
			var present bool
			crtCmd := r.commands[cid]
			if _, present = r.committed[cid]; present {
				continue
			}
			if n, present = idToNode[cid]; !present {
				n = &node{0, make(map[int32]int, 2), WHITE}
				idToNode[cid] = n
			}
			n.count++
			for j := 0; j < idx; j++ {
				if _, present = r.committed[cs[j]]; present {
					continue
				}
				if crtCmd == nil {
					log.Println("crtCmd is nil")
					return false, nil, nil
				}
				if r.commands[cs[j]] == nil {
					log.Println("cs[j] is nil")
					return false, nil, nil
				}
				if !state.Conflict(crtCmd, r.commands[cs[j]]) {
					continue
				}
				n.outEdges[cs[j]] = n.outEdges[cs[j]] + 1
			}
		}
	}

	// hack

	/*    conflict = false
	      glb = make([]int32, 0)
	      lub = make([]int32, 0)

	      for cid, n := range idToNode {
	          if n.count >= r.fastQSize {
	              glb = append(glb, cid)
	          }
	          lub = append(lub, cid)
	      }

	      return false, glb, lub*/

	// depth-first search

	for cid, n := range idToNode {
		if n.color == WHITE {
			var conf bool
			conf, glb, lub = r.dfs(cid, n, glb, lub, idToNode)
			conflict = conflict || conf
		}
	}
	/*
	   if getLub && conflict {
	       //sort out lub
	       done := false
	       for !done {
	           done = true
	           for i := 1; i < len(lub) - 1; i++ {
	               for j := i + 1; j < len(lub); j++ {
	                   u := lub[i]
	                   v := lub[j]
	                   cu := r.commands[u]
	                   cv := r.commands[v]
	                   if !state.Conflict(cu, cv) {
	                       continue
	                   }
	                   nu := idToNode[u]
	                   nv := idToNode[v]
	                   if nv.count - nv.outEdges[u] > nu.count - nu.outEdges[v] {
	                       lub[i] = v
	                       lub[j] = u
	                       done = false
	                   }
	               }
	           }
	       }
	   }
	*/
	return conflict, glb, lub
}

func (r *Replica) dfs(cid int32, n *node, glb []int32, lub []int32, idToNode map[int32]*node) (bool, []int32, []int32) {
	conf := false
	n.color = GRAY
	for id, _ := range n.outEdges {
		m := idToNode[id]
		if m.color == BLACK {
			continue
		}
		if m.color == GRAY {
			conf = true
			continue
		}
		var aux bool
		aux, glb, lub = r.dfs(id, m, glb, lub, idToNode)
		conf = conf || aux
	}
	n.color = BLACK
	if !conf && ((!r.fastRound && n.count > r.N/2) || (r.fastRound && n.count >= r.fastQSize)) {
		glb = append(glb, cid)
	}
	lub = append(lub, cid)
	return conf, glb, lub
}
