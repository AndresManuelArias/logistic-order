// Package database centraliza la conexión a PostgreSQL/TimescaleDB.
// El circuit breaker sobre el pool se añade en F5 (RNF-05).
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect crea el pool de conexiones y espera a que la base de datos
// esté disponible (retry con backoff fijo, tolerante al arranque de Docker).
func Connect(ctx context.Context, url string, maxConns int32, connTimeout time.Duration, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("database: URL inválida: %w", err)
	}
	cfg.MaxConns = maxConns

	deadline := time.Now().Add(connTimeout)
	var lastErr error
	for attempt := 1; ; attempt++ {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				log.Info("conexión a base de datos establecida",
					"max_conns", maxConns, "intentos", attempt)
				return pool, nil
			}
			pool.Close()
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("database: sin conexión tras %v: %w", connTimeout, lastErr)
		}
		log.Debug("base de datos no disponible, reintentando", "intento", attempt, "error", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
