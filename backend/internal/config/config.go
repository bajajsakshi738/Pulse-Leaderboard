package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds the settings. everything comes from environment variables,
// from .env locally or from the host in production.
type Config struct {
	Environment string // "development" | "production"

	ServerPort string

	DatabaseURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int
	RedisUseTLS   bool

	JWTSecret string
	JWTExpiry time.Duration

	AllowedOrigins []string

	// RateLimitBypassToken lets the bot generator skip rate limits. empty = off.
	RateLimitBypassToken string

	// TrustProxyHeader: true only behind a reverse proxy (like Render) that adds
	// the real client IP to X-Forwarded-For. see middleware.NewRateLimiter.
	TrustProxyHeader bool
}

// Load reads .env if there is one, then the environment. environment
// variables win, so it is fine in production where there is no .env.
func Load() (*Config, error) {
	// .env is optional, ignore the error.
	_ = godotenv.Load()

	cfg := &Config{
		Environment: getEnv("ENVIRONMENT", "development"),
		ServerPort:  getEnv("SERVER_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", ""),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		JWTSecret:     getEnv("JWT_SECRET", ""),

		RedisUseTLS: getEnvBool("REDIS_USE_TLS", false),

		RateLimitBypassToken: getEnv("RATE_LIMIT_BYPASS_TOKEN", ""),
		TrustProxyHeader:     getEnvBool("TRUST_PROXY_HEADER", false),
	}

	redisDB, err := strconv.Atoi(getEnv("REDIS_DB", "0"))
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_DB: %w", err)
	}
	cfg.RedisDB = redisDB

	jwtExpiryHours, err := strconv.Atoi(getEnv("JWT_EXPIRY_HOURS", "72"))
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_EXPIRY_HOURS: %w", err)
	}
	cfg.JWTExpiry = time.Duration(jwtExpiryHours) * time.Hour

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	originsRaw := getEnv("ALLOWED_ORIGINS", "http://localhost:4200")
	cfg.AllowedOrigins = splitAndTrim(originsRaw)

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitAndTrim(s string) []string {
	var result []string
	current := ""
	for _, r := range s {
		if r == ',' {
			if trimmed := trimSpace(current); trimmed != "" {
				result = append(result, trimmed)
			}
			current = ""
			continue
		}
		current += string(r)
	}
	if trimmed := trimSpace(current); trimmed != "" {
		result = append(result, trimmed)
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
