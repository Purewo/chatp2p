package config

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

type Config struct {
	AppName            string
	Environment        string
	HTTPAddr           string
	CORSAllowedOrigins []string
	DBDriver           string
	DBDSN              string
	JWTSecret          string
	JWTIssuer          string
	JWTTTL             time.Duration
	LogLevelName       string
	ShutdownTimeout    time.Duration
}

func Load() Config {
	return Config{
		AppName:            envOrDefault("APP_NAME", "chatp2p"),
		Environment:        envOrDefault("APP_ENV", "development"),
		HTTPAddr:           envOrDefault("HTTP_ADDR", ":8080"),
		CORSAllowedOrigins: csvOrDefault("CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173", "http://127.0.0.1:5173"}),
		DBDriver:           envOrDefault("DB_DRIVER", "sqlite"),
		DBDSN:              envOrDefault("DB_DSN", "file:chatp2p.db"),
		JWTSecret:          envOrDefault("JWT_SECRET", "chatp2p-dev-secret-change-me"),
		JWTIssuer:          envOrDefault("JWT_ISSUER", "chatp2p"),
		JWTTTL:             durationOrDefault("JWT_TTL", 24*time.Hour),
		LogLevelName:       envOrDefault("LOG_LEVEL", "info"),
		ShutdownTimeout:    durationOrDefault("SHUTDOWN_TIMEOUT", 5*time.Second),
	}
}

func (c Config) LogLevel() slog.Level {
	switch strings.ToLower(c.LogLevelName) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func durationOrDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}

func csvOrDefault(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return append([]string(nil), fallback...)
	}

	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	if len(items) == 0 {
		return append([]string(nil), fallback...)
	}
	return items
}
