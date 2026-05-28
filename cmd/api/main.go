package main

import (
	"context"
	"log"
	"time"

	apidocs "github.com/diploma/analysis-api-service/docs"
	analysisanalyzer "github.com/diploma/analysis-api-service/internal/analyzer"
	"github.com/diploma/analysis-api-service/internal/config"
	"github.com/diploma/analysis-api-service/internal/handler"
	"github.com/diploma/analysis-api-service/internal/kafka"
	"github.com/diploma/analysis-api-service/internal/middleware"
	"github.com/diploma/analysis-api-service/internal/repository"
	"github.com/diploma/analysis-api-service/internal/storage"
	"github.com/diploma/analysis-api-service/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func main() {
	cfg := config.Load()

	db, err := connectDB(cfg.DSN())
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	runMigrations(db)

	minioClient, err := storage.NewMinIOClient(cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioUseSSL)
	if err != nil {
		log.Fatalf("failed to connect to MinIO: %v", err)
	}

	redisClient, err := storage.NewRedisClient(cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		log.Fatalf("failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	producer := kafka.NewProducer(cfg.KafkaBrokers)
	defer producer.Close()

	repo := repository.NewAnalysisRepository(db)

	chRepo, err := repository.NewClickHouseRepo(cfg.ClickHouseAddr, cfg.ClickHouseUser, cfg.ClickHousePassword, cfg.ClickHouseDB)
	if err != nil {
		log.Fatalf("failed to connect to clickhouse: %v", err)
	}
	defer chRepo.Close()

	analyzer := analysisanalyzer.New(cfg.AnalyzerBinary)
	analysisUC := usecase.NewAnalysisUseCase(repo, chRepo, minioClient, producer, analyzer, cfg.InterpreterVersion, redisClient)
	consumer := kafka.NewConsumer(cfg.KafkaBrokers, analysisUC)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	consumer.StartListening(ctx)

	analysisHandler := handler.NewAnalysisHandler(analysisUC, chRepo)
	authHandler := handler.NewAuthHandler(cfg.JWTSecret, cfg.DevUserID, cfg.DevUserEmail, cfg.DevUserRole)
	adminHandler := handler.NewAdminHandler(analysisUC, chRepo, repo, minioClient, cfg.KafkaBrokers)

	r := gin.Default()
	r.Use(corsMiddleware())

	r.GET("/api/v1/analysis/dev-token", authHandler.IssueDevToken)

	v1 := r.Group("/api/v1/analysis")
	v1.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		v1.POST("/upload", analysisHandler.Upload)
		v1.GET("/cache-configs", analysisHandler.ListCacheConfigs)
		v1.POST("/cache-configs", analysisHandler.CreateCacheConfig)
		v1.DELETE("/cache-configs/:config_id", analysisHandler.DeleteCacheConfig)
		v1.GET("/tasks/:task_id", analysisHandler.GetTaskStatus)
		v1.GET("/tasks/:task_id/metrics", analysisHandler.GetTaskMetrics)
		v1.GET("/tasks/:task_id/static-patterns", analysisHandler.GetTaskStaticPatterns)
		v1.GET("/tasks/:task_id/aggregated", analysisHandler.GetAggregatedMetrics)
		v1.GET("/projects/:project_id/tasks", analysisHandler.GetProjectTasks)
		v1.GET("/projects/:project_id/files", analysisHandler.GetProjectFiles)
		v1.DELETE("/files/:file_id", analysisHandler.SoftDeleteFile)
		v1.GET("/files/:file_id/content", analysisHandler.GetFileContent)
		v1.GET("/files/:file_id/metrics", analysisHandler.GetFileMetrics)
		v1.GET("/files/:file_id/patterns", analysisHandler.GetFilePatterns)
		v1.GET("/files/:file_id/simulation-results", analysisHandler.GetFileSimulationResults)
		v1.POST("/files/:file_id/analyze", analysisHandler.AnalyzeExistingFile)

		admin := v1.Group("/admin")
		{
			admin.GET("/stats", adminHandler.GetStats)
			admin.GET("/patterns/top", adminHandler.GetTopPatterns)
			admin.GET("/system-status", adminHandler.GetSystemStatus)
			admin.POST("/debug/static-patterns", analysisHandler.DebugStaticAnalyzer)
		}
	}

	r.GET("/swagger", func(c *gin.Context) {
		c.Data(200, "text/html; charset=utf-8", []byte(apidocs.SwaggerHTML))
	})
	r.GET("/swagger/openapi.json", func(c *gin.Context) {
		c.Data(200, "application/json; charset=utf-8", []byte(apidocs.OpenAPIJSON))
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "analysis-api"})
	})

	log.Printf("Analysis API starting on port %s", cfg.ServerPort)
	if err := r.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}

func connectDB(dsn string) (*sqlx.DB, error) {
	var db *sqlx.DB
	var err error

	for i := 0; i < 30; i++ {
		db, err = sqlx.Connect("postgres", dsn)
		if err == nil {
			db.SetMaxOpenConns(25)
			db.SetMaxIdleConns(5)
			return db, nil
		}
		log.Printf("waiting for database... attempt %d/30", i+1)
		time.Sleep(2 * time.Second)
	}
	return nil, err
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func runMigrations(db *sqlx.DB) {
	schema := `
	CREATE TABLE IF NOT EXISTS files (
		id           VARCHAR(36) PRIMARY KEY,
		project_id   VARCHAR(36) NOT NULL,
		filename     VARCHAR(255) NOT NULL,
		s3_path      TEXT NOT NULL,
		content_hash VARCHAR(64) NOT NULL DEFAULT '',
		size_bytes   BIGINT NOT NULL DEFAULT 0,
		created_at   TIMESTAMP NOT NULL DEFAULT NOW()
	);

	ALTER TABLE files ADD COLUMN IF NOT EXISTS content_hash VARCHAR(64) NOT NULL DEFAULT '';
	ALTER TABLE files ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0;
	ALTER TABLE files ADD COLUMN IF NOT EXISTS owner_user_id VARCHAR(36) NOT NULL DEFAULT '';
	ALTER TABLE files ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP NULL;

	CREATE INDEX IF NOT EXISTS idx_files_project_id ON files(project_id);
	CREATE INDEX IF NOT EXISTS idx_files_dedup ON files(project_id, filename, content_hash);

	CREATE TABLE IF NOT EXISTS analysis_tasks (
		id                      VARCHAR(36) PRIMARY KEY,
		file_id                 VARCHAR(36) NOT NULL REFERENCES files(id) ON DELETE CASCADE,
		status                  VARCHAR(50) NOT NULL DEFAULT 'pending',
		type                    VARCHAR(50) NOT NULL DEFAULT 'full_analysis',
		cache_profile_hash      TEXT NOT NULL DEFAULT '',
		static_artifact_s3_path TEXT NOT NULL DEFAULT '',
		cache_artifact_s3_path  TEXT NOT NULL DEFAULT '',
		reused_from_task_id     VARCHAR(36) NOT NULL DEFAULT '',
		created_at              TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at              TIMESTAMP NOT NULL DEFAULT NOW()
	);

	ALTER TABLE analysis_tasks ADD COLUMN IF NOT EXISTS cache_profile_hash TEXT NOT NULL DEFAULT '';
	ALTER TABLE analysis_tasks ADD COLUMN IF NOT EXISTS static_artifact_s3_path TEXT NOT NULL DEFAULT '';
	ALTER TABLE analysis_tasks ADD COLUMN IF NOT EXISTS cache_artifact_s3_path TEXT NOT NULL DEFAULT '';
	ALTER TABLE analysis_tasks ADD COLUMN IF NOT EXISTS reused_from_task_id VARCHAR(36) NOT NULL DEFAULT '';
	ALTER TABLE analysis_tasks ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '';

	CREATE INDEX IF NOT EXISTS idx_tasks_file_id ON analysis_tasks(file_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_status ON analysis_tasks(status);

	CREATE TABLE IF NOT EXISTS cache_simulator_configs (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		display_name VARCHAR(255) NOT NULL,
		original_filename VARCHAR(255) NOT NULL,
		s3_path TEXT NOT NULL,
		size_bytes BIGINT NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_cache_sim_configs_user_id ON cache_simulator_configs(user_id);

	ALTER TABLE analysis_tasks ADD COLUMN IF NOT EXISTS cache_config_id VARCHAR(36) NOT NULL DEFAULT '';
	ALTER TABLE analysis_tasks ADD COLUMN IF NOT EXISTS cache_config_s3_path TEXT NOT NULL DEFAULT '';
	`

	if _, err := db.Exec(schema); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}
	log.Println("analysis-db migrations completed successfully")
}
