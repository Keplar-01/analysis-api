package model

import "time"

type File struct {
	ID          string     `json:"id" db:"id"`
	ProjectID   string     `json:"project_id" db:"project_id"`
	Filename    string     `json:"filename" db:"filename"`
	S3Path      string     `json:"s3_path" db:"s3_path"`
	ContentHash string     `json:"content_hash" db:"content_hash"`
	SizeBytes   int64      `json:"size_bytes" db:"size_bytes"`
	OwnerUserID string     `json:"owner_user_id,omitempty" db:"owner_user_id"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

// FileIsHiddenFromUser возвращает true, если файл скрыт из списка из-за мягкого удаления.
func FileIsHiddenFromUser(f *File) bool {
	return f != nil && f.DeletedAt != nil
}

type AnalysisTask struct {
	ID                 string    `json:"id" db:"id"`
	FileID             string    `json:"file_id" db:"file_id"`
	Status             string    `json:"status" db:"status"`
	Type               string    `json:"type" db:"type"`
	ErrorMessage       string    `json:"error_message,omitempty" db:"error_message"`
	CacheProfileHash   string    `json:"cache_profile_hash" db:"cache_profile_hash"`
	CacheConfigID      string    `json:"cache_config_id,omitempty" db:"cache_config_id"`
	CacheConfigS3Path  string    `json:"cache_config_s3_path,omitempty" db:"cache_config_s3_path"`
	StaticArtifactPath string    `json:"static_artifact_s3_path" db:"static_artifact_s3_path"`
	CacheArtifactPath  string    `json:"cache_artifact_s3_path" db:"cache_artifact_s3_path"`
	ReusedFromTaskID   string    `json:"reused_from_task_id,omitempty" db:"reused_from_task_id"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

const (
	StatusPending    = "pending"
	StatusStaticRun  = "static_running"
	StatusStaticDone = "static_done"
	StatusCacheRun   = "cache_running"
	StatusDone       = "done"
	StatusError      = "error"
)

type StartAnalysisEvent struct {
	TaskID            string `json:"task_id"`
	FileS3Path        string `json:"file_s3_path"`
	ProjectID         string `json:"project_id"`
	CacheProfileHash  string `json:"cache_profile_hash"`
	CacheConfigS3Path string `json:"cache_config_s3_path,omitempty"`
}

// CacheSimulatorConfig — конфиг симулятора кэша, загруженный пользователем для cache-analysis-worker.
type CacheSimulatorConfig struct {
	ID                string    `json:"id" db:"id"`
	UserID            string    `json:"user_id" db:"user_id"`
	DisplayName       string    `json:"display_name" db:"display_name"`
	OriginalFilename  string    `json:"original_filename" db:"original_filename"`
	S3Path            string    `json:"s3_path" db:"s3_path"`
	SizeBytes         int64     `json:"size_bytes" db:"size_bytes"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
}

type AnalysisCompletedEvent struct {
	TaskID         string `json:"task_id"`
	Status         string `json:"status"`
	ArtifactS3Path string `json:"artifact_s3_path,omitempty"`
	Error          string `json:"error,omitempty"`
}

type UploadRequest struct {
	ProjectID string `form:"project_id"`
}

type MetricsResponse struct {
	TaskID            string  `json:"task_id"`
	Status            string  `json:"status"`
	TotalMemoryAccess uint64  `json:"total_memory_accesses"`
	CacheHits         uint64  `json:"cache_hits"`
	CacheMisses       uint64  `json:"cache_misses"`
	HitRate           float64 `json:"hit_rate"`
	MissRate          float64 `json:"miss_rate"`
	OptimizationScore float64 `json:"optimization_score"`
}

type AggregatedMetricsResponse struct {
	TaskID string            `json:"task_id"`
	Status string            `json:"status"`
	Rows   []AggregatedEntry `json:"patterns"`
}

type FileSimulationResultsResponse struct {
	FileID         string            `json:"file_id"`
	Filename       string            `json:"filename"`
	TaskID         string            `json:"task_id"`
	Status         string            `json:"status"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	ReusedFromTask string            `json:"reused_from_task_id,omitempty"`
	Metrics        MetricsResponse   `json:"metrics"`
	Patterns       []AggregatedEntry `json:"patterns"`
}

type FilePatternResultsResponse struct {
	FileID         string            `json:"file_id"`
	Filename       string            `json:"filename"`
	TaskID         string            `json:"task_id"`
	Status         string            `json:"status"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	ReusedFromTask string            `json:"reused_from_task_id,omitempty"`
	Patterns       []AggregatedEntry `json:"patterns"`
}

type AnalysisAdminStats struct {
	TotalFiles int `json:"total_files"`
	Done       int `json:"done"`
	Pending    int `json:"pending"`
	Error      int `json:"error"`
}

type SystemComponentStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type SystemStatus struct {
	Postgres         SystemComponentStatus `json:"postgres"`
	Minio            SystemComponentStatus `json:"minio"`
	Kafka            SystemComponentStatus `json:"kafka"`
	ClickHouse       SystemComponentStatus `json:"clickhouse"`
	StartStaticQueue int64                 `json:"start_static_queue"`
}

type AggregatedEntry struct {
	SequenceIndex      uint32   `json:"sequence_index"`
	SourceFile         string   `json:"source_file"`
	SourceLine         uint32   `json:"source_line"`
	SourceColumn       uint32   `json:"source_column"`
	BaseSymbol         string   `json:"base_symbol"`
	BaseKind           string   `json:"base_kind"`
	Function           string   `json:"function"`
	PatternType        string   `json:"pattern_type"`
	PatternFingerprint string   `json:"pattern_fingerprint"`
	PatternSignature   string   `json:"pattern_signature"`
	AccessKind         string   `json:"access_kind"`
	Affine             uint8    `json:"affine"`
	Stride             *float64 `json:"stride,omitempty"`
	Depth              uint8    `json:"depth"`
	FillFactor         float64  `json:"fill_factor"`
	HasIndexedAddr     uint8    `json:"has_indexed_addressing"`
	IndexedByMemory    uint8    `json:"indexed_by_memory"`
	Conditional        uint8    `json:"conditional"`
	Alignment          *uint32  `json:"alignment,omitempty"`
	WorkingSetBytes    uint64   `json:"working_set_bytes"`
	Dependence         string   `json:"dependence"`
	ContiguousBlock    *uint32  `json:"contiguous_block,omitempty"`
	LoadCount          uint32   `json:"load_count"`
	StoreCount         uint32   `json:"store_count"`
	CacheProfileHash   string   `json:"cache_profile_hash"`
	CacheLevel         string   `json:"cache_level"`
	SourceTaskID       string   `json:"source_task_id,omitempty"`
	MissesTotal        uint64   `json:"misses_total"`
	MissesRead         uint64   `json:"misses_read"`
	MissesWrite        uint64   `json:"misses_write"`
}
