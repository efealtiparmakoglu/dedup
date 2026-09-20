// Package xxh64 implements the XXH64 hash (XXH64 specification, copyright
// (c) Yann Collet) — a fast non-cryptographic 64-bit hash used as the first
// similarity pass in dedup. Pure Go, no dependencies.
package xxh64

import (
	"encoding/binary"
	"math/bits"
)

const (
	prime1 uint64 = 0x9E3779B185EBCA87
	prime2 uint64 = 0xC2B2AE3D27D4EB4F
	prime3 uint64 = 0x165667B19E3779F9
	prime4 uint64 = 0x85EBCA77C2B2AE63
	prime5 uint64 = 0x27D4EB2F165667C5
)

// Sum computes the XXH64 hash of data using seed.
func Sum(data []byte, seed uint64) uint64 {
	n := len(data)
	var h uint64

	switch {
	case n >= 32:
		// 4 accumulators, 16 bytes per step
		v1 := seed + prime1 + prime2
		v2 := seed + prime2
		v3 := seed
		v4 := seed - prime1
		remaining := n
		for remaining >= 32 {
			v1 = round(v1, binary.LittleEndian.Uint64(data[0:8]))
			v2 = round(v2, binary.LittleEndian.Uint64(data[8:16]))
			v3 = round(v3, binary.LittleEndian.Uint64(data[16:24]))
			v4 = round(v4, binary.LittleEndian.Uint64(data[24:32]))
			data = data[32:]
			remaining -= 32
		}
		h = bits.RotateLeft64(v1, 1) + bits.RotateLeft64(v2, 7) +
			bits.RotateLeft64(v3, 12) + bits.RotateLeft64(v4, 18)
		h = mergeRound(h, v1)
		h = mergeRound(h, v2)
		h = mergeRound(h, v3)
		h = mergeRound(h, v4)
		if remaining == 0 {
			return avalanche(h + uint64(n))
		}
		h += uint64(n)
	default:
		h = seed + prime5
		h += uint64(n)
		if remaining := n; remaining == 0 {
			return avalanche(h)
		}
	}

	// remaining 32..: strip 8 bytes per step; 8..: 4 bytes; then singles
	for len(data) >= 8 {
		k1 := round(0, binary.LittleEndian.Uint64(data[:8]))
		h ^= k1
		h = bits.RotateLeft64(h, 27)*prime1 + prime4
		data = data[8:]
	}
	if len(data) >= 4 {
		h ^= uint64(binary.LittleEndian.Uint32(data[:4])) * prime1
		h = bits.RotateLeft64(h, 23)*prime2 + prime3
		data = data[4:]
	}
	for _, b := range data {
		h ^= uint64(b) * prime5
		h = bits.RotateLeft64(h, 11) * prime1
	}
	return avalanche(h)
}

func round(acc, input uint64) uint64 {
	acc += input * prime2
	acc = bits.RotateLeft64(acc, 31)
	acc *= prime1
	return acc
}

func mergeRound(acc, val uint64) uint64 {
	val = round(0, val)
	acc ^= val
	acc = acc*prime1 + prime4
	return acc
}

func avalanche(h uint64) uint64 {
	h ^= h >> 33
	h *= prime2
	h ^= h >> 29
	h *= prime3
	h ^= h >> 32
	return h
}
