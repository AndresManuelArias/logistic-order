package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tiendas-track/backend/internal/shared/token"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func testToken(t *testing.T) *token.Service {
	t.Helper()
	svc, err := token.New("secreto-de-desarrollo-con-mas-de-32-caracteres",
		15*time.Minute, 720*time.Hour, 48*time.Hour)
	if err != nil {
		t.Fatalf("token.New: %v", err)
	}
	return svc
}

// Con servicio JWT las rutas de módulos y documentación quedan montadas.
func TestRouterMontaModulosYDocumentacion(t *testing.T) {
	log := testLogger()
	h := NewWithModules(log, nil, Options{Token: testToken(t)}) // sin handlers: solo doc

	cases := []struct {
		url  string
		want int
	}{
		{"/healthz", http.StatusOK},
		{"/api/v1/openapi.json", http.StatusOK},
		{"/swagger/index.html", http.StatusOK},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
		if rec.Code != tc.want {
			t.Errorf("GET %s: status = %d, esperado %d", tc.url, rec.Code, tc.want)
		}
	}

	// Rutas de módulos sin handler registrado → 404 (no 500).
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil))
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/orders sin módulos: status = %d", rec.Code)
	}
}

// Sin servicio JWT no se monta ningún módulo de negocio (fail-safe: no
// servir APIs de negocio sin capacidad de autenticar).
func TestRouterSinTokenNoMontaModulos(t *testing.T) {
	log := testLogger()
	h := New(log, nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("healthz = %d", rec.Code)
	}
	// La documentación sí es pública (especificación, sin datos).
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("openapi.yaml = %d", rec.Code)
	}
}
