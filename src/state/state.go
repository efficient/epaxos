package state

import (
	"sync"
	"github.com/emirpasic/gods/maps/treemap"
	"encoding/hex"
	"strconv"
)

type Operation uint8

const (
	NONE Operation = iota
	PUT
	GET
	SCAN
)

type Value []byte

func NIL() Value {return Value([]byte{})}

type Key int64

type Command struct {
	Op Operation
	K  Key
	V  Value
}

type State struct {
	mutex *sync.Mutex
	Store *treemap.Map
}

func KeyComparator(a, b interface{}) int {
	aAsserted := a.(Key)
	bAsserted := b.(Key)
	switch {
	case aAsserted > bAsserted:
		return 1
	case aAsserted < bAsserted:
		return -1
	default:
		return 0
	}
}

func InitState() *State {
	return &State{new(sync.Mutex), 	treemap.NewWith(KeyComparator)}
}

func Conflict(gamma *Command, delta *Command) bool {
	if gamma.K == delta.K {
		if gamma.Op == PUT || delta.Op == PUT {
			return true
		}
	}
	return false
}

func ConflictBatch(batch1 []Command, batch2 []Command) bool {
	for i := 0; i < len(batch1); i++ {
		for j := 0; j < len(batch2); j++ {
			if Conflict(&batch1[i], &batch2[j]) {
				return true
			}
		}
	}
	return false
}

func IsRead(command *Command) bool {
	return command.Op == GET
}

func (c *Command) Execute(st *State) Value {

	st.mutex.Lock()
	defer st.mutex.Unlock()

	switch c.Op {
	case PUT:
		st.Store.Put(c.K,c.V)

	case GET:
		if val, present := st.Store.Get(c.K); present {
			return val.(Value)
		}

	case SCAN:
		val := NIL()
		return val
	}

	return NIL()
}

func (t *Value) String() string{
	return hex.EncodeToString(*t)
}

func (t *Key) String() string{
	return strconv.FormatInt(int64(*t), 16)
}

func (t *Command) String() string{
	ret := ""
	if t.Op==PUT {
		ret = "PUT( " + t.K.String() + " , " + t.V.String() + " )"
	} else if t.Op==GET {
		ret="GET( "+t.K.String()+" )"
	} else if t.Op==SCAN {
		ret="SCAN( " + t.V.String() + " , " + t.K.String() + " )"
	} else {
		ret="UNKNOWN( " + t.V.String() + " , " + t.K.String() + " )"
	}
	return ret
}

