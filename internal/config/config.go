package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string

	// MinIO
	MinIOEndpoint  string
	MinIOBucket    string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOUseSSL    bool

	// Redis
	RedisURL string
}

func Load() *Config {
	return &Config{
		Port:        getEnv("SERVER_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", ""),

		MinIOEndpoint:  getEnv("MINIO_ENDPOINT", ""),
		MinIOBucket:    getEnv("MINIO_BUCKET", "filesync"),
		MinIOAccessKey: getEnv("MINIO_ACCESS_KEY", ""),
		MinIOSecretKey: getEnv("MINIO_SECRET_KEY", ""),
		MinIOUseSSL:    getEnv("MINIO_USE_SSL", "false") == "true",

		RedisURL: getEnv("REDIS_URL", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
