package bitvec

type Bitvec []uint64

func New(size uint32) Bitvec {
	asize := size / 64
	if asize*64 < size {
		asize++
	}
	bv := Bitvec(make([]uint64, asize))
	bv.Clear()
	return bv
}

func (bv Bitvec) Clear() {
	for i := 0; i < len(bv); i++ {
		bv[i] = 0
	}
}

func (bv Bitvec) GetBit(pos uint32) bool {
	return ((bv[pos>>6] & (1 << (pos & uint32(63)))) != 0)
}

func (bv Bitvec) SetBit(pos uint32) {
	bv[pos>>6] |= uint64(1) << (pos & uint32(63))
}

func (bv Bitvec) ResetBit(pos uint32) {
	bv[pos>>6] &= ^(uint64(1) << (pos & uint32(63)))
}
