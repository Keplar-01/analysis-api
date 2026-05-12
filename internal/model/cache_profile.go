package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type CacheProfile struct {
	L1SizeKB        uint32 `json:"l1_size_kb"`
	L1LineSize      uint32 `json:"l1_line_size"`
	L1Associativity uint8  `json:"l1_associativity"`
	L2SizeKB        uint32 `json:"l2_size_kb"`
	L2LineSize      uint32 `json:"l2_line_size"`
	L2Associativity uint8  `json:"l2_associativity"`
}

func DefaultCacheProfile() CacheProfile {
	return CacheProfile{
		L1SizeKB:        32,
		L1LineSize:      64,
		L1Associativity: 8,
		L2SizeKB:        256,
		L2LineSize:      64,
		L2Associativity: 8,
	}
}

func (p CacheProfile) Hash() string {
	canonical := fmt.Sprintf(
		"l1=%d:%d:%d|l2=%d:%d:%d",
		p.L1SizeKB,
		p.L1LineSize,
		p.L1Associativity,
		p.L2SizeKB,
		p.L2LineSize,
		p.L2Associativity,
	)

	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}
