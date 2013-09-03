package menciusproto

import (
	"state"
)

type Skip struct {
	LeaderId      int32
	StartInstance int32
	EndInstance   int32
}

type Prepare struct {
	LeaderId int32
	Instance int32
	Ballot   int32
}

type PrepareReply struct {
	Instance          int32
	OK                uint8
	Ballot            int32
	Skip              uint8
	NbInstancesToSkip int32
	Command           state.Command
}

type Accept struct {
	LeaderId          int32
	Instance          int32
	Ballot            int32
	Skip              uint8
	NbInstancesToSkip int32
	Command           state.Command
}

type AcceptReply struct {
	Instance             int32
	OK                   uint8
	Ballot               int32
	SkippedStartInstance int32
	SkippedEndInstance   int32
}

type Commit struct {
	LeaderId          int32
	Instance          int32
	Skip              uint8
	NbInstancesToSkip int32
	//Command state.Command
}
