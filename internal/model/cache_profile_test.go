package model

import "testing"

func TestCacheProfileHashChangesWithParameters(t *testing.T) {
	base := DefaultCacheProfile()
	same := DefaultCacheProfile()
	other := DefaultCacheProfile()
	other.L2SizeKB = 512
	other.L2SizeBytes = 512 * 1024

	if base.Hash() != same.Hash() {
		t.Fatalf("expected identical profiles to have identical hashes")
	}
	if base.Hash() == other.Hash() {
		t.Fatalf("expected different profiles to have different hashes")
	}
}

func TestCacheProfileHashPreservesByteLevelCacheSizes(t *testing.T) {
	base := DefaultCacheProfile()
	base.L1SizeKB = 25
	base.L1SizeBytes = 25600

	other := DefaultCacheProfile()
	other.L1SizeKB = 25
	other.L1SizeBytes = 26214

	if base.Hash() == other.Hash() {
		t.Fatalf("expected byte-level cache size difference to change profile hash")
	}
}

func TestCacheProfileFromConfigJSONRequiresSimulatorSchema(t *testing.T) {
	if _, err := CacheProfileFromConfigJSON([]byte(`{}`)); err == nil {
		t.Fatal("expected missing schema to fail")
	}
	if _, err := CacheProfileFromConfigJSON([]byte(`{"num_levels":2,"l1":{"level_name":"L1","cacheSize":32768,"way":8},"l2":{"level_name":"L2","cacheSize":262144,"cacheBlockSize":64,"way":8}}`)); err == nil {
		t.Fatal("expected missing l1 cacheBlockSize to fail")
	}
}

func TestCacheProfileFromConfigJSONParsesL3(t *testing.T) {
	profile, err := CacheProfileFromConfigJSON([]byte(`{
		"num_levels":3,
		"l1":{"level_name":"L1","cacheSize":32768,"cacheBlockSize":64,"way":8},
		"l2":{"level_name":"L2","cacheSize":262144,"cacheBlockSize":64,"way":8},
		"l3":{"level_name":"L3","cacheSize":1048576,"cacheBlockSize":128,"way":16}
	}`))
	if err != nil {
		t.Fatalf("CacheProfileFromConfigJSON returned error: %v", err)
	}
	if profile.NumLevels != 3 || profile.L1SizeKB != 32 || profile.L3SizeKB != 1024 || profile.L3LineSize != 128 {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}

func TestCacheProfileFromConfigJSONAcceptsNonKiBByteSizes(t *testing.T) {
	profile, err := CacheProfileFromConfigJSON([]byte(`{
		"num_levels":2,
		"l1":{"level_name":"L1","cacheSize":26214,"cacheBlockSize":64,"way":8},
		"l2":{"level_name":"L2","cacheSize":26214,"cacheBlockSize":64,"way":8}
	}`))
	if err != nil {
		t.Fatalf("CacheProfileFromConfigJSON returned error: %v", err)
	}
	if profile.L1SizeBytes != 26214 || profile.L2SizeBytes != 26214 {
		t.Fatalf("unexpected byte sizes: %+v", profile)
	}
	if profile.L1SizeKB != 25 || profile.L2SizeKB != 25 {
		t.Fatalf("unexpected KiB projection: %+v", profile)
	}
}
