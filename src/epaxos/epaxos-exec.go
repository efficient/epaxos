package epaxos

import (
	//    "state"
	"dlog"
	"epaxosproto"
	"genericsmrproto"
	"sort"
	"state"
)

const (
	WHITE int8 = iota
	GRAY
	BLACK
)

type Exec struct {
	r *Replica
}

type SCComponent struct {
	nodes []*Instance
	color int8
}

func (e *Exec) executeCommand(replica int32, instance int32) bool {
	if e.r.InstanceSpace[replica][instance] == nil {
		return false
	}
	inst := e.r.InstanceSpace[replica][instance]
	if inst.Status == epaxosproto.EXECUTED {
		return true
	}
	if inst.Status != epaxosproto.COMMITTED {
		dlog.Printf("Not committed instance %d.%d\n", replica, instance)
		return false
	}

	if !e.findSCC(inst) {
		return false
	}

	return true
}

var stack []*Instance = make([]*Instance, 0, 100)

func (e *Exec) findSCC(root *Instance) bool {
	index := 1
	// find SCCs using Tarjan's algorithm
	stack = stack[0:0]
	ret := e.strongconnect(root, &index)
	// reset all indexes in the stack
	for j := 0; j < len(stack); j++ {
		stack[j].Index = 0
	}
	return ret
}

func (e *Exec) strongconnect(v *Instance, index *int) bool {
	v.Index = *index
	v.Lowlink = *index
	*index = *index + 1

	l := len(stack)
	if l == cap(stack) {
		newSlice := make([]*Instance, l, 2*l)
		copy(newSlice, stack)
		stack = newSlice
	}
	stack = stack[0 : l+1]
	stack[l] = v

	if v.Cmds == nil {
		dlog.Printf("Null instance! \n")
		return false
	}

	for q := int32(0); q < int32(e.r.N); q++ {
		inst := v.Deps[q]
		for i := e.r.ExecedUpTo[q] + 1; i <= inst; i++ {
			if e.r.InstanceSpace[q][i] == nil {
				dlog.Printf("Null instance %d.%d\n", q, i)
				return false
			}

			if e.r.InstanceSpace[q][i].Cmds == nil {
				dlog.Printf("Null command %d.%d\n", q, i)
				return false
			}

			if e.r.InstanceSpace[q][i].Status == epaxosproto.EXECUTED {
				continue
			}

			for e.r.InstanceSpace[q][i].Status != epaxosproto.COMMITTED {
				dlog.Printf("Not committed instance %d.%d\n", q, i)
				return false
			}

			w := e.r.InstanceSpace[q][i]

			if w.Index == 0 {
				if !e.strongconnect(w, index) {
					return false
				}
				if w.Lowlink < v.Lowlink {
					v.Lowlink = w.Lowlink
				}
			} else { //if e.inStack(w)  //<- probably unnecessary condition, saves a linear search
				if w.Index < v.Lowlink {
					v.Lowlink = w.Index
				}
			}
		}
	}

	if v.Lowlink == v.Index {
		//found SCC
		list := stack[l:]

		//execute commands in the increasing order of the Seq field
		sort.Sort(nodeArray(list))
		for _, w := range list {
			for idx := 0; idx < len(w.Cmds); idx++ {
				dlog.Printf("Executing "+w.Cmds[idx].String()+" (%d,%d)[%d], deps=%d, scc_size=%d", w.Coordinator, w.Seq, idx, w.Deps, len(list))
				if w.Cmds[idx].Op == state.NONE {
					dlog.Printf("Skipping no-op command")
				} else if e.r.Dreply && w.lb != nil && w.lb.clientProposals != nil {
					val := w.Cmds[idx].Execute(e.r.State)

					e.r.ReplyProposeTS(
						&genericsmrproto.ProposeReplyTS{
							TRUE,
							w.lb.clientProposals[idx].CommandId,
							val,
							w.lb.clientProposals[idx].Timestamp},
						w.lb.clientProposals[idx].Reply,
						w.lb.clientProposals[idx].Mutex)
				} else if w.Cmds[idx].Op == state.PUT {
					w.Cmds[idx].Execute(e.r.State)
				}
			}
			w.Status = epaxosproto.EXECUTED
		}
		stack = stack[0:l]
	}

	return true
}

func (e *Exec) inStack(w *Instance) bool {
	for _, u := range stack {
		if w == u {
			return true
		}
	}
	return false
}

type nodeArray []*Instance

func (na nodeArray) Len() int {
	return len(na)
}

func (na nodeArray) Less(i, j int) bool {
	return na[i].Seq < na[j].Seq || (na[i].Seq == na[j].Seq && na[i].Coordinator < na[j].Coordinator) || (na[i].Seq == na[j].Seq && na[i].Coordinator == na[j].Coordinator && na[i].proposeTime < na[j].proposeTime)
}

func (na nodeArray) Swap(i, j int) {
	na[i], na[j] = na[j], na[i]
}
