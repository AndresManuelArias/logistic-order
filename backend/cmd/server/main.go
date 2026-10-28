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

	authdelivery "tiendas-track/backend/internal/auth/delivery"
	authinfrastructure "tiendas-track/backend/internal/auth/infrastructure"
	authpostgres "tiendas-track/backend/internal/auth/infrastructure/postgres"
	authusecase "tiendas-track/backend/internal/auth/usecase"
	ordersdelivery "tiendas-track/backend/internal/orders/delivery"
	orderspostgres "tiendas-track/backend/internal/orders/infrastructure/postgres"
	ordersusecase "tiendas-track/backend/internal/orders/usecase"
	"tiendas-track/backend/internal/shared/config"
	"tiendas-track/backend/internal/shared/database"
	"tiendas-track/backend/internal/shared/httpserver"
	"tiendas-track/backend/internal/shared/token"
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

	// --- F2: composición de módulos (auth + orders) -------------------------
	// Regla de Clean Architecture: main es el único lugar que conoce todas las
	// implementaciones concretas y decide quién inyecta qué a quién.
	tokenSvc, err := token.New(cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL, cfg.GuestTokenTTL)
	if err != nil {
		return err
	}

	// Repositorios (PostgreSQL) y adaptadores.
	userRepo := authpostgres.NewUserRepository(pool)
	refreshRepo := authpostgres.NewRefreshTokenRepository(pool)
	guestRepo := authpostgres.NewGuestTokenRepository(pool)
	tokenAdapter := authinfrastructure.NewJWTAdapter(tokenSvc)
	hasher := authinfrastructure.NewPasswordHasher(cfg.BcryptCost)
	ordersRepo := orderspostgres.NewOrderRepository(pool)

	// Casos de uso.
	ordersUC := ordersusecase.New(ordersRepo, log)
	// El módulo auth no conoce al de orders: se le inyecta una función que
	// decide si el llamante puede emitir un token de invitado sobre la orden.
	authUC := authusecase.New(userRepo, userRepo, refreshRepo, guestRepo,
		tokenAdapter, hasher, ordersUC.IsAccessibleBy, log)

	handler := httpserver.NewWithModules(log, pool, httpserver.Options{
		Token: tokenSvc,
		Modules: httpserver.Modules{
			Auth:   authdelivery.NewHandler(authUC, log),
			Orders: ordersdelivery.NewHandler(ordersUC, log),
		},
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
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
