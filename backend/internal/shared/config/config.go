// Package config carga la configuración del monolito desde variables de entorno.
// Regla: nada de valores por defecto distintos entre máquinas; los secretos
// nunca se definen aquí, llegan por entorno (ver .env.example en la raíz).
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config es la configuración operativa del servidor.
type Config struct {
	AppName          string
	Env              string // development | test | production
	HTTPAddr         string // dirección de escucha HTTP, ej. ":8080"
	DatabaseURL      string // URL de PostgreSQL/TimescaleDB (secreto por entorno)
	DBMaxConns       int32
	DBConnectTimeout time.Duration
	MigrateOnStartup bool
	LogLevel         string
	LogFormat        string
	ShutdownTimeout  time.Duration
}

// Load lee la configuración del entorno. Falla si falta DATABASE_URL,
// que es obligatoria en todos los entornos.
func Load() (*Config, error) {
	cfg := &Config{
		AppName:          getEnv("APP_NAME", "tiendas-track-backend"),
		Env:              getEnv("APP_ENV", "development"),
		HTTPAddr:         getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		DBConnectTimeout: getDuration("DB_CONNECT_TIMEOUT", 30*time.Second),
		MigrateOnStartup: getBool("MIGRATE_ON_STARTUP", true),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		LogFormat:        getEnv("LOG_FORMAT", "text"),
		ShutdownTimeout:  getDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	maxConns, err := getInt32("DB_MAX_CONNS", 10)
	if err != nil {
		return nil, err
	}
	cfg.DBMaxConns = maxConns

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL es obligatoria")
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func getInt32(key string, def int32) (int32, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("config: %s inválida (%q)", key, v)
	}
	return int32(n), nil
}
