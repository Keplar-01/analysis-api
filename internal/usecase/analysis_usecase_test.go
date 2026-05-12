package usecase

import (
	"strings"
	"testing"

	"github.com/diploma/analysis-api-service/internal/model"
)

func TestBuildVariableSequenceRowsStableForSameSequence(t *testing.T) {
	staticRows := []model.StaticPatternRow{
		{TaskID: "task-1", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "b", SequenceIndex: 1, AccessKind: "store", PatternSignature: "sig-store"},
		{TaskID: "task-1", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "a", SequenceIndex: 0, AccessKind: "load", PatternSignature: "sig-load"},
	}

	permutedRows := []model.StaticPatternRow{
		{TaskID: "task-2", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "a", SequenceIndex: 0, AccessKind: "load", PatternSignature: "sig-load"},
		{TaskID: "task-2", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "b", SequenceIndex: 1, AccessKind: "store", PatternSignature: "sig-store"},
	}

	left := toVariableSequenceMap(buildVariableSequenceRows(staticRows))
	right := toVariableSequenceMap(buildVariableSequenceRows(permutedRows))

	if len(left) != 2 || len(right) != 2 {
		t.Fatalf("unexpected variable sequence row count: left=%d right=%d", len(left), len(right))
	}
	if left["a"].VariableSequenceHash != right["a"].VariableSequenceHash {
		t.Fatalf("hash for a differs: %q vs %q", left["a"].VariableSequenceHash, right["a"].VariableSequenceHash)
	}
	if left["b"].VariableSequenceHash != right["b"].VariableSequenceHash {
		t.Fatalf("hash for b differs: %q vs %q", left["b"].VariableSequenceHash, right["b"].VariableSequenceHash)
	}
}

func TestBuildVariableSequenceRowsChangesWhenOrderChanges(t *testing.T) {
	loadThenStore := []model.StaticPatternRow{
		{TaskID: "task-1", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "a", SequenceIndex: 0, AccessKind: "load", PatternSignature: "sig-load"},
		{TaskID: "task-1", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "a", SequenceIndex: 1, AccessKind: "store", PatternSignature: "sig-store"},
	}
	storeThenLoad := []model.StaticPatternRow{
		{TaskID: "task-2", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "a", SequenceIndex: 0, AccessKind: "store", PatternSignature: "sig-store"},
		{TaskID: "task-2", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "a", SequenceIndex: 1, AccessKind: "load", PatternSignature: "sig-load"},
	}

	left := buildVariableSequenceRows(loadThenStore)
	right := buildVariableSequenceRows(storeThenLoad)

	if len(left) != 1 || len(right) != 1 {
		t.Fatalf("unexpected row count: left=%d right=%d", len(left), len(right))
	}
	if left[0].VariableSequenceHash == right[0].VariableSequenceHash {
		t.Fatalf("expected variable sequence hash to change when order changes")
	}
}

func TestBuildVariableSequenceRowsPreservesVariableSet(t *testing.T) {
	left := buildVariableSequenceRows([]model.StaticPatternRow{
		{TaskID: "task-1", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "a", SequenceIndex: 0, AccessKind: "load", PatternSignature: "sig-load"},
		{TaskID: "task-1", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "b", SequenceIndex: 1, AccessKind: "store", PatternSignature: "sig-store"},
	})
	right := buildVariableSequenceRows([]model.StaticPatternRow{
		{TaskID: "task-2", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "a", SequenceIndex: 0, AccessKind: "load", PatternSignature: "sig-load"},
	})

	leftMap := toVariableSequenceMap(left)
	rightMap := toVariableSequenceMap(right)
	if len(leftMap) == len(rightMap) {
		t.Fatalf("expected different variable set sizes")
	}
	if _, ok := rightMap["b"]; ok {
		t.Fatalf("did not expect variable b in right sequence map")
	}
}

func TestBuildVariableRoleSymbolMapIgnoresVariableNames(t *testing.T) {
	sourceRows := []model.StaticPatternRow{
		{TaskID: "source", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "a", SequenceIndex: 1, AccessKind: "store", PatternSignature: "sig-store"},
		{TaskID: "source", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "b", SequenceIndex: 0, AccessKind: "load", PatternSignature: "sig-load"},
	}
	targetRows := []model.StaticPatternRow{
		{TaskID: "target", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "dst", SequenceIndex: 1, AccessKind: "store", PatternSignature: "sig-store"},
		{TaskID: "target", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "src", SequenceIndex: 0, AccessKind: "load", PatternSignature: "sig-load"},
	}

	mapping, ok := buildVariableRoleSymbolMap(sourceRows, targetRows)
	if !ok {
		t.Fatalf("expected role-based symbol mapping to succeed")
	}
	if mapping["a"] != "dst" {
		t.Fatalf("mapping[a] = %q, want %q", mapping["a"], "dst")
	}
	if mapping["b"] != "src" {
		t.Fatalf("mapping[b] = %q, want %q", mapping["b"], "src")
	}
}

func TestBuildVariableRoleSymbolMapRejectsAmbiguousDuplicates(t *testing.T) {
	sourceRows := []model.StaticPatternRow{
		{TaskID: "source", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "a", SequenceIndex: 0, AccessKind: "load", PatternSignature: "sig-load"},
		{TaskID: "source", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "b", SequenceIndex: 1, AccessKind: "load", PatternSignature: "sig-load"},
	}
	targetRows := []model.StaticPatternRow{
		{TaskID: "target", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "x", SequenceIndex: 0, AccessKind: "load", PatternSignature: "sig-load"},
		{TaskID: "target", ProjectID: "project-1", CacheProfileHash: "profile", BaseSymbol: "y", SequenceIndex: 1, AccessKind: "load", PatternSignature: "sig-load"},
	}

	if _, ok := buildVariableRoleSymbolMap(sourceRows, targetRows); ok {
		t.Fatalf("expected ambiguous duplicate role mapping to be rejected")
	}
}

func TestRemapCacheResultRenamesSourceArraysToTargetSymbols(t *testing.T) {
	raw := &model.CacheSimResult{
		SourceFile: "source.c",
		Arrays: []model.ArrayCacheMetric{
			{CacheLevel: "L1", ArrayName: "a", MissesTotal: 16},
			{CacheLevel: "L1", ArrayName: "b", MissesTotal: 1},
		},
	}

	remapped := remapCacheResult(raw, map[string]string{"a": "dst", "b": "src"}, "target.c")
	if remapped.SourceFile != "target.c" {
		t.Fatalf("SourceFile = %q, want %q", remapped.SourceFile, "target.c")
	}
	if remapped.Arrays[0].ArrayName != "dst" || remapped.Arrays[1].ArrayName != "src" {
		t.Fatalf("unexpected remapped arrays: %+v", remapped.Arrays)
	}
}

func toVariableSequenceMap(rows []model.VariableSequenceRow) map[string]model.VariableSequenceRow {
	result := make(map[string]model.VariableSequenceRow, len(rows))
	for _, row := range rows {
		result[row.BaseSymbol] = row
	}
	return result
}

func TestMaterializeDynamicPatternMetricsUsesBaseSymbolAndAccessKind(t *testing.T) {
	uc := &AnalysisUseCase{interpreterVersion: "test-interpreter"}
	raw := &model.CacheSimResult{
		SourceFile: "simple_loop.c",
		Arrays: []model.ArrayCacheMetric{
			{CacheLevel: "L1", ArrayName: "a", MissesTotal: 12, MissesRead: 10, MissesWrite: 2},
			{CacheLevel: "L2", ArrayName: "a", MissesTotal: 6, MissesRead: 5, MissesWrite: 1},
			{CacheLevel: "L1", ArrayName: "b", MissesTotal: 8, MissesRead: 1, MissesWrite: 7},
		},
	}
	staticRows := []model.StaticPatternRow{
		{PatternFingerprint: "fp-load-a", BaseSymbol: "a", AccessKind: "load", CacheProfileHash: "profile"},
		{PatternFingerprint: "fp-store-a", BaseSymbol: "a", AccessKind: "store", CacheProfileHash: "profile"},
		{PatternFingerprint: "fp-load-c", BaseSymbol: "c", AccessKind: "load", CacheProfileHash: "profile"},
	}

	rows := uc.materializeDynamicPatternMetrics("task-1", raw, staticRows, nil)
	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	seen := make(map[string]model.DynamicPatternMetric)
	for _, row := range rows {
		key := row.PatternFingerprint + "|" + row.BaseSymbol + "|" + row.AccessKind + "|" + row.CacheLevel
		seen[key] = row
	}

	loadL1, ok := seen["fp-load-a|a|load|L1"]
	if !ok {
		t.Fatalf("missing materialized load row for a/L1")
	}
	if loadL1.MissesRead != 10 || loadL1.MissesWrite != 2 {
		t.Fatalf("loadL1 = %+v", loadL1)
	}

	storeL2, ok := seen["fp-store-a|a|store|L2"]
	if !ok {
		t.Fatalf("missing materialized store row for a/L2")
	}
	if storeL2.MissesRead != 5 || storeL2.MissesWrite != 1 {
		t.Fatalf("storeL2 = %+v", storeL2)
	}

	if _, ok := seen["fp-load-c|c|load|L1"]; ok {
		t.Fatalf("did not expect rows for missing base_symbol c")
	}
}

func TestMaterializeDynamicPatternMetricsFallsBackToSourceArrayName(t *testing.T) {
	uc := &AnalysisUseCase{interpreterVersion: "test-interpreter"}
	raw := &model.CacheSimResult{
		SourceFile: "matrix_mult.c",
		Arrays: []model.ArrayCacheMetric{
			{CacheLevel: "L1", ArrayName: "a", MissesTotal: 10, MissesRead: 9, MissesWrite: 1},
			{CacheLevel: "L2", ArrayName: "a", MissesTotal: 5, MissesRead: 4, MissesWrite: 1},
			{CacheLevel: "L1", ArrayName: "c", MissesTotal: 7, MissesRead: 2, MissesWrite: 5},
		},
	}
	staticRows := []model.StaticPatternRow{
		{PatternFingerprint: "fp-a", BaseSymbol: "ssa_arrayidx32", AccessKind: "load", CacheProfileHash: "profile", SourceLine: 21, SourceColumn: 37},
		{PatternFingerprint: "fp-c", BaseSymbol: "ssa_arrayidx41", AccessKind: "store", CacheProfileHash: "profile", SourceLine: 21, SourceColumn: 25},
	}
	source := []byte("int main()\n{\n    c[i][j] = c[i][j] + a[i][k] * b[k][j];\n}\n")

	rows := uc.materializeDynamicPatternMetrics("task-1", raw, staticRows, source)
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}

	seen := make(map[string]model.DynamicPatternMetric)
	for _, row := range rows {
		key := row.PatternFingerprint + "|" + row.BaseSymbol + "|" + row.CacheLevel
		seen[key] = row
	}

	aL1, ok := seen["fp-a|ssa_arrayidx32|L1"]
	if !ok {
		t.Fatalf("missing resolved row for array a/L1")
	}
	if aL1.MissesTotal != 10 || aL1.BaseSymbol != "ssa_arrayidx32" {
		t.Fatalf("aL1 = %+v", aL1)
	}

	cL1, ok := seen["fp-c|ssa_arrayidx41|L1"]
	if !ok {
		t.Fatalf("missing resolved row for array c/L1")
	}
	if cL1.MissesWrite != 5 {
		t.Fatalf("cL1 = %+v", cL1)
	}
}

func TestValidateCacheInterpreterSourceRejectsKnownUnsupportedConstructs(t *testing.T) {
	source := []byte(`#define N 16
double a[N];
int main() {
	for (int i = 0; i < N; i++) {
		a[i] = 1.0;
	}
	return 0;
}`)

	err := validateCacheInterpreterSource(source)
	if err == nil {
		t.Fatalf("expected unsupported source to be rejected")
	}
	message := err.Error()
	for _, want := range []string{"preprocessor directives", "float/double", "declarations inside for"} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected %q in error message %q", want, message)
		}
	}
}

func TestValidateCacheInterpreterSourceAcceptsSupportedSubset(t *testing.T) {
	source := []byte(`int main() {
	int i;
	int a[16];
	int b[16];
	for (i = 0; i < 16; i = i + 1) {
		a[i] = b[i];
	}
	return 0;
}`)

	if err := validateCacheInterpreterSource(source); err != nil {
		t.Fatalf("expected supported source to pass validation, got %v", err)
	}
}

func TestNormalizeUploadProjectIDFallsBackToDefault(t *testing.T) {
	if got := normalizeUploadProjectID(""); got != defaultUploadProjectID {
		t.Fatalf("normalizeUploadProjectID(empty) = %q, want %q", got, defaultUploadProjectID)
	}
	if got := normalizeUploadProjectID("   "); got != defaultUploadProjectID {
		t.Fatalf("normalizeUploadProjectID(blank) = %q, want %q", got, defaultUploadProjectID)
	}
	if got := normalizeUploadProjectID("project-123"); got != "project-123" {
		t.Fatalf("normalizeUploadProjectID(project-123) = %q", got)
	}
}
