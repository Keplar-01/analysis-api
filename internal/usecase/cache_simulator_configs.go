package usecase

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/diploma/analysis-api-service/internal/model"
	"github.com/diploma/analysis-api-service/internal/storage"
	"github.com/google/uuid"
)

const (
	maxCacheSimulatorConfigsRegularUser int64 = 10
	maxCacheSimulatorConfigFileBytes    int64 = 256 * 1024
)

var (
	ErrCacheSimulatorConfigRequired    = errors.New("cache_config_id is required")
	ErrCacheSimulatorConfigNotFound    = errors.New("cache simulator configuration not found or not accessible")
	ErrCacheSimulatorConfigQuota       = errors.New("cache simulator configuration quota exceeded (max 10 for regular users)")
	ErrCacheSimulatorConfigInvalidExt  = errors.New("unsupported configuration file extension (only .json is allowed)")
	ErrCacheSimulatorConfigInvalidJSON = errors.New("configuration file must be valid JSON")
	ErrCacheSimulatorConfigTooLarge    = errors.New("configuration file is too large")
)

func isAdmin(role string) bool {
	return strings.EqualFold(strings.TrimSpace(role), "admin")
}

func allowedCacheSimulatorExt(ext string) bool {
	return strings.ToLower(ext) == ".json"
}

func normalizeDisplayName(name, fallback string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		s = strings.TrimSuffix(fallback, filepath.Ext(fallback))
		if s == "" {
			s = fallback
		}
	}
	if utf8.RuneCountInString(s) > 255 {
		rs := []rune(s)
		s = string(rs[:255])
	}
	return s
}

func (uc *AnalysisUseCase) resolveOwnedCacheSimulatorConfig(
	ctx context.Context,
	userID string,
	cacheConfigID string,
) (*model.CacheSimulatorConfig, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user context missing")
	}
	id := strings.TrimSpace(cacheConfigID)
	if id == "" {
		return nil, ErrCacheSimulatorConfigRequired
	}
	cfg, err := uc.repo.GetCacheSimulatorConfigOwnedBy(ctx, id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCacheSimulatorConfigNotFound
		}
		return nil, err
	}
	return cfg, nil
}

func (uc *AnalysisUseCase) ListCacheSimulatorConfigs(ctx context.Context, userID string) ([]model.CacheSimulatorConfig, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user context missing")
	}
	list, err := uc.repo.ListCacheSimulatorConfigsByUser(ctx, userID)
	if list == nil {
		return []model.CacheSimulatorConfig{}, err
	}
	return list, err
}

func (uc *AnalysisUseCase) CreateCacheSimulatorConfig(
	ctx context.Context,
	userID string,
	userRole string,
	displayName string,
	originalFilename string,
	body io.Reader,
	size int64,
) (*model.CacheSimulatorConfig, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user context missing")
	}
	ext := strings.ToLower(filepath.Ext(originalFilename))
	if originalFilename == "" || ext == "" || !allowedCacheSimulatorExt(ext) {
		return nil, ErrCacheSimulatorConfigInvalidExt
	}

	if size > maxCacheSimulatorConfigFileBytes {
		return nil, ErrCacheSimulatorConfigTooLarge
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if int64(len(data)) != size && size > 0 {
		size = int64(len(data))
	}
	if size == 0 {
		size = int64(len(data))
	}
	if size > maxCacheSimulatorConfigFileBytes {
		return nil, ErrCacheSimulatorConfigTooLarge
	}

	var jsonCheck json.RawMessage
	if err := json.Unmarshal(data, &jsonCheck); err != nil || len(jsonCheck) == 0 {
		return nil, ErrCacheSimulatorConfigInvalidJSON
	}

	if !isAdmin(userRole) {
		n, err := uc.repo.CountCacheSimulatorConfigsByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		if int64(n) >= maxCacheSimulatorConfigsRegularUser {
			return nil, ErrCacheSimulatorConfigQuota
		}
	}

	cfgID := uuid.New().String()
	objectKey := fmt.Sprintf("cache-configs/%s/%s%s", userID, cfgID, ext)

	if err := uc.minio.Upload(
		ctx,
		storage.BucketSourceCodes,
		objectKey,
		bytes.NewReader(data),
		size,
		"application/json",
	); err != nil {
		return nil, fmt.Errorf("minio upload: %w", err)
	}

	now := time.Now().UTC()
	cfg := &model.CacheSimulatorConfig{
		ID:               cfgID,
		UserID:           userID,
		DisplayName:      normalizeDisplayName(displayName, originalFilename),
		OriginalFilename: originalFilename,
		S3Path:           storage.BucketSourceCodes + "/" + objectKey,
		SizeBytes:        size,
		CreatedAt:        now,
	}
	if err := uc.repo.CreateCacheSimulatorConfig(ctx, cfg); err != nil {
		return nil, fmt.Errorf("create config record: %w", err)
	}
	return cfg, nil
}

func (uc *AnalysisUseCase) DeleteCacheSimulatorConfig(ctx context.Context, userID string, configID string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user context missing")
	}
	id := strings.TrimSpace(configID)
	if id == "" {
		return ErrCacheSimulatorConfigNotFound
	}
	return uc.repo.DeleteCacheSimulatorConfig(ctx, id, userID)
}
