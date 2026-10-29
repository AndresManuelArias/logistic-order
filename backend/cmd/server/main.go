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
	aidelivery "tiendas-track/backend/internal/aiagent/delivery"
	aipostgres "tiendas-track/backend/internal/aiagent/infrastructure/postgres"
	aillm "tiendas-track/backend/internal/aiagent/infrastructure/llm"
	aiusecase "tiendas-track/backend/internal/aiagent/usecase"
	authinfrastructure "tiendas-track/backend/internal/auth/infrastructure"
	authpostgres "tiendas-track/backend/internal/auth/infrastructure/postgres"
	authusecase "tiendas-track/backend/internal/auth/usecase"
	ordersdelivery "tiendas-track/backend/internal/orders/delivery"
	orderspostgres "tiendas-track/backend/internal/orders/infrastructure/postgres"
	ordersusecase "tiendas-track/backend/internal/orders/usecase"
	"tiendas-track/backend/internal/shared/config"
	"tiendas-track/backend/internal/shared/database"
	"tiendas-track/backend/internal/shared/eventbus"
	"tiendas-track/backend/internal/shared/httpserver"
	"tiendas-track/backend/internal/shared/resilience"
	"tiendas-track/backend/internal/shared/token"
	"tiendas-track/backend/internal/shared/websocket"
	storesdelivery "tiendas-track/backend/internal/stores/delivery"
	storespostgres "tiendas-track/backend/internal/stores/infrastructure/postgres"
	storesusecase "tiendas-track/backend/internal/stores/usecase"
	telemetrydelivery "tiendas-track/backend/internal/telemetry/delivery"
	telemetrypostgres "tiendas-track/backend/internal/telemetry/infrastructure/postgres"
	telemetryusecase "tiendas-track/backend/internal/telemetry/usecase"
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

	// --- F2/F3: composición de módulos (auth, orders, stores, telemetry) ----
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
	storeRepo := storespostgres.NewStoreRepository(pool)
	pointRepo := telemetrypostgres.NewPointRepository(pool)
	scopeRepo := telemetrypostgres.NewScopeRepository(pool)
	fleetRepo := telemetrypostgres.NewFleetRepository(pool)

	// Casos de uso.
	ordersUC := ordersusecase.New(ordersRepo, log)
	storesUC := storesusecase.New(storeRepo, log)
	// El módulo auth no conoce al de orders: se le inyecta una función que
	// decide si el llamante puede emitir un token de invitado sobre la orden.
	authUC := authusecase.New(userRepo, userRepo, refreshRepo, guestRepo,
		tokenAdapter, hasher, ordersUC.IsAccessibleBy, log)

	// F3: bus en memoria + circuit breaker de lecturas + hub de tiempo real.
	bus := eventbus.New()
	readsBreaker := resilience.New(resilience.Settings{
		Name:             "telemetry-reads",
		FailureThreshold: cfg.CBFailureThreshold,
		Timeout:          cfg.CBTimeout,
	})
	telemetryUC := telemetryusecase.New(pointRepo, scopeRepo, bus, readsBreaker, log)
	fleetUC := telemetryusecase.NewFleet(fleetRepo, readsBreaker, log)
	hub := websocket.NewHub(websocket.Options{
		Bus:            bus,
		Authorizer:     telemetryUC,
		Logger:         log,
		AllowedOrigins: cfg.WSAllowedOrigins,
		GrantTTL:       cfg.WSGrantTTL,
	})

	// F5: agente IA operativo (RF-10) + circuit breaker del proveedor LLM
	// (RNF-05). Sin AI_API_KEY el agente responde 503 controlado y el resto
	// del sistema sigue funcionando.
	aiRepo := aipostgres.NewAnalyticsRepository(pool)
	aiAudit := aipostgres.NewAuditRepository(pool)
	aiModel := aillm.New(aillm.Config{
		BaseURL: cfg.AIBaseURL,
		APIKey:  cfg.AIAPIKey,
		Timeout: cfg.AITimeout,
	})
	aiBreaker := resilience.New(resilience.Settings{
		Name:             "ai-llm",
		FailureThreshold: cfg.CBFailureThreshold,
		Timeout:          cfg.CBTimeout,
	})
	aiUC := aiusecase.New(aiRepo, aiAudit, aiModel, aiBreaker, aiusecase.Config{
		Enabled:  cfg.AIEnabled && cfg.AIAPIKey != "",
		Model:    cfg.AIModel,
		MaxSteps: cfg.AIMaxSteps,
		Timeout:  cfg.AITimeout,
	}, log)
	if !cfg.AIEnabled || cfg.AIAPIKey == "" {
		log.Warn("agente IA deshabilitado: configura AI_API_KEY (o AI_ENABLED=false para desactivarlo explícitamente)")
	}

	handler := httpserver.NewWithModules(log, pool, httpserver.Options{
		Token: tokenSvc,
		Modules: httpserver.Modules{
			Auth:      authdelivery.NewHandler(authUC, log),
			Orders:    ordersdelivery.NewHandler(ordersUC, log),
			Stores:    storesdelivery.NewHandler(storesUC, log),
			Telemetry: telemetrydelivery.NewHandler(telemetryUC, fleetUC, hub, tokenSvc, log),
			AI:        aidelivery.NewHandler(aiUC, log),
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
	hub.Close() // cierra las sesiones WS antes de apagar el servidor
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("apagado: %w", err)
	}
	log.Info("servicio detenido limpiamente")
	return nil
}
