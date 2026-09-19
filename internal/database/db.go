package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	AppSearchPath      = "app,public"
	WhatsAppSearchPath = "whatsapp,public"
)

type DB struct {
	Pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string, maxConns, minConns int32) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = AppSearchPath
	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

func (db *DB) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}

func NewWhatsAppSQLDB(dsn string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn for whatsmeow: %w", err)
	}
	cfg.RuntimeParams["search_path"] = WhatsAppSearchPath
	return stdlib.OpenDB(*cfg), nil
}
