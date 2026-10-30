package migrations

import (
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// Las migraciones embebidas existen, son ordenables y con sufijo .sql.
func TestMigracionesEmbebidas(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var sqls []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			sqls = append(sqls, e.Name())
		}
	}
	if len(sqls) == 0 {
		t.Fatal("no hay migraciones SQL embebidas")
	}
	if !sort.StringsAreSorted(sqls) {
		t.Errorf("los nombres no están ordenados: %v", sqls)
	}
}

// Migración crítica presente: hipertable + continuous aggregate.
func TestMigracionTelemetryPresente(t *testing.T) {
	data, err := FS.ReadFile("0004_telemetry.sql")
	if err != nil {
		t.Fatalf("0004_telemetry.sql no encontrada: %v", err)
	}
	for _, want := range []string{"create_hypertable", "timescaledb.continuous", "add_retention_policy"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("0004_telemetry.sql debería contener %q", want)
		}
	}
}

// Migración crítica presente: PostGIS + orders.destination (ADR-09).
func TestMigracionPostGISPresente(t *testing.T) {
	data, err := FS.ReadFile("0001_extensions.sql")
	if err != nil {
		t.Fatalf("0001_extensions.sql no encontrada: %v", err)
	}
	if !strings.Contains(string(data), "postgis") {
		t.Error("0001 debería crear la extensión postgis")
	}

	orders, err := FS.ReadFile("0003_app_tables.sql")
	if err != nil {
		t.Fatalf("0003_app_tables.sql no encontrada: %v", err)
	}
	if !strings.Contains(string(orders), "destination") || !strings.Contains(string(orders), "geometry(Point, 4326)") {
		t.Error("0003 debería definir orders.destination como geometry(Point, 4326)")
	}
}

// Migración 0008: alianzas proveedor-tienda con unicidad por par.
func TestMigracionProviderStoreAgreements(t *testing.T) {
	data, err := FS.ReadFile("0008_provider_store_agreements.sql")
	if err != nil {
		t.Fatalf("0008_provider_store_agreements.sql no encontrada: %v", err)
	}
	sql := string(data)
	for _, want := range []string{
		"CREATE TABLE app.provider_store_agreements",
		"REFERENCES app.users(id)",
		"REFERENCES app.stores(id)",
		"UNIQUE (provider_id, store_id)",
		"idx_stores_location_geography",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("0008 debería contener %q", want)
		}
	}
}
