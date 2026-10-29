package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tiendas-track/backend/internal/shared/ratelimit"
	"tiendas-track/backend/internal/shared/token"
)

func rlTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testMux() *http.ServeMux {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("POST /api/v1/auth/login", ok)
	mux.HandleFunc("POST /api/v1/telemetry/points", ok)
	mux.HandleFunc("GET /healthz", ok)
	return mux
}

func TestRateLimitLoginPorIP(t *testing.T) {
	r := &router{log: rlTestLogger()}
	h := r.rateLimit(testMux(), &RateLimits{Login: ratelimit.New(1)}, nil)

	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("primera petición = %d, esperado 200", first.Code)
	}

	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("segunda petición = %d, esperado 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("falta Retry-After en 429")
	}
}

func TestRateLimitNoAfectaRutasPublicas(t *testing.T) {
	r := &router{log: rlTestLogger()}
	h := r.rateLimit(testMux(), &RateLimits{Login: ratelimit.New(1)}, nil)
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("healthz = %d, esperado 200", rec.Code)
		}
	}
}

func TestRateLimitTelemetriaUsaUsuario(t *testing.T) {
	svc, err := token.New("secreto-de-pruebas-con-mas-de-32-caracteres!!", time.Minute, time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("token.New: %v", err)
	}
	r := &router{log: rlTestLogger()}
	h := r.rateLimit(testMux(), &RateLimits{Telemetry: ratelimit.New(1)}, svc)

	newReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry/points", nil)
		issued, err := svc.IssueAccess("driver-1", token.Identity{Role: "DRIVER", DriverID: "d1"})
		if err != nil {
			t.Fatalf("IssueAccess: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+issued.Raw)
		return req
	}

	if rec := httptest.NewRecorder(); func() int {
		h.ServeHTTP(rec, newReq())
		return rec.Code
	}() != http.StatusOK {
		t.Fatalf("primera petición = %d, esperado 200", rec.Code)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq())
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("segunda petición = %d, esperado 429 (mismo usuario)", rec.Code)
	}
}
