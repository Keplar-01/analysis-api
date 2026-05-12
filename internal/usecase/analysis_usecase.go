package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	analysisanalyzer "github.com/diploma/analysis-api-service/internal/analyzer"
	"github.com/diploma/analysis-api-service/internal/kafka"
	"github.com/diploma/analysis-api-service/internal/model"
	"github.com/diploma/analysis-api-service/internal/repository"
	"github.com/diploma/analysis-api-service/internal/storage"
	"github.com/google/uuid"
)

var indexedAccessPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(\[[^\]]+\])+`)
const defaultUploadProjectID = "default-upload-project"

var cacheInterpreterUnsupportedRules = []struct {
	pattern *regexp.Regexp
	reason  string
}{
	{pattern: regexp.MustCompile(`(?m)^\s*#`), reason: "cache simulation does not support preprocessor directives like #include or #define"},
	{pattern: regexp.MustCompile(`\b(double|float)\b`), reason: "cache simulation does not support float/double types; use integer-based code for cache stage"},
	{pattern: regexp.MustCompile(`for\s*\(\s*(?:int|long|short|char|float|double|size_t)\b`), reason: "cache simulation does not support declarations inside for (...); declare loop counters before the loop"},
}
var mainFunctionPattern = regexp.MustCompile(`\bmain\s*\(`)

type AnalysisUseCase struct {
	repo               *repository.AnalysisRepository
	chRepo             *repository.ClickHouseRepo
	minio              *storage.MinIOClient
	producer           *kafka.Producer
	analyzer           *analysisanalyzer.Analyzer
	interpreterVersion string
}

func NewAnalysisUseCase(
	repo *repository.AnalysisRepository,
	chRepo *repository.ClickHouseRepo,
	minio *storage.MinIOClient,
	producer *kafka.Producer,
	analyzer *analysisanalyzer.Analyzer,
	interpreterVersion string,
) *AnalysisUseCase {
	return &AnalysisUseCase{
		repo:               repo,
		chRepo:             chRepo,
		minio:              minio,
		producer:           producer,
		analyzer:           analyzer,
		interpreterVersion: interpreterVersion,
	}
}

func (uc *AnalysisUseCase) UploadAndAnalyze(
	ctx context.Context,
	projectID string,
	filename string,
	fileReader io.Reader,
	fileSize int64,
	cacheProfile model.CacheProfile,
) (*model.AnalysisTask, error) {
	projectID = normalizeUploadProjectID(projectID)

	content, err := io.ReadAll(fileReader)
	if err != nil {
		return nil, fmt.Errorf("read upload: %w", err)
	}
	if int64(len(content)) != fileSize && fileSize > 0 {
		fileSize = int64(len(content))
	}
	if fileSize == 0 {
		fileSize = int64(len(content))
	}

	contentHash := hashBytes(content)

	existing, err := uc.repo.FindFileByHash(ctx, projectID, filename, contentHash)
	if err != nil {
		return nil, fmt.Errorf("dedup lookup: %w", err)
	}

	var file *model.File
	if existing != nil {
		file = existing
	} else {
		file, err = uc.persistNewFile(ctx, projectID, filename, content, fileSize, contentHash)
		if err != nil {
			return nil, err
		}
	}

	return uc.startAnalysisOnFile(ctx, file, cacheProfile)
}

func (uc *AnalysisUseCase) RunAnalysisOnExistingFile(
	ctx context.Context,
	fileID string,
	cacheProfile model.CacheProfile,
) (*model.AnalysisTask, error) {
	file, err := uc.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, err
	}
	return uc.startAnalysisOnFile(ctx, file, cacheProfile)
}

func (uc *AnalysisUseCase) GetProjectFiles(ctx context.Context, projectID string) ([]model.File, error) {
	return uc.repo.GetFilesByProjectID(ctx, projectID)
}

func (uc *AnalysisUseCase) GetFileContent(ctx context.Context, fileID string) (*model.File, []byte, error) {
	file, err := uc.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, nil, err
	}

	bucket, key, err := splitS3Path(file.S3Path)
	if err != nil {
		return nil, nil, err
	}

	reader, err := uc.minio.Download(ctx, bucket, key)
	if err != nil {
		return nil, nil, fmt.Errorf("download %s/%s: %w", bucket, key, err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("read object: %w", err)
	}
	return file, content, nil
}

func (uc *AnalysisUseCase) persistNewFile(
	ctx context.Context,
	projectID string,
	filename string,
	content []byte,
	fileSize int64,
	contentHash string,
) (*model.File, error) {
	fileID := uuid.New().String()
	ext := filepath.Ext(filename)
	objectKey := fmt.Sprintf("%s/%s%s", projectID, fileID, ext)

	if err := uc.minio.Upload(
		ctx,
		storage.BucketSourceCodes,
		objectKey,
		bytes.NewReader(content),
		fileSize,
		"text/x-csrc",
	); err != nil {
		return nil, fmt.Errorf("minio upload: %w", err)
	}

	file := &model.File{
		ID:          fileID,
		ProjectID:   projectID,
		Filename:    filename,
		S3Path:      storage.BucketSourceCodes + "/" + objectKey,
		ContentHash: contentHash,
		SizeBytes:   fileSize,
		CreatedAt:   time.Now().UTC(),
	}
	if err := uc.repo.CreateFile(ctx, file); err != nil {
		return nil, fmt.Errorf("create file record: %w", err)
	}
	return file, nil
}

func (uc *AnalysisUseCase) startAnalysisOnFile(
	ctx context.Context,
	file *model.File,
	cacheProfile model.CacheProfile,
) (*model.AnalysisTask, error) {
	taskID := uuid.New().String()
	now := time.Now().UTC()

	task := &model.AnalysisTask{
		ID:               taskID,
		FileID:           file.ID,
		Status:           model.StatusPending,
		Type:             "full_analysis",
		CacheProfileHash: cacheProfile.Hash(),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := uc.repo.CreateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("create task record: %w", err)
	}

	if err := uc.repo.UpdateTaskStatus(ctx, taskID, model.StatusStaticRun); err != nil {
		return nil, fmt.Errorf("update task status: %w", err)
	}
	task.Status = model.StatusStaticRun

	event := model.StartAnalysisEvent{
		TaskID:           taskID,
		FileS3Path:       file.S3Path,
		ProjectID:        file.ProjectID,
		CacheProfileHash: task.CacheProfileHash,
	}
	if err := uc.producer.Publish(ctx, kafka.TopicStartStatic, taskID, event); err != nil {
		return nil, fmt.Errorf("kafka publish: %w", err)
	}

	return task, nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (uc *AnalysisUseCase) HandleStaticCompleted(ctx context.Context, event model.AnalysisCompletedEvent) error {
	if event.Status != "success" {
		return uc.repo.UpdateTaskError(ctx, event.TaskID, taskErrorMessage(event.Error, "static analysis failed"))
	}

	if err := uc.repo.UpdateTaskStatus(ctx, event.TaskID, model.StatusStaticDone); err != nil {
		return err
	}
	if err := uc.repo.UpdateTaskStaticArtifact(ctx, event.TaskID, event.ArtifactS3Path); err != nil {
		return err
	}

	task, err := uc.repo.GetTaskByID(ctx, event.TaskID)
	if err != nil {
		return err
	}
	file, err := uc.repo.GetFileByID(ctx, task.FileID)
	if err != nil {
		return err
	}

	staticRows, err := uc.downloadStaticPatterns(ctx, event.TaskID, file.ProjectID, task.CacheProfileHash, event.ArtifactS3Path)
	if err != nil {
		return fmt.Errorf("download static patterns: %w", err)
	}
	if err := uc.chRepo.WriteStaticPatterns(ctx, staticRows); err != nil {
		return fmt.Errorf("write static patterns: %w", err)
	}
	if len(staticRows) == 0 {
		return uc.repo.UpdateTaskStatus(ctx, event.TaskID, model.StatusDone)
	}

	variableSequenceRows := buildVariableSequenceRows(staticRows)
	if err := uc.chRepo.WriteVariableSequences(ctx, variableSequenceRows); err != nil {
		return fmt.Errorf("write variable sequences: %w", err)
	}

	sourceTaskID, err := uc.chRepo.FindMatchingVariableSequenceTask(ctx, event.TaskID, file.ProjectID, task.CacheProfileHash)
	if err != nil {
		return fmt.Errorf("find variable sequence reuse: %w", err)
	}

	if sourceTaskID != "" {
		sourceTask, err := uc.repo.GetTaskByID(ctx, sourceTaskID)
		if err == nil && sourceTask != nil && sourceTask.Status == model.StatusDone && sourceTask.CacheArtifactPath != "" {
			sourceStaticRows, err := uc.chRepo.GetStaticPatterns(ctx, sourceTaskID)
			if err == nil {
				symbolMap, reusable := buildVariableRoleSymbolMap(sourceStaticRows, staticRows)
				if reusable {
					targetFile, targetSourceContent, err := uc.GetFileContent(ctx, task.FileID)
					if err == nil {
						rawResult, err := uc.downloadCacheResult(ctx, sourceTask.CacheArtifactPath)
						if err == nil {
							remappedResult := remapCacheResult(rawResult, symbolMap, targetFile.Filename)
							dynamicRows := uc.materializeDynamicPatternMetrics(event.TaskID, remappedResult, staticRows, targetSourceContent)
							for index := range dynamicRows {
								dynamicRows[index].SourceTaskID = sourceTaskID
							}
							if err := uc.chRepo.WriteDynamicPatternMetrics(ctx, dynamicRows); err == nil {
								artifactPath, err := uc.uploadCacheArtifact(ctx, event.TaskID, remappedResult)
								if err == nil {
									if err := uc.repo.UpdateTaskReusedFrom(ctx, event.TaskID, sourceTaskID); err != nil {
										return err
									}
									if err := uc.repo.UpdateTaskCacheArtifact(ctx, event.TaskID, artifactPath); err != nil {
										return err
									}
									return uc.repo.UpdateTaskStatus(ctx, event.TaskID, model.StatusDone)
								}
							}
						}
					}
				}
			}
		}
	}

	_, sourceContent, err := uc.GetFileContent(ctx, task.FileID)
	if err != nil {
		return fmt.Errorf("download source file: %w", err)
	}
	if validationErr := validateCacheInterpreterSource(sourceContent); validationErr != nil {
		return uc.repo.UpdateTaskError(ctx, event.TaskID, validationErr.Error())
	}

	if err := uc.repo.UpdateTaskStatus(ctx, event.TaskID, model.StatusCacheRun); err != nil {
		return err
	}

	startEvent := model.StartAnalysisEvent{
		TaskID:           event.TaskID,
		FileS3Path:       file.S3Path,
		ProjectID:        file.ProjectID,
		CacheProfileHash: task.CacheProfileHash,
	}

	return uc.producer.Publish(ctx, kafka.TopicStartCache, event.TaskID, startEvent)
}

func (uc *AnalysisUseCase) HandleCacheCompleted(ctx context.Context, event model.AnalysisCompletedEvent) error {
	if event.Status != "success" {
		return uc.repo.UpdateTaskError(ctx, event.TaskID, taskErrorMessage(event.Error, "cache simulation failed"))
	}

	if err := uc.repo.UpdateTaskCacheArtifact(ctx, event.TaskID, event.ArtifactS3Path); err != nil {
		return err
	}

	staticRows, err := uc.chRepo.GetStaticPatterns(ctx, event.TaskID)
	if err != nil {
		return fmt.Errorf("load static rows: %w", err)
	}

	rawResult, err := uc.downloadCacheResult(ctx, event.ArtifactS3Path)
	if err != nil {
		return fmt.Errorf("download cache artifact: %w", err)
	}

	task, err := uc.repo.GetTaskByID(ctx, event.TaskID)
	if err != nil {
		return err
	}

	_, sourceContent, err := uc.GetFileContent(ctx, task.FileID)
	if err != nil {
		return fmt.Errorf("download source file: %w", err)
	}

	dynamicRows := uc.materializeDynamicPatternMetrics(event.TaskID, rawResult, staticRows, sourceContent)
	if err := uc.chRepo.WriteDynamicPatternMetrics(ctx, dynamicRows); err != nil {
		return fmt.Errorf("write dynamic pattern rows: %w", err)
	}

	return uc.repo.UpdateTaskStatus(ctx, event.TaskID, model.StatusDone)
}

func (uc *AnalysisUseCase) GetTask(ctx context.Context, taskID string) (*model.AnalysisTask, error) {
	return uc.repo.GetTaskByID(ctx, taskID)
}

func (uc *AnalysisUseCase) GetFileSimulationResults(ctx context.Context, fileID string) (*model.FileSimulationResultsResponse, error) {
	file, err := uc.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, err
	}

	task, err := uc.repo.GetLatestTaskByFileID(ctx, fileID)
	if err != nil {
		return nil, err
	}

	metrics, err := uc.ComputeTaskMetrics(ctx, task.ID)
	if err != nil {
		return nil, err
	}

	patterns, err := uc.chRepo.GetAggregatedMetrics(ctx, task.ID)
	if err != nil {
		patterns = []model.AggregatedEntry{}
	}

	return &model.FileSimulationResultsResponse{
		FileID:         file.ID,
		Filename:       file.Filename,
		TaskID:         task.ID,
		Status:         task.Status,
		ErrorMessage:   task.ErrorMessage,
		ReusedFromTask: task.ReusedFromTaskID,
		Metrics:        *metrics,
		Patterns:       patterns,
	}, nil
}

func (uc *AnalysisUseCase) GetFileMetrics(ctx context.Context, fileID string) (*model.MetricsResponse, error) {
	if _, err := uc.repo.GetFileByID(ctx, fileID); err != nil {
		return nil, err
	}

	task, err := uc.repo.GetLatestTaskByFileID(ctx, fileID)
	if err != nil {
		return nil, err
	}

	return uc.ComputeTaskMetrics(ctx, task.ID)
}

func (uc *AnalysisUseCase) GetFilePatterns(ctx context.Context, fileID string) (*model.FilePatternResultsResponse, error) {
	file, err := uc.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, err
	}

	task, err := uc.repo.GetLatestTaskByFileID(ctx, fileID)
	if err != nil {
		return nil, err
	}

	patterns, err := uc.chRepo.GetAggregatedMetrics(ctx, task.ID)
	if err != nil {
		patterns = []model.AggregatedEntry{}
	}

	return &model.FilePatternResultsResponse{
		FileID:         file.ID,
		Filename:       file.Filename,
		TaskID:         task.ID,
		Status:         task.Status,
		ErrorMessage:   task.ErrorMessage,
		ReusedFromTask: task.ReusedFromTaskID,
		Patterns:       patterns,
	}, nil
}

func (uc *AnalysisUseCase) GetTasksByProject(ctx context.Context, projectID string) ([]model.AnalysisTask, error) {
	return uc.repo.GetTasksByProjectID(ctx, projectID)
}

func (uc *AnalysisUseCase) ComputeTaskMetrics(ctx context.Context, taskID string) (*model.MetricsResponse, error) {
	task, err := uc.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	resp := &model.MetricsResponse{TaskID: taskID, Status: task.Status}
	if task.CacheArtifactPath == "" {
		return resp, nil
	}

	raw, err := uc.downloadCacheResult(ctx, task.CacheArtifactPath)
	if err != nil {
		return resp, nil
	}

	l1 := raw.L1
	resp.TotalMemoryAccess = l1.TotalAccesses
	resp.CacheHits = l1.TotalHits
	resp.CacheMisses = l1.TotalMisses

	if l1.TotalAccesses > 0 {
		resp.HitRate = float64(l1.TotalHits) / float64(l1.TotalAccesses)
		resp.MissRate = float64(l1.TotalMisses) / float64(l1.TotalAccesses)
	}
	resp.OptimizationScore = computeOptimizationScore(raw)

	return resp, nil
}

func computeOptimizationScore(raw *model.CacheSimResult) float64 {
	if raw == nil {
		return 0
	}

	score := 0.0
	if raw.L1.TotalAccesses > 0 {
		score = float64(raw.L1.TotalHits) / float64(raw.L1.TotalAccesses) * 90.0
	}
	if raw.L2.TotalAccesses > 0 {
		l2Rate := float64(raw.L2.TotalHits) / float64(raw.L2.TotalAccesses)
		score += l2Rate * 10.0
	}
	if score > 100 {
		score = 100
	}
	return score
}

func validateCacheInterpreterSource(source []byte) error {
	text := string(source)
	reasons := make([]string, 0, len(cacheInterpreterUnsupportedRules)+1)
	if !mainFunctionPattern.MatchString(text) {
		reasons = append(reasons, "cache simulation requires a main() function")
	}
	for _, rule := range cacheInterpreterUnsupportedRules {
		if rule.pattern.MatchString(text) {
			reasons = append(reasons, rule.reason)
		}
	}
	if len(reasons) == 0 {
		return nil
	}
	return fmt.Errorf("cache stage skipped: source is unsupported by the current cache interpreter: %s", strings.Join(uniqueStrings(reasons), "; "))
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func taskErrorMessage(message, fallback string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func normalizeUploadProjectID(projectID string) string {
	trimmed := strings.TrimSpace(projectID)
	if trimmed == "" {
		return defaultUploadProjectID
	}
	return trimmed
}

func (uc *AnalysisUseCase) GetAnalysisAdminStats(ctx context.Context) (*model.AnalysisAdminStats, error) {
	return uc.repo.GetAdminStats(ctx)
}

func (uc *AnalysisUseCase) downloadCacheResult(ctx context.Context, artifactPath string) (*model.CacheSimResult, error) {
	bucket, key, err := splitS3Path(artifactPath)
	if err != nil {
		return nil, err
	}

	reader, err := uc.minio.Download(ctx, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("download %s/%s: %w", bucket, key, err)
	}
	defer reader.Close()

	var result model.CacheSimResult
	if err := json.NewDecoder(reader).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode cache artifact: %w", err)
	}

	return &result, nil
}

func (uc *AnalysisUseCase) materializeDynamicPatternMetrics(
	taskID string,
	raw *model.CacheSimResult,
	staticRows []model.StaticPatternRow,
	sourceContent []byte,
) []model.DynamicPatternMetric {
	arrayMetricsBySymbol := make(map[string][]model.ArrayCacheMetric)
	for _, arrayMetric := range raw.Arrays {
		arrayMetricsBySymbol[arrayMetric.ArrayName] = append(arrayMetricsBySymbol[arrayMetric.ArrayName], arrayMetric)
	}
	sourceLines := strings.Split(string(sourceContent), "\n")

	type materializedKey struct {
		patternFingerprint string
		baseSymbol         string
		accessKind         string
		cacheProfileHash   string
		cacheLevel         string
	}

	uniqueRows := make(map[materializedKey]model.DynamicPatternMetric)
	for _, staticRow := range staticRows {
		lookupSymbol := staticRow.BaseSymbol
		if len(arrayMetricsBySymbol[lookupSymbol]) == 0 {
			if resolved := resolveArrayNameFromSource(sourceLines, staticRow.SourceLine, staticRow.SourceColumn, staticRow.AccessKind, arrayMetricsBySymbol); resolved != "" {
				lookupSymbol = resolved
			}
		}

		arrayMetrics := arrayMetricsBySymbol[lookupSymbol]
		for _, arrayMetric := range arrayMetrics {
			key := materializedKey{
				patternFingerprint: staticRow.PatternFingerprint,
				baseSymbol:         staticRow.BaseSymbol,
				accessKind:         staticRow.AccessKind,
				cacheProfileHash:   staticRow.CacheProfileHash,
				cacheLevel:         arrayMetric.CacheLevel,
			}
			uniqueRows[key] = model.DynamicPatternMetric{
				PatternFingerprint: staticRow.PatternFingerprint,
				BaseSymbol:         staticRow.BaseSymbol,
				AccessKind:         staticRow.AccessKind,
				CacheProfileHash:   staticRow.CacheProfileHash,
				CacheLevel:         arrayMetric.CacheLevel,
				MissesTotal:        arrayMetric.MissesTotal,
				MissesRead:         arrayMetric.MissesRead,
				MissesWrite:        arrayMetric.MissesWrite,
				SourceTaskID:       taskID,
				SourceFile:         raw.SourceFile,
				InterpreterVersion: uc.interpreterVersion,
			}
		}
	}

	result := make([]model.DynamicPatternMetric, 0, len(uniqueRows))
	for _, row := range uniqueRows {
		result = append(result, row)
	}

	return result
}

type indexedAccessCandidate struct {
	base  string
	start int
	end   int
}

func resolveArrayNameFromSource(
	sourceLines []string,
	sourceLine, sourceColumn uint32,
	accessKind string,
	available map[string][]model.ArrayCacheMetric,
) string {
	line, ok := chooseSourceLine(sourceLines, sourceLine)
	if !ok {
		return ""
	}

	allCandidates := extractIndexedAccessCandidates(line)
	if len(allCandidates) == 0 {
		return ""
	}

	lhsCandidates, rhsCandidates := splitAssignmentCandidates(line)
	if accessKind == "store" && len(lhsCandidates) > 0 {
		if base := firstAvailableCandidate(lhsCandidates, available); base != "" {
			return base
		}
		return lhsCandidates[0].base
	}

	pool := rhsCandidates
	if len(pool) == 0 {
		pool = allCandidates
	}

	if base := nearestAvailableCandidate(pool, sourceColumn, available); base != "" {
		return base
	}
	if base := nearestCandidate(pool, sourceColumn); base != "" {
		return base
	}
	return ""
}

func chooseSourceLine(sourceLines []string, sourceLine uint32) (string, bool) {
	if sourceLine > 0 {
		lineIndex := int(sourceLine - 1)
		if lineIndex >= 0 && lineIndex < len(sourceLines) {
			line := sourceLines[lineIndex]
			if len(extractIndexedAccessCandidates(line)) > 0 {
				return line, true
			}
		}
	}

	for _, line := range sourceLines {
		if len(extractIndexedAccessCandidates(line)) > 0 {
			return line, true
		}
	}
	return "", false
}

func splitAssignmentCandidates(line string) ([]indexedAccessCandidate, []indexedAccessCandidate) {
	pos := strings.Index(line, "=")
	if pos == -1 {
		return nil, extractIndexedAccessCandidates(line)
	}
	return extractIndexedAccessCandidates(line[:pos]), extractIndexedAccessCandidates(line[pos+1:])
}

func extractIndexedAccessCandidates(line string) []indexedAccessCandidate {
	matches := indexedAccessPattern.FindAllStringIndex(line, -1)
	result := make([]indexedAccessCandidate, 0, len(matches))
	for _, match := range matches {
		text := line[match[0]:match[1]]
		bracket := strings.IndexByte(text, '[')
		if bracket <= 0 {
			continue
		}
		result = append(result, indexedAccessCandidate{
			base:  text[:bracket],
			start: match[0],
			end:   match[1] - 1,
		})
	}
	return result
}

func firstAvailableCandidate(candidates []indexedAccessCandidate, available map[string][]model.ArrayCacheMetric) string {
	for _, candidate := range candidates {
		if len(available[candidate.base]) > 0 {
			return candidate.base
		}
	}
	return ""
}

func nearestAvailableCandidate(candidates []indexedAccessCandidate, sourceColumn uint32, available map[string][]model.ArrayCacheMetric) string {
	best := ""
	bestDistance := -1
	for _, candidate := range candidates {
		if len(available[candidate.base]) == 0 {
			continue
		}
		distance := candidateDistance(candidate, sourceColumn)
		if bestDistance == -1 || distance < bestDistance {
			best = candidate.base
			bestDistance = distance
		}
	}
	return best
}

func nearestCandidate(candidates []indexedAccessCandidate, sourceColumn uint32) string {
	best := ""
	bestDistance := -1
	for _, candidate := range candidates {
		distance := candidateDistance(candidate, sourceColumn)
		if bestDistance == -1 || distance < bestDistance {
			best = candidate.base
			bestDistance = distance
		}
	}
	return best
}

func candidateDistance(candidate indexedAccessCandidate, sourceColumn uint32) int {
	if sourceColumn == 0 {
		return 0
	}
	column := int(sourceColumn - 1)
	if column < candidate.start {
		return candidate.start - column
	}
	if column > candidate.end {
		return column - candidate.end
	}
	return 0
}

func isIdentifierChar(ch byte) bool {
	return ch == '_' || (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

type variableRoleDescriptor struct {
	TaskID           string
	ProjectID        string
	CacheProfileHash string
	BaseSymbol       string
	Sequence         string
	PatternCount     uint32
}

type variableRoleGroup struct {
	Sequence string
	Symbols  []string
}

func buildVariableRoleDescriptors(staticRows []model.StaticPatternRow) []variableRoleDescriptor {
	if len(staticRows) == 0 {
		return nil
	}

	grouped := make(map[string][]model.StaticPatternRow)
	for _, row := range staticRows {
		grouped[row.BaseSymbol] = append(grouped[row.BaseSymbol], row)
	}

	baseSymbols := make([]string, 0, len(grouped))
	for baseSymbol := range grouped {
		baseSymbols = append(baseSymbols, baseSymbol)
	}
	sort.Strings(baseSymbols)

	descriptors := make([]variableRoleDescriptor, 0, len(baseSymbols))
	for _, baseSymbol := range baseSymbols {
		rows := grouped[baseSymbol]
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].SequenceIndex != rows[j].SequenceIndex {
				return rows[i].SequenceIndex < rows[j].SequenceIndex
			}
			if rows[i].SourceFile != rows[j].SourceFile {
				return rows[i].SourceFile < rows[j].SourceFile
			}
			if rows[i].SourceLine != rows[j].SourceLine {
				return rows[i].SourceLine < rows[j].SourceLine
			}
			if rows[i].SourceColumn != rows[j].SourceColumn {
				return rows[i].SourceColumn < rows[j].SourceColumn
			}
			if rows[i].AccessKind != rows[j].AccessKind {
				return rows[i].AccessKind < rows[j].AccessKind
			}
			return sequenceSignature(rows[i]) < sequenceSignature(rows[j])
		})

		var sequence strings.Builder
		for index, row := range rows {
			if index > 0 {
				sequence.WriteString("->")
			}
			sequence.WriteString(row.AccessKind)
			sequence.WriteByte('|')
			sequence.WriteString(sequenceSignature(row))
		}

		descriptors = append(descriptors, variableRoleDescriptor{
			TaskID:           rows[0].TaskID,
			ProjectID:        rows[0].ProjectID,
			CacheProfileHash: rows[0].CacheProfileHash,
			BaseSymbol:       baseSymbol,
			Sequence:         sequence.String(),
			PatternCount:     uint32(len(rows)),
		})
	}

	return descriptors
}

func buildVariableSequenceRows(staticRows []model.StaticPatternRow) []model.VariableSequenceRow {
	descriptors := buildVariableRoleDescriptors(staticRows)
	result := make([]model.VariableSequenceRow, 0, len(descriptors))
	for _, descriptor := range descriptors {
		result = append(result, model.VariableSequenceRow{
			TaskID:               descriptor.TaskID,
			ProjectID:            descriptor.ProjectID,
			CacheProfileHash:     descriptor.CacheProfileHash,
			BaseSymbol:           descriptor.BaseSymbol,
			VariableSequenceHash: hashBytes([]byte(descriptor.Sequence)),
			PatternCount:         descriptor.PatternCount,
		})
	}

	return result
}

func buildVariableRoleGroups(staticRows []model.StaticPatternRow) []variableRoleGroup {
	descriptors := buildVariableRoleDescriptors(staticRows)
	bySequence := make(map[string][]string)
	for _, descriptor := range descriptors {
		bySequence[descriptor.Sequence] = append(bySequence[descriptor.Sequence], descriptor.BaseSymbol)
	}

	sequences := make([]string, 0, len(bySequence))
	for sequence := range bySequence {
		sequences = append(sequences, sequence)
	}
	sort.Strings(sequences)

	groups := make([]variableRoleGroup, 0, len(sequences))
	for _, sequence := range sequences {
		symbols := bySequence[sequence]
		sort.Strings(symbols)
		groups = append(groups, variableRoleGroup{Sequence: sequence, Symbols: symbols})
	}

	return groups
}

func buildVariableRoleSymbolMap(sourceRows, targetRows []model.StaticPatternRow) (map[string]string, bool) {
	sourceGroups := buildVariableRoleGroups(sourceRows)
	targetGroups := buildVariableRoleGroups(targetRows)
	if len(sourceGroups) != len(targetGroups) {
		return nil, false
	}

	mapping := make(map[string]string, len(sourceGroups))
	for index := range sourceGroups {
		sourceGroup := sourceGroups[index]
		targetGroup := targetGroups[index]
		if sourceGroup.Sequence != targetGroup.Sequence || len(sourceGroup.Symbols) != len(targetGroup.Symbols) {
			return nil, false
		}
		if len(sourceGroup.Symbols) != 1 {
			return nil, false
		}
		mapping[sourceGroup.Symbols[0]] = targetGroup.Symbols[0]
	}

	return mapping, true
}

func remapCacheResult(raw *model.CacheSimResult, symbolMap map[string]string, sourceFile string) *model.CacheSimResult {
	if raw == nil {
		return nil
	}

	remapped := *raw
	remapped.SourceFile = sourceFile
	remapped.Arrays = make([]model.ArrayCacheMetric, 0, len(raw.Arrays))
	for _, arrayMetric := range raw.Arrays {
		cloned := arrayMetric
		if mappedSymbol, ok := symbolMap[arrayMetric.ArrayName]; ok {
			cloned.ArrayName = mappedSymbol
		}
		remapped.Arrays = append(remapped.Arrays, cloned)
	}

	return &remapped
}

func (uc *AnalysisUseCase) uploadCacheArtifact(ctx context.Context, taskID string, raw *model.CacheSimResult) (string, error) {
	payload, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal cache artifact: %w", err)
	}

	objectKey := fmt.Sprintf("%s/cache-out.json", taskID)
	if err := uc.minio.Upload(
		ctx,
		storage.BucketAnalysisArtifacts,
		objectKey,
		bytes.NewReader(payload),
		int64(len(payload)),
		"application/json",
	); err != nil {
		return "", fmt.Errorf("upload cache artifact: %w", err)
	}

	return storage.BucketAnalysisArtifacts + "/" + objectKey, nil
}

func sequenceSignature(row model.StaticPatternRow) string {
	if row.PatternSignature != "" {
		return row.PatternSignature
	}
	return row.PatternFingerprint
}

func splitS3Path(s3Path string) (string, string, error) {
	parts := strings.SplitN(s3Path, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid s3 path: %s", s3Path)
	}
	return parts[0], parts[1], nil
}

func (uc *AnalysisUseCase) RunStaticAnalyzerDebug(ctx context.Context, filename string, content []byte) ([]model.StaticArtifactPattern, error) {
	if uc.analyzer == nil {
		return nil, fmt.Errorf("analyzer debug is not configured")
	}
	return uc.analyzer.RunSource(ctx, filename, content)
}

func (uc *AnalysisUseCase) downloadStaticPatterns(ctx context.Context, taskID, projectID, cacheProfileHash, artifactPath string) ([]model.StaticPatternRow, error) {
	bucket, key, err := splitS3Path(artifactPath)
	if err != nil {
		return nil, err
	}

	reader, err := uc.minio.Download(ctx, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("download %s/%s: %w", bucket, key, err)
	}
	defer reader.Close()

	var patterns []model.StaticArtifactPattern
	if err := json.NewDecoder(reader).Decode(&patterns); err != nil {
		return nil, fmt.Errorf("decode static artifact: %w", err)
	}

	rows := make([]model.StaticPatternRow, 0, len(patterns))
	for index := range patterns {
		rows = append(rows, model.StaticPatternRow{
			TaskID:             taskID,
			ProjectID:          projectID,
			SequenceIndex:      patterns[index].SequenceIndex,
			SourceFile:         patterns[index].SourceFile,
			SourceLine:         uint32(nonNegativeInt(patterns[index].SourceLine)),
			SourceColumn:       uint32(nonNegativeInt(patterns[index].SourceColumn)),
			Function:           patterns[index].Function,
			BaseSymbol:         patterns[index].BaseSymbol,
			BaseKind:           patterns[index].BaseKind,
			AccessKind:         patterns[index].AccessKind,
			PatternType:        patterns[index].PatternType,
			PatternFingerprint: patterns[index].PatternFingerprint,
			Affine:             boolToUInt8(patterns[index].Affine),
			CacheProfileHash:   cacheProfileHash,
			FillFactor:         patterns[index].FillFactor,
			Stride:             patterns[index].Stride,
			Depth:              uint8(nonNegativeInt(patterns[index].Depth)),
			HasIndexedAddr:     boolToUInt8(patterns[index].HasIndexedAddr),
			IndexedByMemory:    boolToUInt8(patterns[index].IndexedByMemory),
			Conditional:        boolToUInt8(patterns[index].Conditional),
			Alignment:          intPtrToUint32Ptr(patterns[index].Alignment),
			WorkingSetBytes:    uint64(nonNegativeInt(patterns[index].WorkingSetBytes)),
			Dependence:         patterns[index].Dependence,
			PatternSignature:   patterns[index].PatternSig,
			ContiguousBlock:    intPtrToUint32Ptr(patterns[index].ContiguousBlock),
			LoadCount:          uint32(nonNegativeInt(patterns[index].LoadCount)),
			StoreCount:         uint32(nonNegativeInt(patterns[index].StoreCount)),
			ArtifactS3Path:     artifactPath,
		})
	}

	return rows, nil
}

func boolToUInt8(value bool) uint8 {
	if value {
		return 1
	}
	return 0
}

func intPtrToUint32Ptr(value *int) *uint32 {
	if value == nil {
		return nil
	}
	converted := uint32(nonNegativeInt(*value))
	return &converted
}

func nonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
