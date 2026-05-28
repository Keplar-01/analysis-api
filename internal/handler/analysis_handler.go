package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/diploma/analysis-api-service/internal/model"
	"github.com/diploma/analysis-api-service/internal/repository"
	"github.com/diploma/analysis-api-service/internal/usecase"
	"github.com/gin-gonic/gin"
)

func writeAnalysisUploadError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, usecase.ErrCacheSimulatorConfigRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return true
	case errors.Is(err, usecase.ErrCacheSimulatorConfigNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return true
	case errors.Is(err, usecase.ErrCacheSimulatorConfigQuota):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return true
	case errors.Is(err, usecase.ErrQuotaExceeded):
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		return true
	case errors.Is(err, usecase.ErrCacheSimulatorConfigInvalidExt):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return true
	case errors.Is(err, usecase.ErrCacheSimulatorConfigInvalidJSON):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return true
	case errors.Is(err, usecase.ErrCacheSimulatorConfigInvalidSchema):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return true
	case errors.Is(err, usecase.ErrCacheSimulatorConfigTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
		return true
	case errors.Is(err, usecase.ErrFileSoftDeleted):
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return true
	default:
		return false
	}
}

type AnalysisHandler struct {
	analysisUC *usecase.AnalysisUseCase
	chRepo     *repository.ClickHouseRepo
}

func NewAnalysisHandler(analysisUC *usecase.AnalysisUseCase, chRepo *repository.ClickHouseRepo) *AnalysisHandler {
	return &AnalysisHandler{analysisUC: analysisUC, chRepo: chRepo}
}

type errorResponse struct {
	Error string `json:"error"`
}

type taskEnvelope struct {
	Message string              `json:"message"`
	Task    *model.AnalysisTask `json:"task"`
}

type cacheConfigListEnvelope struct {
	Configs []model.CacheSimulatorConfig `json:"configs"`
}

type cacheConfigEnvelope struct {
	Config *model.CacheSimulatorConfig `json:"config"`
}

type staticPatternsEnvelope struct {
	TaskID   string                   `json:"task_id"`
	Status   string                   `json:"status"`
	Patterns []model.StaticPatternRow `json:"patterns"`
}

type projectTasksEnvelope struct {
	Tasks []model.AnalysisTask `json:"tasks"`
}

type projectFilesEnvelope struct {
	Files []model.File `json:"files"`
}

type staticAnalyzerDebugResponse struct {
	Filename string                        `json:"filename"`
	Patterns []model.StaticArtifactPattern `json:"patterns"`
}

func (h *AnalysisHandler) Upload(c *gin.Context) {
	projectID := c.PostForm("project_id")
	userID := c.GetString("user_id")
	quota := c.GetInt("analysis_quota")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	cacheConfigID := strings.TrimSpace(c.PostForm("cache_config_id"))

	cacheProfile, err := parseCacheProfile(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.analysisUC.UploadAndAnalyze(
		c.Request.Context(),
		userID,
		quota,
		projectID,
		header.Filename,
		file,
		header.Size,
		cacheConfigID,
		cacheProfile,
	)
	if err != nil {
		if writeAnalysisUploadError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "file uploaded, analysis started",
		"task":    task,
	})
}

func (h *AnalysisHandler) ListCacheConfigs(c *gin.Context) {
	userID := c.GetString("user_id")
	list, err := h.analysisUC.ListCacheSimulatorConfigs(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"configs": list})
}

func (h *AnalysisHandler) CreateCacheConfig(c *gin.Context) {
	userID := c.GetString("user_id")
	userRole := c.GetString("role")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	displayName := strings.TrimSpace(c.PostForm("name"))

	cfg, err := h.analysisUC.CreateCacheSimulatorConfig(
		c.Request.Context(),
		userID,
		userRole,
		displayName,
		header.Filename,
		file,
		header.Size,
	)
	if err != nil {
		if writeAnalysisUploadError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"config": cfg})
}

func (h *AnalysisHandler) DeleteCacheConfig(c *gin.Context) {
	userID := c.GetString("user_id")
	id := strings.TrimSpace(c.Param("config_id"))

	if err := h.analysisUC.DeleteCacheSimulatorConfig(c.Request.Context(), userID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "configuration not found"})
			return
		}
		if writeAnalysisUploadError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AnalysisHandler) GetTaskStatus(c *gin.Context) {
	taskID := c.Param("task_id")

	task, err := h.analysisUC.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	c.JSON(http.StatusOK, task)
}

func (h *AnalysisHandler) GetTaskMetrics(c *gin.Context) {
	taskID := c.Param("task_id")

	resp, err := h.analysisUC.ComputeTaskMetrics(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *AnalysisHandler) GetAggregatedMetrics(c *gin.Context) {
	taskID := c.Param("task_id")

	task, err := h.analysisUC.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	rows, err := h.chRepo.GetAggregatedMetrics(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"task_id":  taskID,
			"status":   task.Status,
			"patterns": []interface{}{},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id":  taskID,
		"status":   task.Status,
		"patterns": rows,
	})
}

func (h *AnalysisHandler) GetTaskStaticPatterns(c *gin.Context) {
	taskID := c.Param("task_id")

	task, err := h.analysisUC.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	rows, err := h.chRepo.GetStaticPatterns(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"task_id":  taskID,
			"status":   task.Status,
			"patterns": []interface{}{},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id":  taskID,
		"status":   task.Status,
		"patterns": rows,
	})
}

func (h *AnalysisHandler) DebugStaticAnalyzer(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read file"})
		return
	}

	patterns, err := h.analysisUC.RunStaticAnalyzerDebug(c.Request.Context(), header.Filename, content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"filename": header.Filename,
		"patterns": patterns,
	})
}

func parseCacheProfile(c *gin.Context) (model.CacheProfile, error) {
	profile := model.DefaultCacheProfile()

	var err error
	if profile.NumLevels, err = parseUint8Form(c, "num_levels", profile.NumLevels); err != nil {
		return model.CacheProfile{}, err
	}
	if profile.L1SizeKB, err = parseUint32Form(c, "l1_size_kb", profile.L1SizeKB); err != nil {
		return model.CacheProfile{}, err
	}
	if profile.L1LineSize, err = parseUint32Form(c, "l1_line_size", profile.L1LineSize); err != nil {
		return model.CacheProfile{}, err
	}
	if profile.L1Associativity, err = parseUint8Form(c, "l1_associativity", profile.L1Associativity); err != nil {
		return model.CacheProfile{}, err
	}
	if profile.L2SizeKB, err = parseUint32Form(c, "l2_size_kb", profile.L2SizeKB); err != nil {
		return model.CacheProfile{}, err
	}
	if profile.L2LineSize, err = parseUint32Form(c, "l2_line_size", profile.L2LineSize); err != nil {
		return model.CacheProfile{}, err
	}
	if profile.L2Associativity, err = parseUint8Form(c, "l2_associativity", profile.L2Associativity); err != nil {
		return model.CacheProfile{}, err
	}
	if profile.L3SizeKB, err = parseUint32Form(c, "l3_size_kb", profile.L3SizeKB); err != nil {
		return model.CacheProfile{}, err
	}
	if profile.L3LineSize, err = parseUint32Form(c, "l3_line_size", profile.L3LineSize); err != nil {
		return model.CacheProfile{}, err
	}
	if profile.L3Associativity, err = parseUint8Form(c, "l3_associativity", profile.L3Associativity); err != nil {
		return model.CacheProfile{}, err
	}

	return profile, nil
}

func parseUint32Form(c *gin.Context, field string, fallback uint32) (uint32, error) {
	raw := c.PostForm(field)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be an unsigned integer", field)
	}
	return uint32(value), nil
}

func parseUint8Form(c *gin.Context, field string, fallback uint8) (uint8, error) {
	raw := c.PostForm(field)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseUint(raw, 10, 8)
	if err != nil {
		return 0, fmt.Errorf("%s must be an unsigned integer", field)
	}
	return uint8(value), nil
}

func (h *AnalysisHandler) GetProjectTasks(c *gin.Context) {
	projectID := c.Param("project_id")

	tasks, err := h.analysisUC.GetTasksByProject(c.Request.Context(), projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func (h *AnalysisHandler) SoftDeleteFile(c *gin.Context) {
	fileID := strings.TrimSpace(c.Param("file_id"))
	userID := c.GetString("user_id")
	role := c.GetString("role")

	err := h.analysisUC.SoftDeleteFile(c.Request.Context(), userID, role, fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		if errors.Is(err, usecase.ErrFileDeleteForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AnalysisHandler) GetProjectFiles(c *gin.Context) {
	projectID := c.Param("project_id")

	files, err := h.analysisUC.GetProjectFiles(c.Request.Context(), projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if files == nil {
		files = []model.File{}
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

func (h *AnalysisHandler) GetFileContent(c *gin.Context) {
	fileID := c.Param("file_id")

	file, content, err := h.analysisUC.GetFileContent(c.Request.Context(), fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, file.Filename))
	c.Data(http.StatusOK, "text/plain; charset=utf-8", content)
}

func (h *AnalysisHandler) GetFileSimulationResults(c *gin.Context) {
	fileID := c.Param("file_id")

	resp, err := h.analysisUC.GetFileSimulationResults(c.Request.Context(), fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file or analysis results not found"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *AnalysisHandler) GetFileMetrics(c *gin.Context) {
	fileID := c.Param("file_id")

	resp, err := h.analysisUC.GetFileMetrics(c.Request.Context(), fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file or analysis results not found"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *AnalysisHandler) GetFilePatterns(c *gin.Context) {
	fileID := c.Param("file_id")

	resp, err := h.analysisUC.GetFilePatterns(c.Request.Context(), fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file or analysis results not found"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *AnalysisHandler) AnalyzeExistingFile(c *gin.Context) {
	fileID := c.Param("file_id")
	userID := c.GetString("user_id")
	quota := c.GetInt("analysis_quota")
	cacheConfigID := strings.TrimSpace(c.PostForm("cache_config_id"))

	cacheProfile, err := parseCacheProfile(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.analysisUC.RunAnalysisOnExistingFile(
		c.Request.Context(),
		userID,
		quota,
		fileID,
		cacheConfigID,
		cacheProfile,
	)
	if err != nil {
		if writeAnalysisUploadError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "analysis started for existing file",
		"task":    task,
	})
}
