package featureflags

import (
	"encoding/binary"
	"math/bits"
)

// rolloutBucket returns a bucket from 1 to 100 for a targeting key. Hashing
// the flag's evaluation key in keeps a caller's bucket stable for one flag
// while spreading callers independently across flags. Every Octopus provider
// buckets the same way, so a caller lands in the same bucket whichever
// provider evaluates the flag.
func rolloutBucket(evaluationKey, targetingKey string) int32 {
	hash := murmur3x86_32([]byte(evaluationKey + ":" + targetingKey))
	return int32(hash%100 + 1)
}

// murmur3x86_32 is MurmurHash3's x86 32-bit variant with a seed of 0.
func murmur3x86_32(data []byte) uint32 {
	const (
		c1 = 0xcc9e2d51
		c2 = 0x1b873593
	)

	length := len(data)
	var hash uint32

	for ; len(data) >= 4; data = data[4:] {
		k := binary.LittleEndian.Uint32(data)
		k *= c1
		k = bits.RotateLeft32(k, 15)
		k *= c2

		hash ^= k
		hash = bits.RotateLeft32(hash, 13)
		hash = hash*5 + 0xe6546b64
	}

	var k uint32
	switch len(data) {
	case 3:
		k ^= uint32(data[2]) << 16
		fallthrough
	case 2:
		k ^= uint32(data[1]) << 8
		fallthrough
	case 1:
		k ^= uint32(data[0])
		k *= c1
		k = bits.RotateLeft32(k, 15)
		k *= c2
		hash ^= k
	}

	hash ^= uint32(length)
	hash ^= hash >> 16
	hash *= 0x85ebca6b
	hash ^= hash >> 13
	hash *= 0xc2b2ae35
	hash ^= hash >> 16

	return hash
}
