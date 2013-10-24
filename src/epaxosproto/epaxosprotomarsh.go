package epaxosproto

import (
	"bufio"
	"encoding/binary"
	"fastrpc"
	"io"
	"state"
	"sync"
)

type byteReader interface {
	io.Reader
	ReadByte() (c byte, err error)
}

func (t *TryPreAccept) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

type TryPreAcceptCache struct {
	mu    sync.Mutex
	cache []*TryPreAccept
}

func NewTryPreAcceptCache() *TryPreAcceptCache {
	c := &TryPreAcceptCache{}
	c.cache = make([]*TryPreAccept, 0)
	return c
}

func (p *TryPreAcceptCache) Get() *TryPreAccept {
	var t *TryPreAccept
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &TryPreAccept{}
	}
	return t
}

func (p *TryPreAcceptCache) Put(t *TryPreAccept) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (p *TryPreAccept) New() fastrpc.Serializable {
	return new(TryPreAccept)
}

func (t *TryPreAccept) Marshal(wire io.Writer) {
	var b [24]byte
	var bs []byte
	bs = b[:16]
	tmp32 := t.LeaderId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Replica
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Instance
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	wire.Write(bs)
	bs = b[:]
	alen1 := int64(len(t.Command))
	if wlen := binary.PutVarint(bs, alen1); wlen >= 0 {
		wire.Write(b[0:wlen])
	}
	for i := int64(0); i < alen1; i++ {
		t.Command[i].Marshal(wire)
	}
	tmp32 = t.Seq
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Deps[0]
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Deps[1]
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.Deps[2]
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	tmp32 = t.Deps[3]
	bs[16] = byte(tmp32)
	bs[17] = byte(tmp32 >> 8)
	bs[18] = byte(tmp32 >> 16)
	bs[19] = byte(tmp32 >> 24)
	tmp32 = t.Deps[4]
	bs[20] = byte(tmp32)
	bs[21] = byte(tmp32 >> 8)
	bs[22] = byte(tmp32 >> 16)
	bs[23] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *TryPreAccept) Unmarshal(rr io.Reader) error {
	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	var b [24]byte
	var bs []byte
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.LeaderId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Replica = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Instance = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.Ballot = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	alen1, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Command = make([]state.Command, alen1)
	for i := int64(0); i < alen1; i++ {
		t.Command[i].Unmarshal(wire)
	}
	bs = b[:24]
	if _, err := io.ReadAtLeast(wire, bs, 24); err != nil {
		return err
	}
	t.Seq = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Deps[0] = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Deps[1] = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.Deps[2] = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	t.Deps[3] = int32((uint32(bs[16]) | (uint32(bs[17]) << 8) | (uint32(bs[18]) << 16) | (uint32(bs[19]) << 24)))
	t.Deps[4] = int32((uint32(bs[20]) | (uint32(bs[21]) << 8) | (uint32(bs[22]) << 16) | (uint32(bs[23]) << 24)))
	return nil
}

func (t *PreAcceptReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 57, true
}

type PreAcceptReplyCache struct {
	mu    sync.Mutex
	cache []*PreAcceptReply
}

func NewPreAcceptReplyCache() *PreAcceptReplyCache {
	c := &PreAcceptReplyCache{}
	c.cache = make([]*PreAcceptReply, 0)
	return c
}

func (p *PreAcceptReplyCache) Get() *PreAcceptReply {
	var t *PreAcceptReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &PreAcceptReply{}
	}
	return t
}

func (p *PreAcceptReplyCache) Put(t *PreAcceptReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (p *PreAcceptReply) New() fastrpc.Serializable {
	return new(PreAcceptReply)
}

func (t *PreAcceptReply) Marshal(wire io.Writer) {
	var b [57]byte
	var bs []byte
	bs = b[:57]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Instance
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	bs[8] = byte(t.OK)
	tmp32 = t.Ballot
	bs[9] = byte(tmp32)
	bs[10] = byte(tmp32 >> 8)
	bs[11] = byte(tmp32 >> 16)
	bs[12] = byte(tmp32 >> 24)
	tmp32 = t.Seq
	bs[13] = byte(tmp32)
	bs[14] = byte(tmp32 >> 8)
	bs[15] = byte(tmp32 >> 16)
	bs[16] = byte(tmp32 >> 24)
	tmp32 = t.Deps[0]
	bs[17] = byte(tmp32)
	bs[18] = byte(tmp32 >> 8)
	bs[19] = byte(tmp32 >> 16)
	bs[20] = byte(tmp32 >> 24)
	tmp32 = t.Deps[1]
	bs[21] = byte(tmp32)
	bs[22] = byte(tmp32 >> 8)
	bs[23] = byte(tmp32 >> 16)
	bs[24] = byte(tmp32 >> 24)
	tmp32 = t.Deps[2]
	bs[25] = byte(tmp32)
	bs[26] = byte(tmp32 >> 8)
	bs[27] = byte(tmp32 >> 16)
	bs[28] = byte(tmp32 >> 24)
	tmp32 = t.Deps[3]
	bs[29] = byte(tmp32)
	bs[30] = byte(tmp32 >> 8)
	bs[31] = byte(tmp32 >> 16)
	bs[32] = byte(tmp32 >> 24)
	tmp32 = t.Deps[4]
	bs[33] = byte(tmp32)
	bs[34] = byte(tmp32 >> 8)
	bs[35] = byte(tmp32 >> 16)
	bs[36] = byte(tmp32 >> 24)
	tmp32 = t.CommittedDeps[0]
	bs[37] = byte(tmp32)
	bs[38] = byte(tmp32 >> 8)
	bs[39] = byte(tmp32 >> 16)
	bs[40] = byte(tmp32 >> 24)
	tmp32 = t.CommittedDeps[1]
	bs[41] = byte(tmp32)
	bs[42] = byte(tmp32 >> 8)
	bs[43] = byte(tmp32 >> 16)
	bs[44] = byte(tmp32 >> 24)
	tmp32 = t.CommittedDeps[2]
	bs[45] = byte(tmp32)
	bs[46] = byte(tmp32 >> 8)
	bs[47] = byte(tmp32 >> 16)
	bs[48] = byte(tmp32 >> 24)
	tmp32 = t.CommittedDeps[3]
	bs[49] = byte(tmp32)
	bs[50] = byte(tmp32 >> 8)
	bs[51] = byte(tmp32 >> 16)
	bs[52] = byte(tmp32 >> 24)
	tmp32 = t.CommittedDeps[4]
	bs[53] = byte(tmp32)
	bs[54] = byte(tmp32 >> 8)
	bs[55] = byte(tmp32 >> 16)
	bs[56] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *PreAcceptReply) Unmarshal(wire io.Reader) error {
	var b [57]byte
	var bs []byte
	bs = b[:57]
	if _, err := io.ReadAtLeast(wire, bs, 57); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Instance = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.OK = uint8(bs[8])
	t.Ballot = int32((uint32(bs[9]) | (uint32(bs[10]) << 8) | (uint32(bs[11]) << 16) | (uint32(bs[12]) << 24)))
	t.Seq = int32((uint32(bs[13]) | (uint32(bs[14]) << 8) | (uint32(bs[15]) << 16) | (uint32(bs[16]) << 24)))
	t.Deps[0] = int32((uint32(bs[17]) | (uint32(bs[18]) << 8) | (uint32(bs[19]) << 16) | (uint32(bs[20]) << 24)))
	t.Deps[1] = int32((uint32(bs[21]) | (uint32(bs[22]) << 8) | (uint32(bs[23]) << 16) | (uint32(bs[24]) << 24)))
	t.Deps[2] = int32((uint32(bs[25]) | (uint32(bs[26]) << 8) | (uint32(bs[27]) << 16) | (uint32(bs[28]) << 24)))
	t.Deps[3] = int32((uint32(bs[29]) | (uint32(bs[30]) << 8) | (uint32(bs[31]) << 16) | (uint32(bs[32]) << 24)))
	t.Deps[4] = int32((uint32(bs[33]) | (uint32(bs[34]) << 8) | (uint32(bs[35]) << 16) | (uint32(bs[36]) << 24)))
	t.CommittedDeps[0] = int32((uint32(bs[37]) | (uint32(bs[38]) << 8) | (uint32(bs[39]) << 16) | (uint32(bs[40]) << 24)))
	t.CommittedDeps[1] = int32((uint32(bs[41]) | (uint32(bs[42]) << 8) | (uint32(bs[43]) << 16) | (uint32(bs[44]) << 24)))
	t.CommittedDeps[2] = int32((uint32(bs[45]) | (uint32(bs[46]) << 8) | (uint32(bs[47]) << 16) | (uint32(bs[48]) << 24)))
	t.CommittedDeps[3] = int32((uint32(bs[49]) | (uint32(bs[50]) << 8) | (uint32(bs[51]) << 16) | (uint32(bs[52]) << 24)))
	t.CommittedDeps[4] = int32((uint32(bs[53]) | (uint32(bs[54]) << 8) | (uint32(bs[55]) << 16) | (uint32(bs[56]) << 24)))
	return nil
}

func (t *TryPreAcceptReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 26, true
}

type TryPreAcceptReplyCache struct {
	mu    sync.Mutex
	cache []*TryPreAcceptReply
}

func NewTryPreAcceptReplyCache() *TryPreAcceptReplyCache {
	c := &TryPreAcceptReplyCache{}
	c.cache = make([]*TryPreAcceptReply, 0)
	return c
}

func (p *TryPreAcceptReplyCache) Get() *TryPreAcceptReply {
	var t *TryPreAcceptReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &TryPreAcceptReply{}
	}
	return t
}

func (p *TryPreAcceptReplyCache) Put(t *TryPreAcceptReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (p *TryPreAcceptReply) New() fastrpc.Serializable {
	return new(TryPreAcceptReply)
}

func (t *TryPreAcceptReply) Marshal(wire io.Writer) {
	var b [26]byte
	var bs []byte
	bs = b[:26]
	tmp32 := t.AcceptorId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Replica
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Instance
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	bs[12] = byte(t.OK)
	tmp32 = t.Ballot
	bs[13] = byte(tmp32)
	bs[14] = byte(tmp32 >> 8)
	bs[15] = byte(tmp32 >> 16)
	bs[16] = byte(tmp32 >> 24)
	tmp32 = t.ConflictReplica
	bs[17] = byte(tmp32)
	bs[18] = byte(tmp32 >> 8)
	bs[19] = byte(tmp32 >> 16)
	bs[20] = byte(tmp32 >> 24)
	tmp32 = t.ConflictInstance
	bs[21] = byte(tmp32)
	bs[22] = byte(tmp32 >> 8)
	bs[23] = byte(tmp32 >> 16)
	bs[24] = byte(tmp32 >> 24)
	bs[25] = byte(t.ConflictStatus)
	wire.Write(bs)
}

func (t *TryPreAcceptReply) Unmarshal(wire io.Reader) error {
	var b [26]byte
	var bs []byte
	bs = b[:26]
	if _, err := io.ReadAtLeast(wire, bs, 26); err != nil {
		return err
	}
	t.AcceptorId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Replica = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Instance = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.OK = uint8(bs[12])
	t.Ballot = int32((uint32(bs[13]) | (uint32(bs[14]) << 8) | (uint32(bs[15]) << 16) | (uint32(bs[16]) << 24)))
	t.ConflictReplica = int32((uint32(bs[17]) | (uint32(bs[18]) << 8) | (uint32(bs[19]) << 16) | (uint32(bs[20]) << 24)))
	t.ConflictInstance = int32((uint32(bs[21]) | (uint32(bs[22]) << 8) | (uint32(bs[23]) << 16) | (uint32(bs[24]) << 24)))
	t.ConflictStatus = int8(bs[25])
	return nil
}

func (t *CommitShort) BinarySize() (nbytes int, sizeKnown bool) {
	return 40, true
}

type CommitShortCache struct {
	mu    sync.Mutex
	cache []*CommitShort
}

func NewCommitShortCache() *CommitShortCache {
	c := &CommitShortCache{}
	c.cache = make([]*CommitShort, 0)
	return c
}

func (p *CommitShortCache) Get() *CommitShort {
	var t *CommitShort
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &CommitShort{}
	}
	return t
}

func (p *CommitShortCache) Put(t *CommitShort) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (p *CommitShort) New() fastrpc.Serializable {
	return new(CommitShort)
}

func (t *CommitShort) Marshal(wire io.Writer) {
	var b [40]byte
	var bs []byte
	bs = b[:40]
	tmp32 := t.LeaderId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Replica
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Instance
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.Count
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	tmp32 = t.Seq
	bs[16] = byte(tmp32)
	bs[17] = byte(tmp32 >> 8)
	bs[18] = byte(tmp32 >> 16)
	bs[19] = byte(tmp32 >> 24)
	tmp32 = t.Deps[0]
	bs[20] = byte(tmp32)
	bs[21] = byte(tmp32 >> 8)
	bs[22] = byte(tmp32 >> 16)
	bs[23] = byte(tmp32 >> 24)
	tmp32 = t.Deps[1]
	bs[24] = byte(tmp32)
	bs[25] = byte(tmp32 >> 8)
	bs[26] = byte(tmp32 >> 16)
	bs[27] = byte(tmp32 >> 24)
	tmp32 = t.Deps[2]
	bs[28] = byte(tmp32)
	bs[29] = byte(tmp32 >> 8)
	bs[30] = byte(tmp32 >> 16)
	bs[31] = byte(tmp32 >> 24)
	tmp32 = t.Deps[3]
	bs[32] = byte(tmp32)
	bs[33] = byte(tmp32 >> 8)
	bs[34] = byte(tmp32 >> 16)
	bs[35] = byte(tmp32 >> 24)
	tmp32 = t.Deps[4]
	bs[36] = byte(tmp32)
	bs[37] = byte(tmp32 >> 8)
	bs[38] = byte(tmp32 >> 16)
	bs[39] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *CommitShort) Unmarshal(wire io.Reader) error {
	var b [40]byte
	var bs []byte
	bs = b[:40]
	if _, err := io.ReadAtLeast(wire, bs, 40); err != nil {
		return err
	}
	t.LeaderId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Replica = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Instance = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.Count = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	t.Seq = int32((uint32(bs[16]) | (uint32(bs[17]) << 8) | (uint32(bs[18]) << 16) | (uint32(bs[19]) << 24)))
	t.Deps[0] = int32((uint32(bs[20]) | (uint32(bs[21]) << 8) | (uint32(bs[22]) << 16) | (uint32(bs[23]) << 24)))
	t.Deps[1] = int32((uint32(bs[24]) | (uint32(bs[25]) << 8) | (uint32(bs[26]) << 16) | (uint32(bs[27]) << 24)))
	t.Deps[2] = int32((uint32(bs[28]) | (uint32(bs[29]) << 8) | (uint32(bs[30]) << 16) | (uint32(bs[31]) << 24)))
	t.Deps[3] = int32((uint32(bs[32]) | (uint32(bs[33]) << 8) | (uint32(bs[34]) << 16) | (uint32(bs[35]) << 24)))
	t.Deps[4] = int32((uint32(bs[36]) | (uint32(bs[37]) << 8) | (uint32(bs[38]) << 16) | (uint32(bs[39]) << 24)))
	return nil
}

func (t *PreAccept) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

type PreAcceptCache struct {
	mu    sync.Mutex
	cache []*PreAccept
}

func NewPreAcceptCache() *PreAcceptCache {
	c := &PreAcceptCache{}
	c.cache = make([]*PreAccept, 0)
	return c
}

func (p *PreAcceptCache) Get() *PreAccept {
	var t *PreAccept
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &PreAccept{}
	}
	return t
}

func (p *PreAcceptCache) Put(t *PreAccept) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (p *PreAccept) New() fastrpc.Serializable {
	return new(PreAccept)
}

func (t *PreAccept) Marshal(wire io.Writer) {
	var b [24]byte
	var bs []byte
	bs = b[:16]
	tmp32 := t.LeaderId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Replica
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Instance
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	wire.Write(bs)
	bs = b[:]
	alen1 := int64(len(t.Command))
	if wlen := binary.PutVarint(bs, alen1); wlen >= 0 {
		wire.Write(b[0:wlen])
	}
	for i := int64(0); i < alen1; i++ {
		t.Command[i].Marshal(wire)
	}
	tmp32 = t.Seq
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Deps[0]
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Deps[1]
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.Deps[2]
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	tmp32 = t.Deps[3]
	bs[16] = byte(tmp32)
	bs[17] = byte(tmp32 >> 8)
	bs[18] = byte(tmp32 >> 16)
	bs[19] = byte(tmp32 >> 24)
	tmp32 = t.Deps[4]
	bs[20] = byte(tmp32)
	bs[21] = byte(tmp32 >> 8)
	bs[22] = byte(tmp32 >> 16)
	bs[23] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *PreAccept) Unmarshal(rr io.Reader) error {
	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	var b [24]byte
	var bs []byte
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.LeaderId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Replica = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Instance = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.Ballot = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	alen1, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Command = make([]state.Command, alen1)
	for i := int64(0); i < alen1; i++ {
		t.Command[i].Unmarshal(wire)
	}
	bs = b[:24]
	if _, err := io.ReadAtLeast(wire, bs, 24); err != nil {
		return err
	}
	t.Seq = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Deps[0] = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Deps[1] = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.Deps[2] = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	t.Deps[3] = int32((uint32(bs[16]) | (uint32(bs[17]) << 8) | (uint32(bs[18]) << 16) | (uint32(bs[19]) << 24)))
	t.Deps[4] = int32((uint32(bs[20]) | (uint32(bs[21]) << 8) | (uint32(bs[22]) << 16) | (uint32(bs[23]) << 24)))
	return nil
}

func (t *PreAcceptOK) BinarySize() (nbytes int, sizeKnown bool) {
	return 4, true
}

type PreAcceptOKCache struct {
	mu    sync.Mutex
	cache []*PreAcceptOK
}

func NewPreAcceptOKCache() *PreAcceptOKCache {
	c := &PreAcceptOKCache{}
	c.cache = make([]*PreAcceptOK, 0)
	return c
}

func (p *PreAcceptOKCache) Get() *PreAcceptOK {
	var t *PreAcceptOK
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &PreAcceptOK{}
	}
	return t
}

func (p *PreAcceptOKCache) Put(t *PreAcceptOK) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (p *PreAcceptOK) New() fastrpc.Serializable {
	return new(PreAcceptOK)
}

func (t *PreAcceptOK) Marshal(wire io.Writer) {
	var b [4]byte
	var bs []byte
	bs = b[:4]
	tmp32 := t.Instance
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *PreAcceptOK) Unmarshal(wire io.Reader) error {
	var b [4]byte
	var bs []byte
	bs = b[:4]
	if _, err := io.ReadAtLeast(wire, bs, 4); err != nil {
		return err
	}
	t.Instance = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	return nil
}

func (t *PrepareReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

type PrepareReplyCache struct {
	mu    sync.Mutex
	cache []*PrepareReply
}

func NewPrepareReplyCache() *PrepareReplyCache {
	c := &PrepareReplyCache{}
	c.cache = make([]*PrepareReply, 0)
	return c
}

func (p *PrepareReplyCache) Get() *PrepareReply {
	var t *PrepareReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &PrepareReply{}
	}
	return t
}

func (p *PrepareReplyCache) Put(t *PrepareReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (p *PrepareReply) New() fastrpc.Serializable {
	return new(PrepareReply)
}

func (t *PrepareReply) Marshal(wire io.Writer) {
	var b [24]byte
	var bs []byte
	bs = b[:18]
	tmp32 := t.AcceptorId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Replica
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Instance
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	bs[12] = byte(t.OK)
	tmp32 = t.Ballot
	bs[13] = byte(tmp32)
	bs[14] = byte(tmp32 >> 8)
	bs[15] = byte(tmp32 >> 16)
	bs[16] = byte(tmp32 >> 24)
	bs[17] = byte(t.Status)
	wire.Write(bs)
	bs = b[:]
	alen1 := int64(len(t.Command))
	if wlen := binary.PutVarint(bs, alen1); wlen >= 0 {
		wire.Write(b[0:wlen])
	}
	for i := int64(0); i < alen1; i++ {
		t.Command[i].Marshal(wire)
	}
	tmp32 = t.Seq
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Deps[0]
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Deps[1]
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.Deps[2]
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	tmp32 = t.Deps[3]
	bs[16] = byte(tmp32)
	bs[17] = byte(tmp32 >> 8)
	bs[18] = byte(tmp32 >> 16)
	bs[19] = byte(tmp32 >> 24)
	tmp32 = t.Deps[4]
	bs[20] = byte(tmp32)
	bs[21] = byte(tmp32 >> 8)
	bs[22] = byte(tmp32 >> 16)
	bs[23] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *PrepareReply) Unmarshal(rr io.Reader) error {
	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	var b [24]byte
	var bs []byte
	bs = b[:18]
	if _, err := io.ReadAtLeast(wire, bs, 18); err != nil {
		return err
	}
	t.AcceptorId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Replica = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Instance = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.OK = uint8(bs[12])
	t.Ballot = int32((uint32(bs[13]) | (uint32(bs[14]) << 8) | (uint32(bs[15]) << 16) | (uint32(bs[16]) << 24)))
	t.Status = int8(bs[17])
	alen1, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Command = make([]state.Command, alen1)
	for i := int64(0); i < alen1; i++ {
		t.Command[i].Unmarshal(wire)
	}
	bs = b[:24]
	if _, err := io.ReadAtLeast(wire, bs, 24); err != nil {
		return err
	}
	t.Seq = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Deps[0] = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Deps[1] = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.Deps[2] = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	t.Deps[3] = int32((uint32(bs[16]) | (uint32(bs[17]) << 8) | (uint32(bs[18]) << 16) | (uint32(bs[19]) << 24)))
	t.Deps[4] = int32((uint32(bs[20]) | (uint32(bs[21]) << 8) | (uint32(bs[22]) << 16) | (uint32(bs[23]) << 24)))
	return nil
}

func (t *Commit) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

type CommitCache struct {
	mu    sync.Mutex
	cache []*Commit
}

func NewCommitCache() *CommitCache {
	c := &CommitCache{}
	c.cache = make([]*Commit, 0)
	return c
}

func (p *CommitCache) Get() *Commit {
	var t *Commit
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &Commit{}
	}
	return t
}

func (p *CommitCache) Put(t *Commit) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (p *Commit) New() fastrpc.Serializable {
	return new(Commit)
}

func (t *Commit) Marshal(wire io.Writer) {
	var b [24]byte
	var bs []byte
	bs = b[:12]
	tmp32 := t.LeaderId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Replica
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Instance
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	wire.Write(bs)
	bs = b[:]
	alen1 := int64(len(t.Command))
	if wlen := binary.PutVarint(bs, alen1); wlen >= 0 {
		wire.Write(b[0:wlen])
	}
	for i := int64(0); i < alen1; i++ {
		t.Command[i].Marshal(wire)
	}
	tmp32 = t.Seq
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Deps[0]
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Deps[1]
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.Deps[2]
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	tmp32 = t.Deps[3]
	bs[16] = byte(tmp32)
	bs[17] = byte(tmp32 >> 8)
	bs[18] = byte(tmp32 >> 16)
	bs[19] = byte(tmp32 >> 24)
	tmp32 = t.Deps[4]
	bs[20] = byte(tmp32)
	bs[21] = byte(tmp32 >> 8)
	bs[22] = byte(tmp32 >> 16)
	bs[23] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *Commit) Unmarshal(rr io.Reader) error {
	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	var b [24]byte
	var bs []byte
	bs = b[:12]
	if _, err := io.ReadAtLeast(wire, bs, 12); err != nil {
		return err
	}
	t.LeaderId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Replica = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Instance = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	alen1, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Command = make([]state.Command, alen1)
	for i := int64(0); i < alen1; i++ {
		t.Command[i].Unmarshal(wire)
	}
	bs = b[:24]
	if _, err := io.ReadAtLeast(wire, bs, 24); err != nil {
		return err
	}
	t.Seq = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Deps[0] = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Deps[1] = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.Deps[2] = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	t.Deps[3] = int32((uint32(bs[16]) | (uint32(bs[17]) << 8) | (uint32(bs[18]) << 16) | (uint32(bs[19]) << 24)))
	t.Deps[4] = int32((uint32(bs[20]) | (uint32(bs[21]) << 8) | (uint32(bs[22]) << 16) | (uint32(bs[23]) << 24)))
	return nil
}

func (t *AcceptReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 13, true
}

type AcceptReplyCache struct {
	mu    sync.Mutex
	cache []*AcceptReply
}

func NewAcceptReplyCache() *AcceptReplyCache {
	c := &AcceptReplyCache{}
	c.cache = make([]*AcceptReply, 0)
	return c
}

func (p *AcceptReplyCache) Get() *AcceptReply {
	var t *AcceptReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &AcceptReply{}
	}
	return t
}

func (p *AcceptReplyCache) Put(t *AcceptReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (p *AcceptReply) New() fastrpc.Serializable {
	return new(AcceptReply)
}

func (t *AcceptReply) Marshal(wire io.Writer) {
	var b [13]byte
	var bs []byte
	bs = b[:13]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Instance
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	bs[8] = byte(t.OK)
	tmp32 = t.Ballot
	bs[9] = byte(tmp32)
	bs[10] = byte(tmp32 >> 8)
	bs[11] = byte(tmp32 >> 16)
	bs[12] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *AcceptReply) Unmarshal(wire io.Reader) error {
	var b [13]byte
	var bs []byte
	bs = b[:13]
	if _, err := io.ReadAtLeast(wire, bs, 13); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Instance = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.OK = uint8(bs[8])
	t.Ballot = int32((uint32(bs[9]) | (uint32(bs[10]) << 8) | (uint32(bs[11]) << 16) | (uint32(bs[12]) << 24)))
	return nil
}

func (t *Accept) BinarySize() (nbytes int, sizeKnown bool) {
	return 44, true
}

type AcceptCache struct {
	mu    sync.Mutex
	cache []*Accept
}

func NewAcceptCache() *AcceptCache {
	c := &AcceptCache{}
	c.cache = make([]*Accept, 0)
	return c
}

func (p *AcceptCache) Get() *Accept {
	var t *Accept
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &Accept{}
	}
	return t
}

func (p *AcceptCache) Put(t *Accept) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (p *Accept) New() fastrpc.Serializable {
	return new(Accept)
}

func (t *Accept) Marshal(wire io.Writer) {
	var b [44]byte
	var bs []byte
	bs = b[:44]
	tmp32 := t.LeaderId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Replica
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Instance
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	tmp32 = t.Count
	bs[16] = byte(tmp32)
	bs[17] = byte(tmp32 >> 8)
	bs[18] = byte(tmp32 >> 16)
	bs[19] = byte(tmp32 >> 24)
	tmp32 = t.Seq
	bs[20] = byte(tmp32)
	bs[21] = byte(tmp32 >> 8)
	bs[22] = byte(tmp32 >> 16)
	bs[23] = byte(tmp32 >> 24)
	tmp32 = t.Deps[0]
	bs[24] = byte(tmp32)
	bs[25] = byte(tmp32 >> 8)
	bs[26] = byte(tmp32 >> 16)
	bs[27] = byte(tmp32 >> 24)
	tmp32 = t.Deps[1]
	bs[28] = byte(tmp32)
	bs[29] = byte(tmp32 >> 8)
	bs[30] = byte(tmp32 >> 16)
	bs[31] = byte(tmp32 >> 24)
	tmp32 = t.Deps[2]
	bs[32] = byte(tmp32)
	bs[33] = byte(tmp32 >> 8)
	bs[34] = byte(tmp32 >> 16)
	bs[35] = byte(tmp32 >> 24)
	tmp32 = t.Deps[3]
	bs[36] = byte(tmp32)
	bs[37] = byte(tmp32 >> 8)
	bs[38] = byte(tmp32 >> 16)
	bs[39] = byte(tmp32 >> 24)
	tmp32 = t.Deps[4]
	bs[40] = byte(tmp32)
	bs[41] = byte(tmp32 >> 8)
	bs[42] = byte(tmp32 >> 16)
	bs[43] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *Accept) Unmarshal(wire io.Reader) error {
	var b [44]byte
	var bs []byte
	bs = b[:44]
	if _, err := io.ReadAtLeast(wire, bs, 44); err != nil {
		return err
	}
	t.LeaderId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Replica = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Instance = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.Ballot = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	t.Count = int32((uint32(bs[16]) | (uint32(bs[17]) << 8) | (uint32(bs[18]) << 16) | (uint32(bs[19]) << 24)))
	t.Seq = int32((uint32(bs[20]) | (uint32(bs[21]) << 8) | (uint32(bs[22]) << 16) | (uint32(bs[23]) << 24)))
	t.Deps[0] = int32((uint32(bs[24]) | (uint32(bs[25]) << 8) | (uint32(bs[26]) << 16) | (uint32(bs[27]) << 24)))
	t.Deps[1] = int32((uint32(bs[28]) | (uint32(bs[29]) << 8) | (uint32(bs[30]) << 16) | (uint32(bs[31]) << 24)))
	t.Deps[2] = int32((uint32(bs[32]) | (uint32(bs[33]) << 8) | (uint32(bs[34]) << 16) | (uint32(bs[35]) << 24)))
	t.Deps[3] = int32((uint32(bs[36]) | (uint32(bs[37]) << 8) | (uint32(bs[38]) << 16) | (uint32(bs[39]) << 24)))
	t.Deps[4] = int32((uint32(bs[40]) | (uint32(bs[41]) << 8) | (uint32(bs[42]) << 16) | (uint32(bs[43]) << 24)))
	return nil
}

func (t *Prepare) BinarySize() (nbytes int, sizeKnown bool) {
	return 16, true
}

type PrepareCache struct {
	mu    sync.Mutex
	cache []*Prepare
}

func NewPrepareCache() *PrepareCache {
	c := &PrepareCache{}
	c.cache = make([]*Prepare, 0)
	return c
}

func (p *PrepareCache) Get() *Prepare {
	var t *Prepare
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &Prepare{}
	}
	return t
}

func (p *PrepareCache) Put(t *Prepare) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (p *Prepare) New() fastrpc.Serializable {
	return new(Prepare)
}

func (t *Prepare) Marshal(wire io.Writer) {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	tmp32 := t.LeaderId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Replica
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Instance
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *Prepare) Unmarshal(wire io.Reader) error {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.LeaderId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Replica = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Instance = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.Ballot = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	return nil
}
