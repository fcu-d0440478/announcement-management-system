package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	CORSOrigin  string
}

func Load() Config {
	return Config{
		Port:        env("PORT", "8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/announcements?sslmode=disable"),
		JWTSecret:   env("JWT_SECRET", "dev-secret-change-me"),
		CORSOrigin:  env("CORS_ORIGIN", "http://localhost:5173"),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func EnvBool(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

