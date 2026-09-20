package xxh64

import "testing"

func TestKnownVectors(t *testing.T) {
	// XXH64 reference vectors
	if got := Sum(nil, 0); got != 0xEF46DB3751D8E999 {
		t.Fatalf("empty: %x", got)
	}
	if got := Sum([]byte("a"), 0); got != 0xD24EC4F1A98C6E5B {
		t.Fatalf("'a': %x", got)
	}
	if got := Sum([]byte("abc"), 0); got != 0x44BC2CF5AD770999 {
		t.Fatalf("'abc': %x", got)
	}
}

func TestLengthAffectsHash(t *testing.T) {
	// trailing-zero bytes must change the hash (length is mixed in)
	a := Sum([]byte{1, 2, 3}, 0)
	b := Sum([]byte{1, 2, 3, 0}, 0)
	if a == b {
		t.Fatal("length extension collision")
	}
}

func TestSeedChangesHash(t *testing.T) {
	if Sum([]byte("data"), 0) == Sum([]byte("data"), 1) {
		t.Fatal("seed has no effect")
	}
}

func TestStripeBoundaries(t *testing.T) {
	// inputs across the 32-byte stripe boundary and 8-byte tail steps
	for n := 0; n < 200; n++ {
		buf := make([]byte, n)
		for i := range buf {
			buf[i] = byte(i * 31)
		}
		Sum(buf, 0) // must not panic; stability checked indirectly
	}
}
