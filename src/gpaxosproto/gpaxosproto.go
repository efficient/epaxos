package gpaxosproto

//import (
//    "state"
//)

const (
	PREPARE uint8 = iota
	PREPARE_REPLY
	M1A
	M1B
	M2A
	M2B
	COMMIT
)

type Prepare struct {
	LeaderId int32
	Balnum   int32
	Ballot   int32
}

type PrepareReply struct {
	Balnum  int32
	OK      uint8
	Ballot  int32
	Cstruct []int32
}

type M_1a struct {
	LeaderId int32
	Balnum   int32
	Fast     uint8
}

type M_1b struct {
	ReplicaId int32
	Balnum    int32
	Cstruct   []int32
}

type M_2a struct {
	LeaderId int32
	Balnum   int32
	Cstruct  []int32
}

type M_2b struct {
	ReplicaId int32
	Balnum    int32
	Cstruct   []int32
	Cids      []int32
}

type Commit struct {
	Cstruct []int32
}
