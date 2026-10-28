package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/token"
)

func svc(t *testing.T) *token.Service {
	t.Helper()
	s, err := token.New("secreto-de-desarrollo-con-mas-de-32-caracteres",
		15*time.Minute, 720*time.Hour, 48*time.Hour)
	if err != nil {
		t.Fatalf("token.New: %v", err)
	}
	return s
}

func newAuth(t *testing.T, roles ...string) (http.Handler, *bool) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := Authenticate(svc(t), log)
	called := new(bool)
	var inner http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		p, _ := PrincipalFrom(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(p.Role))
	})
	if len(roles) > 0 {
		inner = RequireRole(roles...)(inner)
	}
	return auth(inner), called
}

// Sin cabecera Authorization → 401, y el handler protegido NO se ejecuta.
func TestAuthenticateSinToken401(t *testing.T) {
	h, called := newAuth(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
	if *called {
		t.Error("el handler protegido no debe ejecutarse sin token")
	}
}

// Un token de acceso válido pasa y el principal queda en el contexto.
func TestAuthenticateTokenValido(t *testing.T) {
	h, called := newAuth(t)
	raw, err := svc(t).IssueAccess("u1", token.Identity{Role: "STORE", StoreID: "s1"})
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+raw.Raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !*called {
		t.Fatalf("status = %d, called = %v", rec.Code, *called)
	}
}

// Un refresh token no sirve como credenciales de acceso.
func TestAuthenticateRechazaRefresh(t *testing.T) {
	h, called := newAuth(t)
	raw, _ := svc(t).IssueRefresh("u1")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+raw.Raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401 para refresh usado como access", rec.Code)
	}
	if *called {
		t.Error("el handler no debe ejecutarse con refresh token")
	}
}

// RequireRole: rol correcto pasa; rol distinto y invitado → 403.
func TestRequireRole(t *testing.T) {
	cases := []struct {
		name    string
		role    string
		isGuest bool
		want    int
	}{
		{"rol permitido", "PROVIDER", false, http.StatusOK},
		{"rol no permitido", "DRIVER", false, http.StatusForbidden},
		{"invitado siempre 403", "STORE", true, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, called := newAuth(t, "PROVIDER")
			var raw token.Issued
			var err error
			if tc.isGuest {
				raw, err = svc(t).IssueGuest("orden-1")
			} else {
				raw, err = svc(t).IssueAccess("u1", token.Identity{Role: tc.role})
			}
			if err != nil {
				t.Fatalf("emitir token: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set("Authorization", "Bearer "+raw.Raw)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("status = %d, esperado %d", rec.Code, tc.want)
			}
			if (rec.Code == http.StatusOK) != *called {
				t.Errorf("handler ejecutado=%v con status %d", *called, rec.Code)
			}
		})
	}
}

// El principal GuestOrderID extrae la orden del scope.
func TestGuestOrderID(t *testing.T) {
	p := authz.Principal{TokenType: authz.GuestToken, Scope: "order:abc"}
	id, ok := p.GuestOrderID()
	if !ok || id != "abc" {
		t.Errorf("GuestOrderID = (%q, %v)", id, ok)
	}
	if _, ok := (authz.Principal{TokenType: authz.AccessToken, Scope: "order:abc"}).GuestOrderID(); ok {
		t.Error("un access token no tiene orden de invitado")
	}
}
