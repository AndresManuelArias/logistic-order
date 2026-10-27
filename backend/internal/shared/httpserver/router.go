// Package httpserver arma el router HTTP del monolito.
// Endpoints de F1: healthz (liveness) y readyz (readiness con ping a BD).
package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type router struct {
	log *slog.Logger
	db  *pgxpool.Pool
}

// New construye el handler raíz del servidor.
func New(log *slog.Logger, db *pgxpool.Pool) http.Handler {
	r := &router{log: log, db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", r.healthz)
	mux.HandleFunc("GET /readyz", r.readyz)
	mux.HandleFunc("GET /", r.root)
	return r.recover(mux)
}

// healthz: liveness. No toca la base de datos: sirve para saber si el
// proceso está vivo (contenedor healthy en Docker Compose).
func (r *router) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyz: readiness. Comprueba que la base de datos responde.
func (r *router) readyz(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "no database pool"})
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
	defer cancel()
	if err := r.db.Ping(ctx); err != nil {
		r.log.Warn("readyz: base de datos no responde", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "database unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// root: información mínima del servicio (evita 404 raíz confusos).
func (r *router) root(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "tiendas-track-backend",
		"docs":    "/swagger/index.html (F2, ADR-10)",
		"healthz": "/healthz",
		"readyz":  "/readyz",
	})
}

func (r *router) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				r.log.Error("panic atendiendo petición", "path", req.URL.Path, "panic", rec)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			}
		}()
		next.ServeHTTP(w, req)
	})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
