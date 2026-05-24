package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type CacheProfile struct {
	NumLevels       uint8  `json:"num_levels"`
	L1SizeKB        uint32 `json:"l1_size_kb"`
	L1SizeBytes     uint64 `json:"-"`
	L1LineSize      uint32 `json:"l1_line_size"`
	L1Associativity uint8  `json:"l1_associativity"`
	L2SizeKB        uint32 `json:"l2_size_kb"`
	L2SizeBytes     uint64 `json:"-"`
	L2LineSize      uint32 `json:"l2_line_size"`
	L2Associativity uint8  `json:"l2_associativity"`
	L3SizeKB        uint32 `json:"l3_size_kb"`
	L3SizeBytes     uint64 `json:"-"`
	L3LineSize      uint32 `json:"l3_line_size"`
	L3Associativity uint8  `json:"l3_associativity"`
}

type CacheConfigLevel struct {
	LevelName      string `json:"level_name"`
	CacheSize      uint64 `json:"cacheSize"`
	CacheBlockSize uint64 `json:"cacheBlockSize"`
	Way            uint64 `json:"way"`
}

type CacheConfigDocument struct {
	NumLevels uint8            `json:"num_levels"`
	L1        CacheConfigLevel `json:"l1"`
	L2        CacheConfigLevel `json:"l2"`
	L3        CacheConfigLevel `json:"l3"`
}

type cacheConfigDocumentStrict struct {
	NumLevels uint8             `json:"num_levels"`
	L1        *CacheConfigLevel `json:"l1"`
	L2        *CacheConfigLevel `json:"l2"`
	L3        *CacheConfigLevel `json:"l3"`
}

func DefaultCacheProfile() CacheProfile {
	return CacheProfile{
		NumLevels:       2,
		L1SizeKB:        32,
		L1SizeBytes:     32 * 1024,
		L1LineSize:      64,
		L1Associativity: 8,
		L2SizeKB:        256,
		L2SizeBytes:     256 * 1024,
		L2LineSize:      64,
		L2Associativity: 8,
	}
}

func (p CacheProfile) Hash() string {
	p = p.normalized()
	canonical := fmt.Sprintf(
		"levels=%d|l1=%d:%d:%d|l2=%d:%d:%d|l3=%d:%d:%d",
		p.NumLevels,
		p.l1SizeBytes(),
		p.L1LineSize,
		p.L1Associativity,
		p.l2SizeBytes(),
		p.L2LineSize,
		p.L2Associativity,
		p.l3SizeBytes(),
		p.L3LineSize,
		p.L3Associativity,
	)

	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func (p CacheProfile) normalized() CacheProfile {
	if p.L1SizeBytes == 0 {
		p.L1SizeBytes = uint64(p.L1SizeKB) * 1024
	}
	if p.L2SizeBytes == 0 {
		p.L2SizeBytes = uint64(p.L2SizeKB) * 1024
	}
	if p.L3SizeBytes == 0 {
		p.L3SizeBytes = uint64(p.L3SizeKB) * 1024
	}
	if p.NumLevels < 2 {
		p.NumLevels = 2
	}
	if p.NumLevels < 3 {
		p.L3SizeKB = 0
		p.L3SizeBytes = 0
		p.L3LineSize = 0
		p.L3Associativity = 0
	}
	if p.L3SizeBytes > 0 || p.L3SizeKB > 0 || p.L3LineSize > 0 || p.L3Associativity > 0 {
		p.NumLevels = 3
	}
	return p
}

func (p CacheProfile) l1SizeBytes() uint64 {
	if p.L1SizeBytes > 0 {
		return p.L1SizeBytes
	}
	return uint64(p.L1SizeKB) * 1024
}

func (p CacheProfile) l2SizeBytes() uint64 {
	if p.L2SizeBytes > 0 {
		return p.L2SizeBytes
	}
	return uint64(p.L2SizeKB) * 1024
}

func (p CacheProfile) l3SizeBytes() uint64 {
	if p.L3SizeBytes > 0 {
		return p.L3SizeBytes
	}
	return uint64(p.L3SizeKB) * 1024
}

func CacheProfileFromConfigJSON(data []byte) (CacheProfile, error) {
	var cfg cacheConfigDocumentStrict
	if err := json.Unmarshal(data, &cfg); err != nil {
		return CacheProfile{}, err
	}
	if cfg.NumLevels < 2 || cfg.NumLevels > 3 {
		return CacheProfile{}, fmt.Errorf("num_levels must be 2 or 3")
	}
	if cfg.L1 == nil {
		return CacheProfile{}, fmt.Errorf("missing l1")
	}
	if cfg.L2 == nil {
		return CacheProfile{}, fmt.Errorf("missing l2")
	}
	if err := validateCacheConfigLevel("l1", *cfg.L1); err != nil {
		return CacheProfile{}, err
	}
	if err := validateCacheConfigLevel("l2", *cfg.L2); err != nil {
		return CacheProfile{}, err
	}
	if cfg.NumLevels == 3 {
		if cfg.L3 == nil {
			return CacheProfile{}, fmt.Errorf("missing l3")
		}
		if err := validateCacheConfigLevel("l3", *cfg.L3); err != nil {
			return CacheProfile{}, err
		}
	}

	profile := DefaultCacheProfile()
	profile.NumLevels = cfg.NumLevels
	applyLevel := func(level CacheConfigLevel, sizeKB *uint32, lineSize *uint32, associativity *uint8) {
		*sizeKB = uint32(level.CacheSize / 1024)
		*lineSize = uint32(level.CacheBlockSize)
		*associativity = uint8(level.Way)
	}

	applyLevel(*cfg.L1, &profile.L1SizeKB, &profile.L1LineSize, &profile.L1Associativity)
	profile.L1SizeBytes = cfg.L1.CacheSize
	applyLevel(*cfg.L2, &profile.L2SizeKB, &profile.L2LineSize, &profile.L2Associativity)
	profile.L2SizeBytes = cfg.L2.CacheSize
	if cfg.NumLevels == 3 {
		applyLevel(*cfg.L3, &profile.L3SizeKB, &profile.L3LineSize, &profile.L3Associativity)
		profile.L3SizeBytes = cfg.L3.CacheSize
	}

	return profile.normalized(), nil
}

func validateCacheConfigLevel(name string, level CacheConfigLevel) error {
	if level.LevelName == "" {
		return fmt.Errorf("missing %s.level_name", name)
	}
	if level.CacheSize == 0 {
		return fmt.Errorf("missing %s.cacheSize", name)
	}
	if level.CacheBlockSize == 0 {
		return fmt.Errorf("missing %s.cacheBlockSize", name)
	}
	if level.Way == 0 {
		return fmt.Errorf("missing %s.way", name)
	}
	if level.Way > 255 {
		return fmt.Errorf("%s.way is too large", name)
	}
	return nil
}
