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

	// F2: autenticación JWT (RF-01, ADR-04). El secreto llega solo por entorno.
	JWTSecret     string
	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration
	GuestTokenTTL time.Duration
	BcryptCost    int
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

	cost, err := getInt32("BCRYPT_COST", 10)
	if err != nil {
		return nil, err
	}
	cfg.BcryptCost = int(cost)

	// Autenticación (F2). SECRETOS: sin valor por defecto en código.
	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	cfg.JWTAccessTTL = getDuration("JWT_ACCESS_TTL", 15*time.Minute)
	cfg.JWTRefreshTTL = getDuration("JWT_REFRESH_TTL", 720*time.Hour) // 30 días
	cfg.GuestTokenTTL = getDuration("GUEST_TOKEN_TTL", 48*time.Hour)

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL es obligatoria")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("config: JWT_SECRET es obligatoria y debe tener al menos 32 caracteres")
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
