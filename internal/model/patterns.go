package model

type StaticArtifactPattern struct {
	SequenceIndex      uint32   `json:"sequence_index"`
	AccessKind         string   `json:"access_kind"`
	Affine             bool     `json:"affine"`
	Alignment          *int     `json:"alignment"`
	BaseKind           string   `json:"base_kind"`
	BaseSymbol         string   `json:"base_symbol"`
	Conditional        bool     `json:"conditional"`
	ContiguousBlock    *int     `json:"contiguous_block"`
	Dependence         string   `json:"dependence"`
	Depth              int      `json:"depth"`
	FillFactor         float64  `json:"fill_factor"`
	Function           string   `json:"function"`
	HasIndexedAddr     bool     `json:"has_indexed_addressing"`
	IndexedByMemory    bool     `json:"indexed_by_memory"`
	LoadCount          int      `json:"load_count"`
	PatternFingerprint string   `json:"pattern_fingerprint"`
	PatternSig         string   `json:"pattern_signature"`
	PatternType        string   `json:"pattern_type"`
	SourceColumn       int      `json:"source_column"`
	SourceFile         string   `json:"source_file"`
	SourceLine         int      `json:"source_line"`
	StoreCount         int      `json:"store_count"`
	Stride             *float64 `json:"stride"`
	WorkingSetBytes    int      `json:"working_set_bytes"`
}

type StaticPatternRow struct {
	TaskID             string   `json:"task_id"`
	ProjectID          string   `json:"project_id"`
	SequenceIndex      uint32   `json:"sequence_index"`
	SourceFile         string   `json:"source_file"`
	SourceLine         uint32   `json:"source_line"`
	SourceColumn       uint32   `json:"source_column"`
	Function           string   `json:"function"`
	BaseSymbol         string   `json:"base_symbol"`
	BaseKind           string   `json:"base_kind"`
	AccessKind         string   `json:"access_kind"`
	PatternType        string   `json:"pattern_type"`
	PatternFingerprint string   `json:"pattern_fingerprint"`
	Affine             uint8    `json:"affine"`
	CacheProfileHash   string   `json:"cache_profile_hash"`
	FillFactor         float64  `json:"fill_factor"`
	Stride             *float64 `json:"stride"`
	Depth              uint8    `json:"depth"`
	HasIndexedAddr     uint8    `json:"has_indexed_addr"`
	IndexedByMemory    uint8    `json:"indexed_by_memory"`
	Conditional        uint8    `json:"conditional"`
	Alignment          *uint32  `json:"alignment"`
	WorkingSetBytes    uint64   `json:"working_set_bytes"`
	Dependence         string   `json:"dependence"`
	PatternSignature   string   `json:"pattern_signature"`
	ContiguousBlock    *uint32  `json:"contiguous_block"`
	LoadCount          uint32   `json:"load_count"`
	StoreCount         uint32   `json:"store_count"`
	ArtifactS3Path     string   `json:"artifact_s3_path"`
}

type VariableSequenceRow struct {
	TaskID               string `json:"task_id"`
	ProjectID            string `json:"project_id"`
	CacheProfileHash     string `json:"cache_profile_hash"`
	BaseSymbol           string `json:"base_symbol"`
	VariableSequenceHash string `json:"variable_sequence_hash"`
	PatternCount         uint32 `json:"pattern_count"`
}

type DynamicPatternMetric struct {
	SequenceIndex      uint32 `json:"sequence_index"`
	PatternFingerprint string `json:"pattern_fingerprint"`
	BaseSymbol         string `json:"base_symbol"`
	AccessKind         string `json:"access_kind"`
	CacheProfileHash   string `json:"cache_profile_hash"`
	CacheLevel         string `json:"cache_level"`
	MissesTotal        uint64 `json:"misses_total"`
	MissesRead         uint64 `json:"misses_read"`
	MissesWrite        uint64 `json:"misses_write"`
	SourceTaskID       string `json:"source_task_id"`
	SourceFile         string `json:"source_file"`
	InterpreterVersion string `json:"interpreter_version"`
}

type CacheSimResult struct {
	SourceFile   string             `json:"source_file"`
	SimTimeSec   float64            `json:"sim_time_sec"`
	L1           CacheLevelSummary  `json:"l1"`
	L2           CacheLevelSummary  `json:"l2"`
	L3           CacheLevelSummary  `json:"l3"`
	Arrays       []ArrayCacheMetric `json:"arrays"`
	MemoryReads  uint64             `json:"memory_reads"`
	MemoryWrites uint64             `json:"memory_writes"`
}

type CacheLevelSummary struct {
	CacheLevel    string  `json:"cache_level"`
	CacheSizeKB   uint32  `json:"cache_size_kb"`
	CacheLineSize uint32  `json:"cache_line_size"`
	Associativity uint8   `json:"associativity"`
	TotalAccesses uint64  `json:"total_accesses"`
	TotalHits     uint64  `json:"total_hits"`
	TotalMisses   uint64  `json:"total_misses"`
	HitsRead      uint64  `json:"hits_read"`
	HitsWrite     uint64  `json:"hits_write"`
	MissesRead    uint64  `json:"misses_read"`
	MissesWrite   uint64  `json:"misses_write"`
	MissRate      float64 `json:"miss_rate"`
}

type ArrayCacheMetric struct {
	CacheLevel  string `json:"cache_level"`
	ArrayName   string `json:"array_name"`
	MissesTotal uint64 `json:"misses_total"`
	MissesRead  uint64 `json:"misses_read"`
	MissesWrite uint64 `json:"misses_write"`
}

func (r CacheSimResult) CacheLevels() []CacheLevelSummary {
	levels := make([]CacheLevelSummary, 0, 3)
	for _, level := range []CacheLevelSummary{r.L1, r.L2, r.L3} {
		if level.CacheLevel == "" {
			continue
		}
		levels = append(levels, level)
	}
	return levels
}
