package handler

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/diploma/analysis-api-service/internal/model"
	"github.com/diploma/analysis-api-service/internal/repository"
	"github.com/diploma/analysis-api-service/internal/usecase"
	"github.com/gin-gonic/gin"
)

type AnalysisHandler struct {
	analysisUC *usecase.AnalysisUseCase
	chRepo     *repository.ClickHouseRepo
}

func NewAnalysisHandler(analysisUC *usecase.AnalysisUseCase, chRepo *repository.ClickHouseRepo) *AnalysisHandler {
	return &AnalysisHandler{analysisUC: analysisUC, chRepo: chRepo}
}

func (h *AnalysisHandler) Upload(c *gin.Context) {
	projectID := c.PostForm("project_id")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	cacheProfile, err := parseCacheProfile(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.analysisUC.UploadAndAnalyze(c.Request.Context(), projectID, header.Filename, file, header.Size, cacheProfile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "file uploaded, analysis started",
		"task":    task,
	})
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

	cacheProfile, err := parseCacheProfile(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.analysisUC.RunAnalysisOnExistingFile(c.Request.Context(), fileID, cacheProfile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "analysis started for existing file",
		"task":    task,
	})
}
