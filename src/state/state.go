package state

import (
	"sync"
	"github.com/emirpasic/gods/maps/treemap"
	"encoding/hex"
	"strconv"
	"encoding/binary"
	"fmt"
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

func concat(slices []Value) Value{
	var totalLen int
	for _, s := range slices {
		totalLen += len(s)
	}
	tmp := make([]byte, totalLen)
	var i int
	for _, s := range slices {
		i += copy(tmp[i:], s)
	}
	return tmp
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
		if value, present := st.Store.Get(c.K); present {
			valAsserted := value.(Value)
			return valAsserted
		}

	case SCAN:
		found := make([]Value,0)
		count := int64(binary.LittleEndian.Uint64(c.V))
		key := int64(c.K)
		for key <= int64(c.K) + count {
			if val,exist := st.Store.Get(Key(key)); exist{
				valAsserted := val.(Value)
				found = append(found, valAsserted)
			}
			key++
		}
		ret := concat(found)
		return ret
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
		count := binary.LittleEndian.Uint64(t.V)
		ret="SCAN( " + t.K.String() + " , " +  fmt.Sprint(count) + " )"
	} else {
		ret="UNKNOWN( " + t.V.String() + " , " + t.K.String() + " )"
	}
	return ret
}

