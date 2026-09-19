package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow/store/sqlstore"

	"github.com/zeyad/whatsapp-gateway/internal/config"
	"github.com/zeyad/whatsapp-gateway/internal/crypto"
	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/httpapi"
	"github.com/zeyad/whatsapp-gateway/internal/instance"
	"github.com/zeyad/whatsapp-gateway/internal/logger"
	"github.com/zeyad/whatsapp-gateway/internal/message"
	"github.com/zeyad/whatsapp-gateway/internal/version"
	"github.com/zeyad/whatsapp-gateway/internal/webhook"
	"github.com/zeyad/whatsapp-gateway/internal/whatsapp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log := logger.New(cfg.LogLevel, cfg.LogFormat)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("starting whatsapp gateway",
		"event", "startup",
		"version", version.Version,
		"commit", version.Commit,
		"env", cfg.AppEnv,
	)

	box, err := crypto.New(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("initialize encryption: %w", err)
	}

	db, err := database.New(ctx, cfg.PostgresDSN, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	startCtx, cancelStart := context.WithTimeout(ctx, 60*time.Second)
	defer cancelStart()
	if err := db.Ping(startCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	if err := database.Migrate(startCtx, db.Pool); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	log.Info("database ready", "event", "database", "result", "success")

	container, waSQLDB, err := openWhatsAppStore(startCtx, cfg, log)
	if err != nil {
		return err
	}
	defer func() {
		_ = waSQLDB.Close()
	}()

	repo := database.NewInstanceRepo(db)
	keyRepo := database.NewAPIKeyRepo(db)

	dispatcher := webhook.NewDispatcher(webhook.Config{
		Timeout:    time.Duration(cfg.WebhookTimeoutSec) * time.Second,
		MaxRetries: cfg.WebhookMaxRetries,
		Workers:    cfg.WebhookWorkers,
		QueueSize:  cfg.WebhookQueueSize,
		Log:        log,
	}, repo, box)
	defer dispatcher.Shutdown(3 * time.Second)

	manager := whatsapp.NewManager(whatsapp.ManagerOptions{
		Container:    container,
		Repo:         repo,
		Notifier:     dispatcher,
		Log:          log,
		SendTimeout:  cfg.SendTimeout,
		MaxInstances: cfg.MaxInstances,
	})

	instanceSvc := instance.NewService(repo, keyRepo, manager, box, log)
	messageSvc := message.NewService(manager, cfg.MaxUploadBytes, log)

	server := httpapi.New(httpapi.Deps{
		Config:    cfg,
		Log:       log,
		DB:        db,
		Keys:      keyRepo,
		Instances: instanceSvc,
		Messages:  messageSvc,
		Manager:   manager,
	})

	go manager.RestoreAll(ctx, restorableInstances(startCtx, repo, log))

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Run()
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
		log.Info("shutdown signal received", "event", "shutdown", "result", "success")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Warn("http server shutdown incomplete", "event", "shutdown", "result", "failure", "error", err)
	}
	dispatcher.Shutdown(3 * time.Second)
	manager.Shutdown(5 * time.Second)
	if err := container.Close(); err != nil {
		log.Warn("whatsapp store close failed", "event", "shutdown", "result", "failure", "error", err)
	}
	log.Info("shutdown complete", "event", "shutdown", "result", "success")
	return nil
}

func openWhatsAppStore(ctx context.Context, cfg *config.Config, log *slog.Logger) (*sqlstore.Container, *sql.DB, error) {
	waSQLDB, err := database.NewWhatsAppSQLDB(cfg.PostgresDSN)
	if err != nil {
		return nil, nil, fmt.Errorf("open whatsapp sql connection: %w", err)
	}
	if _, err := waSQLDB.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS whatsapp"); err != nil {
		_ = waSQLDB.Close()
		return nil, nil, fmt.Errorf("create whatsapp schema: %w", err)
	}
	container := sqlstore.NewWithDB(waSQLDB, "postgres", logger.WhatsAppLogger(log, "sqlstore"))
	if err := container.Upgrade(ctx); err != nil {
		_ = waSQLDB.Close()
		return nil, nil, fmt.Errorf("upgrade whatsapp store: %w", err)
	}
	return container, waSQLDB, nil
}

func restorableInstances(ctx context.Context, repo *database.InstanceRepo, log *slog.Logger) []whatsapp.InstanceInfo {
	rows, err := repo.ListRestorable(ctx)
	if err != nil {
		log.Error("failed to load restorable instances", "event", "restore", "result", "failure", "error", err)
		return nil
	}
	out := make([]whatsapp.InstanceInfo, 0, len(rows))
	for _, inst := range rows {
		if inst.WAJID == nil {
			continue
		}
		out = append(out, whatsapp.InstanceInfo{
			ID:          inst.ID,
			ShopID:      inst.ShopID,
			DisplayName: inst.DisplayName,
			WAJID:       *inst.WAJID,
		})
	}
	return out
}
