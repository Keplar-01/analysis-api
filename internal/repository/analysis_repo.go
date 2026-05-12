package repository

import (
	"context"
	"database/sql"

	"github.com/diploma/analysis-api-service/internal/model"
	"github.com/jmoiron/sqlx"
)

type AnalysisRepository struct {
	db *sqlx.DB
}

func NewAnalysisRepository(db *sqlx.DB) *AnalysisRepository {
	return &AnalysisRepository{db: db}
}

func (r *AnalysisRepository) CreateFile(ctx context.Context, file *model.File) error {
	query := `INSERT INTO files (id, project_id, filename, s3_path, content_hash, size_bytes, created_at)
	          VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.ExecContext(
		ctx,
		query,
		file.ID,
		file.ProjectID,
		file.Filename,
		file.S3Path,
		file.ContentHash,
		file.SizeBytes,
		file.CreatedAt,
	)
	return err
}

func (r *AnalysisRepository) FindFileByHash(ctx context.Context, projectID, filename, contentHash string) (*model.File, error) {
	var file model.File
	query := `
		SELECT id, project_id, filename, s3_path, content_hash, size_bytes, created_at
		FROM files
		WHERE project_id = $1 AND filename = $2 AND content_hash = $3
		ORDER BY created_at DESC
		LIMIT 1`
	if err := r.db.GetContext(ctx, &file, query, projectID, filename, contentHash); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &file, nil
}

func (r *AnalysisRepository) GetFilesByProjectID(ctx context.Context, projectID string) ([]model.File, error) {
	var files []model.File
	query := `
		SELECT id, project_id, filename, s3_path, content_hash, size_bytes, created_at
		FROM files
		WHERE project_id = $1
		ORDER BY created_at DESC`
	err := r.db.SelectContext(ctx, &files, query, projectID)
	return files, err
}

func (r *AnalysisRepository) CreateTask(ctx context.Context, task *model.AnalysisTask) error {
	query := `
		INSERT INTO analysis_tasks (
			id, file_id, status, type, error_message, cache_profile_hash,
			static_artifact_s3_path, cache_artifact_s3_path, reused_from_task_id,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := r.db.ExecContext(
		ctx,
		query,
		task.ID,
		task.FileID,
		task.Status,
		task.Type,
		task.ErrorMessage,
		task.CacheProfileHash,
		task.StaticArtifactPath,
		task.CacheArtifactPath,
		task.ReusedFromTaskID,
		task.CreatedAt,
		task.UpdatedAt,
	)
	return err
}

func (r *AnalysisRepository) UpdateTaskStatus(ctx context.Context, taskID, status string) error {
	query := `UPDATE analysis_tasks SET status = $1, error_message = '', updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, status, taskID)
	return err
}

func (r *AnalysisRepository) UpdateTaskError(ctx context.Context, taskID, errorMessage string) error {
	query := `UPDATE analysis_tasks SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, model.StatusError, errorMessage, taskID)
	return err
}

func (r *AnalysisRepository) GetTaskByID(ctx context.Context, taskID string) (*model.AnalysisTask, error) {
	var task model.AnalysisTask
	query := `
		SELECT
			id, file_id, status, type, error_message, cache_profile_hash,
			static_artifact_s3_path, cache_artifact_s3_path, reused_from_task_id,
			created_at, updated_at
		FROM analysis_tasks
		WHERE id = $1`
	err := r.db.GetContext(ctx, &task, query, taskID)
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *AnalysisRepository) GetTasksByProjectID(ctx context.Context, projectID string) ([]model.AnalysisTask, error) {
	var tasks []model.AnalysisTask
	query := `
		SELECT
			at.id, at.file_id, at.status, at.type, at.error_message, at.cache_profile_hash,
			at.static_artifact_s3_path, at.cache_artifact_s3_path, at.reused_from_task_id,
			at.created_at, at.updated_at
		FROM analysis_tasks at
		JOIN files f ON f.id = at.file_id
		WHERE f.project_id = $1
		ORDER BY at.created_at DESC`
	err := r.db.SelectContext(ctx, &tasks, query, projectID)
	return tasks, err
}

func (r *AnalysisRepository) GetLatestTaskByFileID(ctx context.Context, fileID string) (*model.AnalysisTask, error) {
	var task model.AnalysisTask
	query := `
		SELECT
			id, file_id, status, type, error_message, cache_profile_hash,
			static_artifact_s3_path, cache_artifact_s3_path, reused_from_task_id,
			created_at, updated_at
		FROM analysis_tasks
		WHERE file_id = $1
		ORDER BY created_at DESC
		LIMIT 1`
	err := r.db.GetContext(ctx, &task, query, fileID)
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *AnalysisRepository) UpdateTaskStaticArtifact(ctx context.Context, taskID, artifactPath string) error {
	query := `UPDATE analysis_tasks SET static_artifact_s3_path = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, artifactPath, taskID)
	return err
}

func (r *AnalysisRepository) UpdateTaskCacheArtifact(ctx context.Context, taskID, artifactPath string) error {
	query := `UPDATE analysis_tasks SET cache_artifact_s3_path = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, artifactPath, taskID)
	return err
}

func (r *AnalysisRepository) UpdateTaskReusedFrom(ctx context.Context, taskID, sourceTaskID string) error {
	query := `UPDATE analysis_tasks SET reused_from_task_id = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, sourceTaskID, taskID)
	return err
}

func (r *AnalysisRepository) GetFileByID(ctx context.Context, fileID string) (*model.File, error) {
	var file model.File
	query := `SELECT id, project_id, filename, s3_path, created_at FROM files WHERE id = $1`
	err := r.db.GetContext(ctx, &file, query, fileID)
	if err != nil {
		return nil, err
	}
	return &file, nil
}

func (r *AnalysisRepository) GetAdminStats(ctx context.Context) (*model.AnalysisAdminStats, error) {
	stats := &model.AnalysisAdminStats{}

	if err := r.db.GetContext(ctx, &stats.TotalFiles, `SELECT COUNT(*) FROM files`); err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM analysis_tasks
		GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		switch status {
		case model.StatusDone:
			stats.Done = count
		case model.StatusError:
			stats.Error = count
		case model.StatusPending, model.StatusStaticRun, model.StatusStaticDone, model.StatusCacheRun:
			stats.Pending += count
		}
	}
	return stats, rows.Err()
}

func (r *AnalysisRepository) PingDB(ctx context.Context) error {
	return r.db.PingContext(ctx)
}
