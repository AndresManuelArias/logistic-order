package ratelimit

import (
	"net"
	"net/http"
	"strings"

	"tiendas-track/backend/internal/shared/httperr"
)

// KeyFunc obtiene la clave de limitación de una petición.
type KeyFunc func(*http.Request) string

// Middleware rechaza con 429 y Retry-After cuando la clave supera el límite.
// El handler protegido no se ejecuta si se supera el límite.
func Middleware(l *Limiter, key KeyFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if l == nil || key == nil {
				next.ServeHTTP(w, r)
				return
			}
			allowed, retryAfter := l.Allow(key(r))
			if !allowed {
				httperr.TooManyRequests(w, retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP extrae la IP del cliente. Por defecto usa RemoteAddr y no confía en
// cabeceras reenviadas para evitar suplantación; con trustProxy=true usa el
// primer valor de X-Forwarded-For (necesario detrás de un ALB/ingress).
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
			return real
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// FixedWindowKey devuelve una clave estable "scope:valor" para separar los
// contadores de distintos endpoints (p. ej. "login:1.2.3.4").
func FixedWindowKey(scope, value string) string {
	if value == "" {
		value = "unknown"
	}
	return scope + ":" + value
}
