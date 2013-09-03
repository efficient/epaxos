package bloomfilter

import (
	"bitvec"
	"math"
)

const (
	k2 = 0x9ae16a3b2f90404f
)

func hash64(s uint64) uint64 {
	var mul uint64 = k2 + 8
	var u uint64 = (4 + (s << 3))

	// Murmur-inspired hashing.
	var a uint64 = (u ^ s) * mul
	a ^= (a >> 47)
	b := (s ^ a) * mul
	b ^= (b >> 47)
	b *= mul
	return b
}

func CityHash64(s uint64) uint64 {
	var mul uint64 = k2 + 16
	a := s + k2
	u := ((s<<37)|(s>>27))*mul + a
	v := ((a<<25 | a>>39) + s) * mul
	// HashLen16(u, v, mul)
	// Murmur-inspired hashing.

	a = (u ^ v) * mul
	a ^= (a >> 47)
	b := (v ^ a) * mul
	b ^= (b >> 47)
	b *= mul
	return b
}

type Bloomfilter struct {
	bv     bitvec.Bitvec
	m      uint32
	k      uint32
	powtwo uint32
	mask   uint32
}

/*func New(m uint32, k uint32) *Bloomfilter {
    return &Bloomfilter{bitvec.New(m), m, k}
}*/

func NewPowTwo(pt uint32, k uint32) *Bloomfilter {
	m := uint32(math.Pow(2, float64(pt)))
	return &Bloomfilter{bitvec.New(m), m, k, pt, (uint32(1) << pt) - 1}
}

func hashX(h1 uint32, h2 uint32, i uint32) uint32 {
	switch i {
	case 0:
		return h1
	case 1:
		return h2
	case 2:
		return (h1 << 16) | (h2 >> 16)
	case 3:
		return (h1 >> 16) | (h2 << 16)
	case 4:
		return h1 + h2
	case 5:
		return h1 + 7*h2
	}
	return 0
}

func (bf Bloomfilter) AddUint64(item uint64) {
	//    h64 := hash64(item)
	h64 := CityHash64(item)
	l32 := uint32(h64)
	h32 := uint32(h64 >> 32)

	for i := uint32(0); i < bf.k; i++ {
		bf.bv.SetBit(hashX(l32, h32, i) & bf.mask)
	}
}

func (bf Bloomfilter) CheckUint64(item uint64) bool {
	//    h64 := hash64(item)
	h64 := CityHash64(item)
	l32 := uint32(h64)
	h32 := uint32(h64 >> 32)

	for i := uint32(0); i < bf.k; i++ {
		if !bf.bv.GetBit(hashX(l32, h32, i) & bf.mask) {
			return false
		}
	}
	return true
}
