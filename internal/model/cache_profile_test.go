package model

import "testing"

func TestCacheProfileHashChangesWithParameters(t *testing.T) {
	base := DefaultCacheProfile()
	same := DefaultCacheProfile()
	other := DefaultCacheProfile()
	other.L2SizeKB = 512

	if base.Hash() != same.Hash() {
		t.Fatalf("expected identical profiles to have identical hashes")
	}
	if base.Hash() == other.Hash() {
		t.Fatalf("expected different profiles to have different hashes")
	}
}
