package database

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunMigrations aplica las migraciones SQL pendientes del FS embebido.
// No es un migrador robusto multi-instancia: está pensado para el arranque
// del monolito en desarrollo y CI. En producción se migrará por separado.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, log *slog.Logger) error {
	if _, err := pool.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS telemetry;
		CREATE TABLE IF NOT EXISTS telemetry.schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("migrate: preparando control de versiones: %w", err)
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("migrate: leyendo directorio de migraciones: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(name, ".sql")

		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM telemetry.schema_migrations WHERE version = $1)`,
			version).Scan(&exists); err != nil {
			return fmt.Errorf("migrate: consultando versión %s: %w", version, err)
		}
		if exists {
			log.Debug("migración ya aplicada", "version", version)
			continue
		}

		sql, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("migrate: leyendo %s: %w", name, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("migrate: abriendo transacción para %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migrate: aplicando %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO telemetry.schema_migrations (version) VALUES ($1)`, version); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migrate: registrando %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migrate: confirmando %s: %w", version, err)
		}
		log.Info("migración aplicada", "version", version)
	}
	return nil
}
