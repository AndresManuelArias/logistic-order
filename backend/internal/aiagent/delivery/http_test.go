package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tiendas-track/backend/internal/aiagent/domain"
	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/middleware"
	"tiendas-track/backend/internal/shared/resilience"
)

type fakeAgent struct {
	ans          *domain.Answer
	err          error
	gotQuestion  string
	gotPrincipal authz.Principal
	calls        int
}

func (f *fakeAgent) Query(_ context.Context, p authz.Principal, question string) (*domain.Answer, error) {
	f.calls++
	f.gotPrincipal, f.gotQuestion = p, question
	return f.ans, f.err
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func principal() authz.Principal {
	return authz.Principal{
		UserID:     "user-1",
		Role:       authz.RoleProvider,
		ProviderID: "provider-1",
		TokenType:  authz.AccessToken,
	}
}

func muxWith(agent Agent, p authz.Principal) *http.ServeMux {
	h := NewHandler(agent, testLogger())
	mux := http.NewServeMux()
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(middleware.WithPrincipal(r.Context(), p)))
		})
	}
	h.RegisterRoutes(mux, auth, middleware.RequireRole)
	return mux
}

func doQuery(t *testing.T, mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/query", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("respuesta de error inválida: %v (%s)", err, rec.Body.String())
	}
	return env.Error.Code
}

func TestQueryOK(t *testing.T) {
	agent := &fakeAgent{ans: &domain.Answer{
		Answer:  "ABC123 lleva 27 minutos detenido.",
		Tool:    "get_stopped_vehicles",
		Model:   "fake-model",
		AuditID: "audit-1",
		Tools: []domain.ToolInvocation{{
			Name:       "get_stopped_vehicles",
			Arguments:  json.RawMessage(`{"minutes":20}`),
			OK:         true,
			DurationMS: 7,
		}},
	}}
	rec := doQuery(t, muxWith(agent, principal()), `{"question":"¿vehículos detenidos?"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp queryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body inválido: %v", err)
	}
	if resp.Answer != "ABC123 lleva 27 minutos detenido." || resp.Tool != "get_stopped_vehicles" ||
		resp.AuditID != "audit-1" || len(resp.Tools) != 1 {
		t.Errorf("respuesta = %+v", resp)
	}
	if agent.gotQuestion != "¿vehículos detenidos?" || agent.gotPrincipal.UserID != "user-1" {
		t.Errorf("delegación = %+v / %q", agent.gotPrincipal, agent.gotQuestion)
	}
}

func TestQueryPreguntaVacia(t *testing.T) {
	agent := &fakeAgent{}
	rec := doQuery(t, muxWith(agent, principal()), `{"question":"   "}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, esperado 422", rec.Code)
	}
	if code := decodeError(t, rec); code != "INVALID_QUESTION" {
		t.Errorf("code = %q", code)
	}
	if agent.calls != 0 {
		t.Error("no debería delegar una pregunta vacía")
	}
}

func TestQueryCampoDesconocido(t *testing.T) {
	rec := doQuery(t, muxWith(&fakeAgent{}, principal()), `{"question":"hola","sql":"DROP TABLE"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, esperado 422", rec.Code)
	}
}

func TestQueryCuerpoDemasiadoGrande(t *testing.T) {
	body := `{"question":"` + strings.Repeat("x", 9<<10) + `"}`
	rec := doQuery(t, muxWith(&fakeAgent{}, principal()), body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, esperado 422", rec.Code)
	}
}

func TestQuerySinPrincipal(t *testing.T) {
	h := NewHandler(&fakeAgent{}, testLogger())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, func(next http.Handler) http.Handler { return next }, middleware.RequireRole)

	rec := doQuery(t, mux, `{"question":"hola"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
}

func TestQueryInvitadoForbidden(t *testing.T) {
	p := authz.Principal{UserID: "guest", TokenType: authz.GuestToken, Scope: "order:o-1"}
	agent := &fakeAgent{}
	rec := doQuery(t, muxWith(agent, p), `{"question":"hola"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, esperado 403", rec.Code)
	}
	if agent.calls != 0 {
		t.Error("el invitado no debe llegar al caso de uso")
	}
}

func TestQueryErroresMapeados(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantCode  int
		wantError string
	}{
		{"forbidden", domain.ErrForbidden, http.StatusForbidden, "FORBIDDEN_SCOPE"},
		{"not configured", domain.ErrNotConfigured, http.StatusServiceUnavailable, "AI_NOT_CONFIGURED"},
		{"circuit open", resilience.ErrCircuitOpen, http.StatusServiceUnavailable, "CIRCUIT_OPEN"},
		{"timeout", domain.ErrTimeout, http.StatusGatewayTimeout, "AI_TIMEOUT"},
		{"provider", domain.ErrProvider, http.StatusBadGateway, "AI_PROVIDER_ERROR"},
		{"no answer", domain.ErrNoAnswer, http.StatusBadGateway, "AI_INVALID_RESPONSE"},
		{"unexpected", errors.New("boom"), http.StatusInternalServerError, "INTERNAL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doQuery(t, muxWith(&fakeAgent{err: tc.err}, principal()), `{"question":"hola"}`)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, esperado %d (%s)", rec.Code, tc.wantCode, rec.Body.String())
			}
			if code := decodeError(t, rec); code != tc.wantError {
				t.Errorf("code = %q, esperado %q", code, tc.wantError)
			}
			if tc.wantError == "CIRCUIT_OPEN" && rec.Header().Get("Retry-After") == "" {
				t.Error("CIRCUIT_OPEN debe incluir Retry-After")
			}
		})
	}
}
