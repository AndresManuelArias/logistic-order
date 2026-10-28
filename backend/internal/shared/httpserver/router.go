// Package httpserver arma el router HTTP del monolito modular.
// Endpoints F1: /healthz (liveness), /readyz (readiness con ping a BD).
// Endpoints F2: módulos auth y orders + documentación OpenAPI (ADR-10).
package httpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	authdelivery "tiendas-track/backend/internal/auth/delivery"
	ordersdelivery "tiendas-track/backend/internal/orders/delivery"
	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/middleware"
	"tiendas-track/backend/internal/shared/openapi"
	"tiendas-track/backend/internal/shared/token"
	storesdelivery "tiendas-track/backend/internal/stores/delivery"
	telemetrydelivery "tiendas-track/backend/internal/telemetry/delivery"
	"tiendas-track/backend/internal/tracing"
)

// Modules agrupa los handlers de los módulos de negocio a montar en el router.
// Un módulo no montado (nil) simplemente no publica sus rutas.
type Modules struct {
	Auth      *authdelivery.Handler
	Orders    *ordersdelivery.Handler
	Stores    *storesdelivery.Handler
	Telemetry *telemetrydelivery.Handler
}

type router struct {
	log *slog.Logger
	db  *pgxpool.Pool
}

// Options son las dependencias transversales del router (F2).
type Options struct {
	// Token es el servicio JWT compartido; obligatorio si hay módulos.
	Token *token.Service
	// Modules son los handlers de negocio (auth, orders).
	Modules Modules
}

// New construye el handler raíz del servidor (solo operabilidad, F1).
func New(log *slog.Logger, db *pgxpool.Pool) http.Handler {
	return NewWithModules(log, db, Options{})
}

// NewWithModules construye el handler raíz con los módulos de negocio (F2).
// La autenticación y el RBAC de rol se aplican por ruta dentro de cada módulo:
// las rutas de operabilidad (/healthz, /readyz, /swagger) son públicas.
func NewWithModules(log *slog.Logger, db *pgxpool.Pool, opts Options) http.Handler {
	r := &router{log: log, db: db}

	mux := http.NewServeMux()

	// F1: operabilidad.
	mux.HandleFunc("GET /healthz", r.healthz)
	mux.HandleFunc("GET /readyz", r.readyz)
	mux.HandleFunc("GET /{$}", r.root)

	// F2: documentación OpenAPI embebida (ADR-10), siempre disponible.
	openapi.Register(mux)

	// F2/F3: módulos de negocio tras middleware de autenticación.
	anyModule := opts.Modules.Auth != nil || opts.Modules.Orders != nil ||
		opts.Modules.Stores != nil || opts.Modules.Telemetry != nil
	if opts.Token != nil && anyModule {
		authenticate := middleware.Authenticate(opts.Token, log)
		if opts.Modules.Auth != nil {
			opts.Modules.Auth.RegisterRoutes(mux, authenticate)
		}
		if opts.Modules.Orders != nil {
			opts.Modules.Orders.RegisterRoutes(mux, authenticate, middleware.RequireRole)
		}
		if opts.Modules.Stores != nil {
			opts.Modules.Stores.RegisterRoutes(mux, authenticate, middleware.RequireRole)
		}
		if opts.Modules.Telemetry != nil {
			opts.Modules.Telemetry.RegisterRoutes(mux, authenticate, middleware.RequireRole)
			// /ws no cuelga de /api/v1 y autentica el handshake por sí mismo.
			opts.Modules.Telemetry.RegisterWS(mux)
		}
	} else if opts.Token == nil {
		log.Warn("router: sin JWT_SECRET/servicio de token; los módulos de negocio no se montan")
	}

	return r.logging(r.recover(tracing.TracingMiddleware(mux)))
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
		"docs":    "/swagger/index.html (ADR-10)",
		"openapi": "/api/v1/openapi.json",
		"healthz": "/healthz",
		"readyz":  "/readyz",
	})
}

// logging agrega logging estructurado con correlation ID a cada request.
func (r *router) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		traceID, _ := tracing.CorrelationIDFromContext(req.Context())

		r.log.Info("request iniciada",
			"trace_id", traceID,
			"method", req.Method,
			"path", req.URL.Path,
			"remote", req.RemoteAddr,
		)

		// Capture response status code
		rr := newResponseRecorder(w)
		next.ServeHTTP(rr, req)

		elapsed := time.Since(start)
		r.log.Info("request completada",
			"trace_id", traceID,
			"method", req.Method,
			"path", req.URL.Path,
			"status", rr.status,
			"duration_ms", elapsed.Milliseconds(),
		)
	})
}

func (r *router) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				traceID, _ := tracing.CorrelationIDFromContext(req.Context())
				r.log.Error("panic atendiendo petición", "trace_id", traceID, "path", req.URL.Path, "panic", rec)
				httperr.Write(w, http.StatusInternalServerError, "INTERNAL", "error interno")
			}
		}()
		next.ServeHTTP(w, req)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: w, status: http.StatusOK}
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.status = code
	rr.ResponseWriter.WriteHeader(code)
}

// Unwrap expone el writer original a http.ResponseController (Go 1.20+).
func (rr *responseRecorder) Unwrap() http.ResponseWriter { return rr.ResponseWriter }

// Hijack permite que gorilla/websocket haga el upgrade a través del
// middleware de logging (ADR-03): sin esto /ws responde 500.
func (rr *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rr.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("httpserver: el writer no soporta Hijack")
	}
	rr.status = http.StatusSwitchingProtocols
	return hijacker.Hijack()
}

// Flush mantiene el soporte de streaming (SSE) a través del middleware.
func (rr *responseRecorder) Flush() {
	if f, ok := rr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
