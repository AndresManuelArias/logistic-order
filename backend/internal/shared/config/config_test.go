package config

import (
	"testing"
	"time"
)

// Load falla sin DATABASE_URL (obligatoria en todos los entornos).
func TestLoadSinDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	if _, err := Load(); err == nil {
		t.Fatal("se esperaba error sin DATABASE_URL")
	}
}

// Load con valores por defecto válidos.
func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("DB_MAX_CONNS", "")
	t.Setenv("BCRYPT_COST", "")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("JWT_ACCESS_TTL", "")
	t.Setenv("JWT_REFRESH_TTL", "")
	t.Setenv("GUEST_TOKEN_TTL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, esperado :8080", cfg.HTTPAddr)
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("DBMaxConns = %d, esperado 10", cfg.DBMaxConns)
	}
	if !cfg.MigrateOnStartup {
		t.Error("MigrateOnStartup debería ser true por defecto")
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout = %v, esperado 10s", cfg.ShutdownTimeout)
	}
	if cfg.JWTAccessTTL != 15*time.Minute {
		t.Errorf("JWTAccessTTL = %v, esperado 15m", cfg.JWTAccessTTL)
	}
	if cfg.JWTRefreshTTL != 720*time.Hour {
		t.Errorf("JWTRefreshTTL = %v, esperado 720h", cfg.JWTRefreshTTL)
	}
	if cfg.GuestTokenTTL != 48*time.Hour {
		t.Errorf("GuestTokenTTL = %v, esperado 48h", cfg.GuestTokenTTL)
	}
	if cfg.BcryptCost != 10 {
		t.Errorf("BcryptCost = %d, esperado 10", cfg.BcryptCost)
	}
}

// JWT_SECRET es secreto: sin ella (o corta) el servidor no arranca.
func TestLoadSinJWTSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("se esperaba error sin JWT_SECRET")
	}

	t.Setenv("JWT_SECRET", "corto")
	if _, err := Load(); err == nil {
		t.Fatal("se esperaba error con JWT_SECRET < 32 caracteres")
	}
}

// DB_MAX_CONNS inválida produce error explícito (fallo detectable, no silencioso).
func TestLoadMaxConnsInvalida(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("DB_MAX_CONNS", "no-numero")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	if _, err := Load(); err == nil {
		t.Fatal("se esperaba error con DB_MAX_CONNS inválida")
	}
}
