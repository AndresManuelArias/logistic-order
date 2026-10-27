package config

import (
	"testing"
	"time"
)

// Load falla sin DATABASE_URL (obligatoria en todos los entornos).
func TestLoadSinDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("se esperaba error sin DATABASE_URL")
	}
}

// Load con valores por defecto válidos.
func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("DB_MAX_CONNS", "")

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
}

// DB_MAX_CONNS inválida produce error explícito (fallo detectable, no silencioso).
func TestLoadMaxConnsInvalida(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("DB_MAX_CONNS", "no-numero")

	if _, err := Load(); err == nil {
		t.Fatal("se esperaba error con DB_MAX_CONNS inválida")
	}
}
