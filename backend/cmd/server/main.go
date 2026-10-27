// Command server es el punto de entrada del monolito modular Tiendas-Track.
// F1: config, logger, pool de BD, migraciones embebidas, /healthz y /readyz.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"log/slog"

	"tiendas-track/backend/internal/shared/config"
	"tiendas-track/backend/internal/shared/database"
	"tiendas-track/backend/internal/shared/httpserver"
	"tiendas-track/backend/migrations"
	"tiendas-track/backend/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log, err := logger.New(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return err
	}
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("iniciando servidor",
		"app", cfg.AppName, "env", cfg.Env, "addr", cfg.HTTPAddr)

	pool, err := database.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBConnectTimeout, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.MigrateOnStartup {
		if err := database.RunMigrations(ctx, pool, migrations.FS, log); err != nil {
			return err
		}
	} else {
		log.Warn("migraciones deshabilitadas (MIGRATE_ON_STARTUP=false)")
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpserver.New(log, pool),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("servidor escuchando", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("servidor: %w", err)
	case <-ctx.Done():
		log.Info("señal de apagado recibida, cerrando...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("apagado: %w", err)
	}
	log.Info("servicio detenido limpiamente")
	return nil
}
