package featureflags

import (
	"encoding/binary"
	"fmt"
	"math/bits"

	"github.com/open-feature/go-sdk/openfeature"
)

// percentageByContextCondition matches when the targeting key's bucket,
// from 1 to 100, is at most Percentage. Percentage is a pointer so a missing
// value can be told apart from 0, which legitimately means nobody.
type percentageByContextCondition struct {
	Percentage *int32 `json:"percentage"`
}

func (c percentageByContextCondition) matches(ctx ruleContext) (bool, error) {
	if c.Percentage == nil {
		return false, parseError("A condition is missing a percentage value.")
	}

	percentage := *c.Percentage

	// Rejected rather than clamped: reading 101 as everyone would turn a flag
	// on off the back of a bad payload.
	if percentage < 0 || percentage > 100 {
		return false, parseError(fmt.Sprintf("A condition has a percentage of %d.", percentage))
	}

	// Without a targeting key there's nothing to bucket, so only a 100%
	// rollout matches. The server does the same for a caller with no tenant.
	targetingKey, _ := ctx.flatCtx[openfeature.TargetingKey].(string)
	if targetingKey == "" {
		return percentage >= 100, nil
	}

	return rolloutBucket(ctx.evaluationKey, targetingKey) <= percentage, nil
}

// rolloutBucket returns a bucket from 1 to 100 for a targeting key. Hashing
// the flag's evaluation key along with it keeps a caller in the same bucket
// for one flag, without tying their buckets across flags. Every Octopus
// provider buckets the same way, so a caller lands in the same bucket
// whichever provider evaluates the flag.
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
