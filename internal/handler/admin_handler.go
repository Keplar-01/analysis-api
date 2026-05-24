package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/diploma/analysis-api-service/internal/kafka"
	"github.com/diploma/analysis-api-service/internal/model"
	"github.com/diploma/analysis-api-service/internal/repository"
	"github.com/diploma/analysis-api-service/internal/storage"
	"github.com/diploma/analysis-api-service/internal/usecase"
	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	uc           *usecase.AnalysisUseCase
	chRepo       *repository.ClickHouseRepo
	pgRepo       *repository.AnalysisRepository
	minio        *storage.MinIOClient
	kafkaBrokers string
}

type topPatternsEnvelope struct {
	Patterns []repository.TopPatternRow `json:"patterns"`
}

func NewAdminHandler(
	uc *usecase.AnalysisUseCase,
	chRepo *repository.ClickHouseRepo,
	pgRepo *repository.AnalysisRepository,
	minio *storage.MinIOClient,
	kafkaBrokers string,
) *AdminHandler {
	return &AdminHandler{
		uc:           uc,
		chRepo:       chRepo,
		pgRepo:       pgRepo,
		minio:        minio,
		kafkaBrokers: kafkaBrokers,
	}
}

func (h *AdminHandler) GetStats(c *gin.Context) {
	stats, err := h.uc.GetAnalysisAdminStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *AdminHandler) GetTopPatterns(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	patterns, err := h.chRepo.GetTopPatterns(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"patterns": []any{}})
		return
	}

	if patterns == nil {
		patterns = []repository.TopPatternRow{}
	}
	c.JSON(http.StatusOK, gin.H{"patterns": patterns})
}

func (h *AdminHandler) GetSystemStatus(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp := model.SystemStatus{
		Postgres:   componentStatus(h.pgRepo.PingDB(ctx)),
		Minio:      componentStatus(h.minio.HealthCheck(ctx)),
		ClickHouse: componentStatus(h.chRepo.Ping(ctx)),
		Kafka:      componentStatus(checkKafka(ctx, h.kafkaBrokers)),
	}

	resp.StartStaticQueue = countTopicLag(ctx, h.kafkaBrokers, kafka.TopicStartStatic)

	c.JSON(http.StatusOK, resp)
}

func componentStatus(err error) model.SystemComponentStatus {
	if err != nil {
		return model.SystemComponentStatus{Status: "down", Error: err.Error()}
	}
	return model.SystemComponentStatus{Status: "ok"}
}

func checkKafka(ctx context.Context, brokers string) error {
	dialer := &kafkago.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", brokers)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.ReadPartitions()
	return err
}

func countTopicLag(ctx context.Context, brokers, topic string) int64 {
	dialer := &kafkago.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", brokers)
	if err != nil {
		return 0
	}
	defer conn.Close()

	parts, err := conn.ReadPartitions(topic)
	if err != nil {
		return 0
	}

	var lag int64
	for _, p := range parts {
		leaderConn, err := dialer.DialLeader(ctx, "tcp", brokers, topic, p.ID)
		if err != nil {
			continue
		}
		first, err := leaderConn.ReadFirstOffset()
		if err != nil {
			leaderConn.Close()
			continue
		}
		last, err := leaderConn.ReadLastOffset()
		leaderConn.Close()
		if err != nil {
			continue
		}
		if last > first {
			lag += last - first
		}
	}
	return lag
}
