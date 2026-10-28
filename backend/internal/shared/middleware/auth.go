// Package middleware contiene cross-cutting HTTP: autenticación JWT y RBAC.
// El middleware NO ejecuta lógica de negocio: solo resuelve el principal
// y deja el alcance de datos en el contexto para los casos de uso.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"log/slog"

	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/token"
)

type ctxKey int

const principalKey ctxKey = iota

// PrincipalFrom extrae el principal del contexto (ok=false si no hay).
func PrincipalFrom(ctx context.Context) (authz.Principal, bool) {
	p, ok := ctx.Value(principalKey).(authz.Principal)
	return p, ok
}

// WithPrincipal inyecta un principal (usado por pruebas).
func WithPrincipal(ctx context.Context, p authz.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// Authenticate resuelve el token Bearer (access o guest) y lo deja en contexto.
func Authenticate(svc *token.Service, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := BearerToken(r)
			if !ok {
				httperr.Unauthenticated(w, "token requerido")
				return
			}
			p, err := PrincipalFromToken(svc, raw)
			if err != nil {
				log.Debug("token inválido", "error", err)
				httperr.Unauthenticated(w, "token inválido o expirado")
				return
			}

			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// BearerToken extrae el token del header Authorization.
func BearerToken(r *http.Request) (string, bool) {
	return bearerToken(r.Header.Get("Authorization"))
}

// PrincipalFromToken valida un token en claro y devuelve el principal.
// Solo admite access tokens: refresh/otros tipos no autentican peticiones.
func PrincipalFromToken(svc *token.Service, raw string) (authz.Principal, error) {
	claims, err := svc.Parse(raw)
	if err != nil {
		return authz.Principal{}, err
	}

	p := authz.Principal{
		UserID:     claims.Subject,
		Role:       claims.Role,
		StoreID:    claims.StoreID,
		ProviderID: claims.ProviderID,
		DriverID:   claims.DriverID,
		Scope:      claims.Scope,
	}
	if claims.ExpiresAt != nil {
		p.ExpiresAt = claims.ExpiresAt.Time
	}

	switch claims.Kind {
	case token.KindAccess:
		p.TokenType = authz.AccessToken
	case token.KindGuest:
		p.TokenType = authz.GuestToken
	default:
		return authz.Principal{}, errors.New("tipo de token no permitido aquí")
	}
	return p, nil
}

// RequireRole rechaza con 403 si el principal no tiene el rol indicado.
// Para invitar a múltiples roles, encadena RequireRole(ROLE_A, ROLE_B).
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				httperr.Unauthenticated(w, "no autenticado")
				return
			}
			if p.IsGuest() {
				httperr.Forbidden(w, "acceso denegado: token de invitado")
				return
			}
			for _, role := range roles {
				if p.HasRole(role) {
					next.ServeHTTP(w, r)
					return
				}
			}
			httperr.Forbidden(w, "acceso denegado para este rol")
		})
	}
}

func bearerToken(h string) (string, bool) {
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	t := strings.TrimSpace(h[len(prefix):])
	return t, t != ""
}
