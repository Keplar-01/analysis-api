package config

import "os"

type Config struct {
	ServerPort string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	JWTSecret string

	DevUserID    string
	DevUserEmail string
	DevUserRole  string

	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioUseSSL    bool

	KafkaBrokers string

	ClickHouseAddr     string
	ClickHouseUser     string
	ClickHousePassword string
	ClickHouseDB       string

	InterpreterVersion string
	AnalyzerBinary     string
}

func Load() *Config {
	return &Config{
		ServerPort: getEnv("SERVER_PORT", "8082"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "diplom"),
		DBPassword: getEnv("DB_PASSWORD", "diplom_secret"),
		DBName:     getEnv("DB_NAME", "analysis_db"),

		JWTSecret: getEnv("JWT_SECRET", "super-secret-jwt-key-for-diploma-2026"),

		DevUserID:    getEnv("DEV_USER_ID", "00000000-0000-0000-0000-000000000001"),
		DevUserEmail: getEnv("DEV_USER_EMAIL", "dev@analysis.local"),
		DevUserRole:  getEnv("DEV_USER_ROLE", "admin"),

		MinioEndpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinioAccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin123"),
		MinioUseSSL:    getEnv("MINIO_USE_SSL", "false") == "true",

		KafkaBrokers: getEnv("KAFKA_BROKERS", "localhost:9092"),

		ClickHouseAddr:     getEnv("CLICKHOUSE_ADDR", "localhost:9000"),
		ClickHouseUser:     getEnv("CLICKHOUSE_USER", "default"),
		ClickHousePassword: getEnv("CLICKHOUSE_PASSWORD", "clickhouse_secret"),
		ClickHouseDB:       getEnv("CLICKHOUSE_DB", "analysis_metrics"),

		InterpreterVersion: getEnv("INTERPRETER_VERSION", "CacheSim.exe"),
		AnalyzerBinary:     getEnv("ANALYZER_BINARY", "/usr/local/bin/analyzer"),
	}
}

func (c *Config) DSN() string {
	return "postgres://" + c.DBUser + ":" + c.DBPassword +
		"@" + c.DBHost + ":" + c.DBPort + "/" + c.DBName + "?sslmode=disable"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
