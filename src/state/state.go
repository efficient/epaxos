package state

import (
	"sync"
	"fmt"
	//"code.google.com/p/leveldb-go/leveldb"
	//"encoding/binary"
	"encoding/hex"
	"strconv"
	"dlog"
)

type Operation uint8

const (
	NONE Operation = iota
	PUT
	GET
	SCAN
	DELETE
	RLOCK
	WLOCK
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
	Store map[Key]Value
	//DB *leveldb.DB
}

func InitState() *State {
	/*
	   d, err := leveldb.Open("/Users/iulian/git/epaxos-batching/dpaxos/bin/db", nil)

	   if err != nil {
	       fmt.Printf("Leveldb open failed: %v\n", err)
	   }

	   return &State{d}
	*/

	return &State{new(sync.Mutex), make(map[Key]Value)}
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
	fmt.Printf("Executing (%d, %d)\n", c.K, c.V)

	//var key, value [8]byte

	st.mutex.Lock()
	defer st.mutex.Unlock()

	switch c.Op {
	case PUT:
		/*
		   binary.LittleEndian.PutUint64(key[:], uint64(c.K))
		   binary.LittleEndian.PutUint64(value[:], uint64(c.V))
		   st.DB.Set(key[:], value[:], nil)
		*/
		dlog.Println(c.Op,"(",c.K,",",c.V,")")
		st.Store[c.K] = c.V
		return NIL()

	case GET:
		if val, present := st.Store[c.K]; present {
			return val
		}

	case SCAN:
		val := NIL()
		for i:=c.K; i<(c.K+100); i++{
			if tmp, present := st.Store[c.K]; present {
				val = tmp
			}
		}
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

