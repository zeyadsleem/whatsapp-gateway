package testutil

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"database/sql"

	"go.mau.fi/whatsmeow/store/sqlstore"

	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/logger"
)

const envTestDatabaseURL = "TEST_DATABASE_URL"

type TestDB struct {
	DB        *database.DB
	SQLDB     *sql.DB
	Container *sqlstore.Container
	DSN       string
	Log       *slog.Logger
}

// RequireDB creates an isolated database for a test package, runs the
// application and whatsmeow migrations, and drops it on cleanup. Tests are
// skipped when TEST_DATABASE_URL is not set.
func RequireDB(t *testing.T, name string) *TestDB {
	t.Helper()
	adminDSN := os.Getenv(envTestDatabaseURL)
	if adminDSN == "" {
		t.Skipf("%s is not set, skipping integration test", envTestDatabaseURL)
	}

	ctx := context.Background()
	adminPool, err := database.New(ctx, adminDSN, 2, 1)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	dbName := "wg_test_" + sanitize(name)
	if _, err := adminPool.Pool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName)); err != nil {
		adminPool.Close()
		t.Fatalf("drop test database: %v", err)
	}
	if _, err := adminPool.Pool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName)); err != nil {
		adminPool.Close()
		t.Fatalf("create test database: %v", err)
	}
	adminPool.Close()

	dsn, err := replaceDatabase(adminDSN, dbName)
	if err != nil {
		t.Fatalf("rewrite test dsn: %v", err)
	}

	db, err := database.New(ctx, dsn, 4, 1)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	if err := database.Migrate(ctx, db.Pool); err != nil {
		db.Close()
		t.Fatalf("migrate test database: %v", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sqlDB, err := database.NewWhatsAppSQLDB(dsn)
	if err != nil {
		db.Close()
		t.Fatalf("open whatsmeow test connection: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS whatsapp"); err != nil {
		db.Close()
		_ = sqlDB.Close()
		t.Fatalf("create whatsapp schema: %v", err)
	}
	container := sqlstore.NewWithDB(sqlDB, "postgres", logger.WhatsAppLogger(log, "sqlstore-test"))
	if err := container.Upgrade(ctx); err != nil {
		db.Close()
		_ = sqlDB.Close()
		t.Fatalf("upgrade whatsmeow store: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_ = container.Close()
		_ = sqlDB.Close()
		db.Close()
		pool, err := database.New(cleanupCtx, adminDSN, 2, 1)
		if err != nil {
			t.Logf("cleanup: reconnect admin database: %v", err)
			return
		}
		defer pool.Close()
		if _, err := pool.Pool.Exec(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName)); err != nil {
			t.Logf("cleanup: drop test database: %v", err)
		}
	})

	return &TestDB{DB: db, SQLDB: sqlDB, Container: container, DSN: dsn, Log: log}
}

func replaceDatabase(dsn, dbName string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		return "", fmt.Errorf("TEST_DATABASE_URL must be a postgres URL")
	}
	u.Path = "/" + dbName
	return u.String(), nil
}

func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	return b.String()
}
