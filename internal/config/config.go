package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// HTTP
	Port            string
	ShutdownTimeout time.Duration

	// PostgreSQL
	DatabaseURL string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Observability
	LogLevel string

	// Security
	ArgonTime    uint32
	ArgonMemory  uint32
	ArgonThreads uint8
	ArgonKeyLen  uint32
}

// Load reads configuration from environment variables with defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Port:            getEnv("PORT", "8080"),
		ShutdownTimeout: getDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://quotaforge:quotaforge@localhost:5432/quotaforge?sslmode=disable"),
		RedisAddr:       getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:   getEnv("REDIS_PASSWORD", ""),
		RedisDB:         getInt("REDIS_DB", 0),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		ArgonTime:       uint32(getInt("ARGON_TIME", 1)),
		ArgonMemory:     uint32(getInt("ARGON_MEMORY", 64*1024)),
		ArgonThreads:    uint8(getInt("ARGON_THREADS", 4)),
		ArgonKeyLen:     uint32(getInt("ARGON_KEY_LEN", 32)),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.RedisAddr == "" {
		return nil, fmt.Errorf("REDIS_ADDR is required")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}

func getDuration(key string, fallback time.Duration) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}
