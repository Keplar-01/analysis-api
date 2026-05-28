package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/diploma/analysis-api-service/internal/model"
)

const latestDynamicPatternMetricsCTE = `
WITH latest_dynamic AS (
	SELECT
		sequence_index,
		pattern_fingerprint,
		base_symbol,
		access_kind,
		cache_profile_hash,
		cache_level,
		argMax(misses_total, created_at) AS misses_total,
		argMax(misses_read, created_at) AS misses_read,
		argMax(misses_write, created_at) AS misses_write,
		argMax(source_task_id, created_at) AS source_task_id
	FROM dynamic_pattern_metrics
	WHERE task_id = ?
	GROUP BY
		sequence_index,
		pattern_fingerprint,
		base_symbol,
		access_kind,
		cache_profile_hash,
		cache_level
)`

type ClickHouseRepo struct {
	conn clickhouse.Conn
}

func NewClickHouseRepo(addr, user, password, db string) (*ClickHouseRepo, error) {
	if err := ensureCHSchema(addr, user, password, db); err != nil {
		log.Printf("[clickhouse] ensure schema failed: %v", err)
	}

	conn, err := connectCHWithRetry(addr, user, password, db)
	if err != nil {
		return nil, err
	}
	return &ClickHouseRepo{conn: conn}, nil
}

func ensureCHSchema(addr, user, password, db string) error {
	bootstrap, err := connectCHWithRetry(addr, user, password, "default")
	if err != nil {
		return err
	}
	defer bootstrap.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := bootstrap.Exec(ctx, "CREATE DATABASE IF NOT EXISTS "+db); err != nil {
		return fmt.Errorf("create database: %w", err)
	}

	for _, ddl := range schemaDDL(db) {
		if err := bootstrap.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("apply DDL: %w", err)
		}
	}
	return nil
}

func schemaDDL(db string) []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS ` + db + `.static_patterns (
			task_id             String,
			project_id          String,
			sequence_index      UInt32,
			source_file         String,
			source_line         UInt32,
			source_column       UInt32,
			function            String,
			base_symbol         String,
			base_kind           String,
			access_kind         String,
			pattern_type        String,
			pattern_fingerprint String,
			affine              UInt8,
			stride              Nullable(Float64),
			depth               UInt8,
			has_indexed_addr    UInt8,
			indexed_by_memory   UInt8,
			conditional         UInt8,
			fill_factor         Float64,
			alignment           Nullable(UInt32),
			working_set_bytes   UInt64,
			dependence          String,
			pattern_signature   String,
			contiguous_block    Nullable(UInt32),
			load_count          UInt32,
			store_count         UInt32,
			cache_profile_hash  String,
			artifact_s3_path    String,
			created_at          DateTime DEFAULT now()
		) ENGINE = MergeTree()
		ORDER BY (task_id, source_line, source_column, base_symbol, access_kind)`,

		`ALTER TABLE ` + db + `.static_patterns ADD COLUMN IF NOT EXISTS sequence_index UInt32 AFTER project_id`,

		`CREATE TABLE IF NOT EXISTS ` + db + `.variable_sequences (
			task_id               String,
			project_id            String,
			cache_profile_hash    String,
			base_symbol           String,
			variable_sequence_hash String,
			pattern_count         UInt32,
			created_at            DateTime DEFAULT now()
		) ENGINE = MergeTree()
		ORDER BY (project_id, cache_profile_hash, base_symbol, variable_sequence_hash, task_id)`,

		`CREATE TABLE IF NOT EXISTS ` + db + `.dynamic_pattern_metrics (
			task_id             String,
			sequence_index      UInt32,
			pattern_fingerprint String,
			base_symbol         String,
			access_kind         String,
			cache_profile_hash  String,
			cache_level         String,
			misses_total        UInt64,
			misses_read         UInt64,
			misses_write        UInt64,
			source_task_id      String,
			source_file         String,
			interpreter_version String,
			created_at          DateTime DEFAULT now()
		) ENGINE = MergeTree()
		ORDER BY (task_id, sequence_index, pattern_fingerprint, base_symbol, access_kind, cache_profile_hash, cache_level, created_at)`,

		`ALTER TABLE ` + db + `.dynamic_pattern_metrics ADD COLUMN IF NOT EXISTS sequence_index UInt32 AFTER task_id`,
		`ALTER TABLE ` + db + `.dynamic_pattern_metrics ADD COLUMN IF NOT EXISTS task_id String DEFAULT '' FIRST`,
	}
}

func (r *ClickHouseRepo) Close() error {
	return r.conn.Close()
}

func (r *ClickHouseRepo) GetStaticPatterns(ctx context.Context, taskID string) ([]model.StaticPatternRow, error) {
	rows, err := r.conn.Query(ctx, `
		SELECT
			task_id,
			project_id,
			sequence_index,
			source_file,
			source_line,
			source_column,
			function,
			base_symbol,
			base_kind,
			access_kind,
			pattern_type,
			pattern_fingerprint,
			pattern_signature,
			affine,
			cache_profile_hash,
			fill_factor,
			stride,
			depth,
			has_indexed_addr,
			indexed_by_memory,
			conditional,
			alignment,
			working_set_bytes,
			dependence,
			contiguous_block,
			load_count,
			store_count,
			artifact_s3_path
		FROM static_patterns
		WHERE task_id = ?
		ORDER BY sequence_index, source_file, source_line, source_column, base_symbol, access_kind`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("query static patterns: %w", err)
	}
	defer rows.Close()

	var result []model.StaticPatternRow
	for rows.Next() {
		var row model.StaticPatternRow
		if err := rows.Scan(
			&row.TaskID,
			&row.ProjectID,
			&row.SequenceIndex,
			&row.SourceFile,
			&row.SourceLine,
			&row.SourceColumn,
			&row.Function,
			&row.BaseSymbol,
			&row.BaseKind,
			&row.AccessKind,
			&row.PatternType,
			&row.PatternFingerprint,
			&row.PatternSignature,
			&row.Affine,
			&row.CacheProfileHash,
			&row.FillFactor,
			&row.Stride,
			&row.Depth,
			&row.HasIndexedAddr,
			&row.IndexedByMemory,
			&row.Conditional,
			&row.Alignment,
			&row.WorkingSetBytes,
			&row.Dependence,
			&row.ContiguousBlock,
			&row.LoadCount,
			&row.StoreCount,
			&row.ArtifactS3Path,
		); err != nil {
			return nil, fmt.Errorf("scan static pattern: %w", err)
		}
		result = append(result, row)
	}

	return result, nil
}

func (r *ClickHouseRepo) WriteStaticPatterns(ctx context.Context, rows []model.StaticPatternRow) error {
	if len(rows) == 0 {
		return nil
	}

	batch, err := r.conn.PrepareBatch(ctx, `
		INSERT INTO static_patterns (
			task_id,
			project_id,
			sequence_index,
			source_file,
			source_line,
			source_column,
			function,
			base_symbol,
			base_kind,
			access_kind,
			pattern_type,
			pattern_fingerprint,
			affine,
			stride,
			depth,
			has_indexed_addr,
			indexed_by_memory,
			conditional,
			fill_factor,
			alignment,
			working_set_bytes,
			dependence,
			pattern_signature,
			contiguous_block,
			load_count,
			store_count,
			cache_profile_hash,
			artifact_s3_path
		)`)
	if err != nil {
		return fmt.Errorf("prepare static_patterns batch: %w", err)
	}

	for _, row := range rows {
		if err := batch.Append(
			row.TaskID,
			row.ProjectID,
			row.SequenceIndex,
			row.SourceFile,
			row.SourceLine,
			row.SourceColumn,
			row.Function,
			row.BaseSymbol,
			row.BaseKind,
			row.AccessKind,
			row.PatternType,
			row.PatternFingerprint,
			row.Affine,
			row.Stride,
			row.Depth,
			row.HasIndexedAddr,
			row.IndexedByMemory,
			row.Conditional,
			row.FillFactor,
			row.Alignment,
			row.WorkingSetBytes,
			row.Dependence,
			row.PatternSignature,
			row.ContiguousBlock,
			row.LoadCount,
			row.StoreCount,
			row.CacheProfileHash,
			row.ArtifactS3Path,
		); err != nil {
			return fmt.Errorf("append static pattern row: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("send static pattern batch: %w", err)
	}

	return nil
}

func (r *ClickHouseRepo) WriteDynamicPatternMetrics(ctx context.Context, rows []model.DynamicPatternMetric) error {
	if len(rows) == 0 {
		return nil
	}

	batch, err := r.conn.PrepareBatch(ctx, `
		INSERT INTO dynamic_pattern_metrics (
			task_id,
			sequence_index,
			pattern_fingerprint,
			base_symbol,
			access_kind,
			cache_profile_hash,
			cache_level,
			misses_total,
			misses_read,
			misses_write,
			source_task_id,
			source_file,
			interpreter_version
		)`)
	if err != nil {
		return fmt.Errorf("prepare dynamic pattern batch: %w", err)
	}

	for _, row := range rows {
		if err := batch.Append(
			row.TaskID,
			row.SequenceIndex,
			row.PatternFingerprint,
			row.BaseSymbol,
			row.AccessKind,
			row.CacheProfileHash,
			row.CacheLevel,
			row.MissesTotal,
			row.MissesRead,
			row.MissesWrite,
			row.SourceTaskID,
			row.SourceFile,
			row.InterpreterVersion,
		); err != nil {
			return fmt.Errorf("append dynamic pattern row: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("send dynamic pattern batch: %w", err)
	}

	return nil
}

func (r *ClickHouseRepo) WriteVariableSequences(ctx context.Context, rows []model.VariableSequenceRow) error {
	if len(rows) == 0 {
		return nil
	}

	batch, err := r.conn.PrepareBatch(ctx, `
		INSERT INTO variable_sequences (
			task_id,
			project_id,
			cache_profile_hash,
			base_symbol,
			variable_sequence_hash,
			pattern_count
		)`)
	if err != nil {
		return fmt.Errorf("prepare variable_sequences batch: %w", err)
	}

	for _, row := range rows {
		if err := batch.Append(
			row.TaskID,
			row.ProjectID,
			row.CacheProfileHash,
			row.BaseSymbol,
			row.VariableSequenceHash,
			row.PatternCount,
		); err != nil {
			return fmt.Errorf("append variable sequence row: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("send variable_sequences batch: %w", err)
	}

	return nil
}

func (r *ClickHouseRepo) FindMatchingVariableSequenceTask(ctx context.Context, taskID, cacheProfileHash string) (string, error) {
	query := `
		WITH current_sequence_counts AS (
			SELECT
				variable_sequence_hash,
				pattern_count,
				count() AS role_count
			FROM variable_sequences
			WHERE task_id = ?
			GROUP BY variable_sequence_hash, pattern_count
		),
		current_totals AS (
			SELECT
				coalesce(sum(role_count), 0) AS total_roles,
				count() AS distinct_roles
			FROM current_sequence_counts
		),
		candidate_sequence_counts AS (
			SELECT
				task_id,
				variable_sequence_hash,
				pattern_count,
				count() AS role_count,
				min(created_at) AS first_seen_at
			FROM variable_sequences
			WHERE cache_profile_hash = ?
				AND task_id != ?
			GROUP BY task_id, variable_sequence_hash, pattern_count
		),
		candidate_totals AS (
			SELECT
				task_id,
				sum(role_count) AS total_roles,
				count() AS distinct_roles,
				min(first_seen_at) AS first_seen_at
			FROM candidate_sequence_counts
			GROUP BY task_id
		)
		SELECT candidate.task_id
		FROM candidate_sequence_counts candidate
		INNER JOIN current_sequence_counts current
			ON candidate.variable_sequence_hash = current.variable_sequence_hash
			AND candidate.pattern_count = current.pattern_count
			AND candidate.role_count = current.role_count
		INNER JOIN candidate_totals totals
			ON totals.task_id = candidate.task_id
		GROUP BY candidate.task_id, totals.total_roles, totals.distinct_roles, totals.first_seen_at
		HAVING count() = (SELECT distinct_roles FROM current_totals)
			AND sum(candidate.role_count) = (SELECT total_roles FROM current_totals)
			AND totals.total_roles = (SELECT total_roles FROM current_totals)
			AND totals.distinct_roles = (SELECT distinct_roles FROM current_totals)
		ORDER BY totals.first_seen_at ASC
		LIMIT 1`

	var sourceTaskID string
	if err := r.conn.QueryRow(ctx, query, taskID, cacheProfileHash, taskID).Scan(&sourceTaskID); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("query matching variable sequence task: %w", err)
	}

	return sourceTaskID, nil
}

func (r *ClickHouseRepo) GetAggregatedMetrics(ctx context.Context, taskID string) ([]model.AggregatedEntry, error) {
	query := latestDynamicPatternMetricsCTE + `
		SELECT
			sp.sequence_index,
			sp.source_file,
			sp.source_line,
			sp.source_column,
			sp.base_symbol,
			sp.base_kind,
			sp.function,
			sp.pattern_type,
			sp.pattern_fingerprint,
			sp.pattern_signature,
			sp.access_kind,
			sp.affine,
			sp.stride,
			sp.depth,
			sp.fill_factor,
			sp.has_indexed_addr,
			sp.indexed_by_memory,
			sp.conditional,
			sp.alignment,
			sp.working_set_bytes,
			sp.dependence,
			sp.contiguous_block,
			sp.load_count,
			sp.store_count,
			sp.cache_profile_hash,
			coalesce(ld.cache_level, '')    AS cache_level,
			coalesce(ld.source_task_id, '') AS source_task_id,
			coalesce(ld.misses_total, 0)    AS misses_total,
			coalesce(ld.misses_read, 0)     AS misses_read,
			coalesce(ld.misses_write, 0)    AS misses_write
		FROM static_patterns sp
		LEFT JOIN latest_dynamic ld
			ON sp.sequence_index = ld.sequence_index
			AND sp.pattern_fingerprint = ld.pattern_fingerprint
			AND sp.base_symbol = ld.base_symbol
			AND sp.access_kind = ld.access_kind
			AND sp.cache_profile_hash = ld.cache_profile_hash
		WHERE sp.task_id = ?
		ORDER BY sp.sequence_index, sp.source_file, sp.source_line, sp.source_column, sp.base_symbol, sp.access_kind, ld.cache_level`

	rows, err := r.conn.Query(ctx, query, taskID, taskID)
	if err != nil {
		return nil, fmt.Errorf("query aggregated metrics: %w", err)
	}
	defer rows.Close()

	var result []model.AggregatedEntry
	for rows.Next() {
		var row model.AggregatedEntry
		if err := rows.Scan(
			&row.SequenceIndex,
			&row.SourceFile,
			&row.SourceLine,
			&row.SourceColumn,
			&row.BaseSymbol,
			&row.BaseKind,
			&row.Function,
			&row.PatternType,
			&row.PatternFingerprint,
			&row.PatternSignature,
			&row.AccessKind,
			&row.Affine,
			&row.Stride,
			&row.Depth,
			&row.FillFactor,
			&row.HasIndexedAddr,
			&row.IndexedByMemory,
			&row.Conditional,
			&row.Alignment,
			&row.WorkingSetBytes,
			&row.Dependence,
			&row.ContiguousBlock,
			&row.LoadCount,
			&row.StoreCount,
			&row.CacheProfileHash,
			&row.CacheLevel,
			&row.SourceTaskID,
			&row.MissesTotal,
			&row.MissesRead,
			&row.MissesWrite,
		); err != nil {
			return nil, fmt.Errorf("scan aggregated row: %w", err)
		}
		result = append(result, row)
	}

	return result, nil
}

type TaskMetrics struct {
	TotalMemoryAccesses uint64
	CacheHits           uint64
	CacheMisses         uint64
	HitRate             float64
	MissRate            float64
	OptimizationScore   float64
}

func (r *ClickHouseRepo) GetTopPatterns(ctx context.Context, limit int) ([]TopPatternRow, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := r.conn.Query(ctx, `
		SELECT pattern_type, count() AS cnt
		FROM static_patterns
		WHERE pattern_type != ''
		GROUP BY pattern_type
		ORDER BY cnt DESC
		LIMIT ?`, uint32(limit))
	if err != nil {
		return nil, fmt.Errorf("query top patterns: %w", err)
	}
	defer rows.Close()

	var result []TopPatternRow
	for rows.Next() {
		var row TopPatternRow
		if err := rows.Scan(&row.PatternType, &row.Count); err != nil {
			return nil, fmt.Errorf("scan top pattern: %w", err)
		}
		result = append(result, row)
	}
	return result, nil
}

type TopPatternRow struct {
	PatternType string `json:"pattern_type"`
	Count       uint64 `json:"count"`
}

func (r *ClickHouseRepo) Ping(ctx context.Context) error {
	return r.conn.Ping(ctx)
}

func connectCHWithRetry(addr, user, password, db string) (clickhouse.Conn, error) {
	var conn clickhouse.Conn
	var lastErr error

	for i := range 30 {
		conn, lastErr = clickhouse.Open(&clickhouse.Options{
			Addr: []string{addr},
			Auth: clickhouse.Auth{
				Database: db,
				Username: user,
				Password: password,
			},
		})
		if lastErr == nil {
			if pingErr := conn.Ping(context.Background()); pingErr == nil {
				return conn, nil
			}
		}
		log.Printf("[clickhouse] waiting for connection... attempt %d/30", i+1)
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("clickhouse connection failed after 30 attempts: %w", lastErr)
}
