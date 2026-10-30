// Command seed crea los datos de demostración del proyecto (F2).
// Uso: make seed (o `docker compose exec backend seed`).
// Es idempotente: si los datos ya existen, sale sin modificar nada.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"log/slog"

	"tiendas-track/backend/internal/seed"
	"tiendas-track/backend/internal/shared/config"
	"tiendas-track/backend/internal/shared/database"
	"tiendas-track/backend/migrations"
	"tiendas-track/backend/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed error:", err)
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

	pool, err := database.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBConnectTimeout, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	// El seed asume el esquema listo (igual que el servidor).
	if err := database.RunMigrations(ctx, pool, migrations.FS, log); err != nil {
		return err
	}

	res, err := seed.Run(ctx, pool, log)
	if err != nil {
		return err
	}
	if res.Skipped {
		fmt.Printf("seed: los datos base ya existen (idempotente); telemetría verificada (%d puntos nuevos) y %d alianzas nuevas\n",
			res.Points, res.Agreements)
		return nil
	}
	fmt.Printf("seed: %d usuarios, %d tiendas, %d vehículos, %d conductores, %d órdenes, %d puntos de telemetría, %d alianzas\n",
		res.Users, res.Stores, res.Vehicles, res.Drivers, res.Orders, res.Points, res.Agreements)
	return nil
}
