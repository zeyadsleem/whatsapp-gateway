package config

import (
	"strings"
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ADMIN_API_KEY", strings.Repeat("a", 48))
	t.Setenv("APP_ENCRYPTION_KEY", strings.Repeat("b", 48))
	t.Setenv("POSTGRES_DSN", "postgres://user:pass@localhost:5432/db?sslmode=disable")
}

func TestLoadDefaults(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppPort != 8080 {
		t.Fatalf("default port = %d", cfg.AppPort)
	}
	if cfg.MaxInstances != 100 {
		t.Fatalf("default max instances = %d", cfg.MaxInstances)
	}
	if cfg.WebhookMaxRetries != 3 {
		t.Fatalf("default webhook retries = %d", cfg.WebhookMaxRetries)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("default log level = %q", cfg.LogLevel)
	}
}

func TestLoadValidation(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ADMIN_API_KEY", "short")
	if _, err := Load(); err == nil {
		t.Fatal("short admin key must be rejected")
	}

	setRequiredEnv(t)
	t.Setenv("LOG_LEVEL", "verbose")
	if _, err := Load(); err == nil {
		t.Fatal("invalid log level must be rejected")
	}

	setRequiredEnv(t)
	t.Setenv("APP_PORT", "70000")
	if _, err := Load(); err == nil {
		t.Fatal("invalid port must be rejected")
	}
}

func TestIsProduction(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.IsProduction() {
		t.Fatal("default APP_ENV should be production")
	}
}
