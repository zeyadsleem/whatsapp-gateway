package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv      string
	AppPort     int
	AppBaseURL  string
	LogLevel    string
	LogFormat   string
	AdminAPIKey string

	EncryptionKey string

	PostgresDSN string
	DBMaxConns  int32
	DBMinConns  int32

	WebhookTimeoutSec int
	WebhookMaxRetries int
	WebhookWorkers    int
	WebhookQueueSize  int

	MaxInstances   int
	MaxUploadBytes int64

	RateLimitPerSecond  float64
	RateLimitBurst      int
	AdminRateLimitRPS   float64
	AdminRateLimitBurst int

	AllowedOrigins []string

	ShutdownTimeout time.Duration
	SendTimeout     time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		AppEnv:      env("APP_ENV", "production"),
		AppPort:     envInt("APP_PORT", 8080),
		AppBaseURL:  env("APP_BASE_URL", "http://localhost:8080"),
		LogLevel:    strings.ToLower(env("LOG_LEVEL", "info")),
		LogFormat:   strings.ToLower(env("LOG_FORMAT", "json")),
		AdminAPIKey: env("ADMIN_API_KEY", ""),

		EncryptionKey: env("APP_ENCRYPTION_KEY", ""),

		PostgresDSN: env("POSTGRES_DSN", ""),
		DBMaxConns:  int32(envInt("DB_MAX_CONNS", 20)),
		DBMinConns:  int32(envInt("DB_MIN_CONNS", 2)),

		WebhookTimeoutSec: envInt("WEBHOOK_TIMEOUT_SECONDS", 5),
		WebhookMaxRetries: envInt("WEBHOOK_MAX_RETRIES", 3),
		WebhookWorkers:    envInt("WEBHOOK_WORKERS", 4),
		WebhookQueueSize:  envInt("WEBHOOK_QUEUE_SIZE", 256),

		MaxInstances:   envInt("MAX_INSTANCES", 100),
		MaxUploadBytes: int64(envInt("MAX_UPLOAD_MB", 16)) * 1024 * 1024,

		RateLimitPerSecond:  envFloat("SHOP_RATE_LIMIT_RPS", 10),
		RateLimitBurst:      envInt("SHOP_RATE_LIMIT_BURST", 20),
		AdminRateLimitRPS:   envFloat("ADMIN_RATE_LIMIT_RPS", 20),
		AdminRateLimitBurst: envInt("ADMIN_RATE_LIMIT_BURST", 40),

		AllowedOrigins: splitList(env("ALLOWED_ORIGINS", "")),

		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT_SECONDS", 15*time.Second),
		SendTimeout:     envDuration("SEND_TIMEOUT_SECONDS", 30*time.Second),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) IsProduction() bool { return c.AppEnv == "production" }

func (c *Config) validate() error {
	var errs []error

	if c.AdminAPIKey == "" {
		errs = append(errs, errors.New("ADMIN_API_KEY is required"))
	} else if len(c.AdminAPIKey) < 32 {
		errs = append(errs, errors.New("ADMIN_API_KEY must be at least 32 characters"))
	}
	if c.EncryptionKey == "" {
		errs = append(errs, errors.New("APP_ENCRYPTION_KEY is required (base64-encoded 32 bytes)"))
	} else if len(c.EncryptionKey) < 32 {
		errs = append(errs, errors.New("APP_ENCRYPTION_KEY must be at least 32 characters"))
	}
	if c.PostgresDSN == "" {
		errs = append(errs, errors.New("POSTGRES_DSN is required"))
	}
	if c.AppPort < 1 || c.AppPort > 65535 {
		errs = append(errs, fmt.Errorf("APP_PORT out of range: %d", c.AppPort))
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be debug|info|warn|error, got %q", c.LogLevel))
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		errs = append(errs, fmt.Errorf("LOG_FORMAT must be json|text, got %q", c.LogFormat))
	}
	if c.WebhookTimeoutSec < 1 || c.WebhookTimeoutSec > 60 {
		errs = append(errs, errors.New("WEBHOOK_TIMEOUT_SECONDS must be between 1 and 60"))
	}
	if c.WebhookMaxRetries < 0 || c.WebhookMaxRetries > 10 {
		errs = append(errs, errors.New("WEBHOOK_MAX_RETRIES must be between 0 and 10"))
	}
	if c.WebhookWorkers < 1 || c.WebhookWorkers > 64 {
		errs = append(errs, errors.New("WEBHOOK_WORKERS must be between 1 and 64"))
	}
	if c.MaxInstances < 1 || c.MaxInstances > 10000 {
		errs = append(errs, errors.New("MAX_INSTANCES must be between 1 and 10000"))
	}
	if c.RateLimitPerSecond <= 0 {
		errs = append(errs, errors.New("SHOP_RATE_LIMIT_RPS must be positive"))
	}
	if c.MaxUploadBytes < 1 {
		errs = append(errs, errors.New("MAX_UPLOAD_MB must be positive"))
	}
	return errors.Join(errs...)
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloat(key string, def float64) float64 {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func envDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func splitList(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
