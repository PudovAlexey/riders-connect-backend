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
	CORSOrigins   string
	VAPIDPublic   string
	VAPIDPrivate  string
	VAPIDSubject  string
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
		// Comma-separated list of allowed browser origins, or "*" for any.
		CORSOrigins: getEnv("CORS_ORIGINS", "*"),
		// Web Push (VAPID). Empty keys disable push (callers fall back to email).
		VAPIDPublic:  getEnv("VAPID_PUBLIC", ""),
		VAPIDPrivate: getEnv("VAPID_PRIVATE", ""),
		VAPIDSubject: getEnv("VAPID_SUBJECT", "mailto:pudo-aleksej@yandex.ru"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
