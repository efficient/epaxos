package bloomfilter

import (
	"math"
	"testing"
)

func TestFPRate(t *testing.T) {
	n := uint64(1000000)

	bf := NewPowTwo(26, 4)

	for i := uint64(0); i < n; i++ {
		bf.AddUint64(i)
	}
	fp := 0
	for i := n; i < 11*n; i++ {
		if bf.CheckUint64(i) {
			fp++
		}
	}

	t.Logf("Expected FP rate: %f\n", math.Pow((1-math.Exp(-4.0/(float64(bf.m)/float64(n)))), 4.0))
	t.Logf("Actual FP rate: %f\n", float64(fp)/float64(10*n))
}

func TestCorrect(t *testing.T) {
	n := uint64(1000000)
	bf := NewPowTwo(26, 4)

	for i := uint64(0); i < n; i++ {
		bf.AddUint64(i)
	}
	fp := 0

	for i := uint64(0); i < n; i++ {
		if bf.CheckUint64(i) {
			fp++
		}
	}

	t.Logf("Expected to find %d items\n", n)
	t.Logf("Found %d items\n", fp)

	if n != uint64(fp) {
		t.Fail()
	}
}
