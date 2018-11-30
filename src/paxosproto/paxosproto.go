package paxosproto

import (
	"state"
)

type Prepare struct {
	LeaderId   int32
	Instance   int32
	Ballot     int32
}

type PrepareReply struct {
	Instance      int32
	Ballot        int32
	VBallot       int32
	DefaultBallot int32
	AcceptorId    int32
	Command       []state.Command
}

type Accept struct {
	LeaderId int32
	Instance int32
	Ballot   int32
	Command  []state.Command
}

type AcceptReply struct {
	Instance int32
	Ballot   int32
}

type Commit struct {
	LeaderId int32
	Instance int32
	Ballot   int32
	Command  []state.Command
}

type CommitShort struct {
	LeaderId int32
	Instance int32
	Count    int32
	Ballot   int32
}
