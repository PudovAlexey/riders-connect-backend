package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          string
	DatabaseURL   string
	JWTSecret     string
	SMTPHost      string
	SMTPPort      string
	SMTPFrom      string
	SMTPPass      string
	AppEnv        string
	UploadDir     string
	UploadBaseURL string
}

func Load() *Config {
	_ = godotenv.Load()
	return &Config{
		Port:          getEnv("PORT", "8080"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/riders_connect?sslmode=disable"),
		JWTSecret:     getEnv("JWT_SECRET", "secret-change-me"),
		SMTPHost:      getEnv("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:      getEnv("SMTP_PORT", "587"),
		SMTPFrom:      getEnv("SMTP_FROM", ""),
		SMTPPass:      getEnv("SMTP_PASS", ""),
		AppEnv:        getEnv("APP_ENV", "development"),
		UploadDir:     getEnv("UPLOAD_DIR", "./uploads"),
		UploadBaseURL: getEnv("UPLOAD_BASE_URL", "http://localhost:8080"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
