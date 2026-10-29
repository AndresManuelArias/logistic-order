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

	aidelivery "tiendas-track/backend/internal/aiagent/delivery"
	authdelivery "tiendas-track/backend/internal/auth/delivery"
	driversdelivery "tiendas-track/backend/internal/drivers/delivery"
	ordersdelivery "tiendas-track/backend/internal/orders/delivery"
	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/middleware"
	"tiendas-track/backend/internal/shared/openapi"
	"tiendas-track/backend/internal/shared/ratelimit"
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
	AI        *aidelivery.Handler
	Drivers   *driversdelivery.Handler
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
	// RateLimits protege login, ingesta de telemetría y agente IA (§9.4).
	// Si es nil, no se aplica limitación (modo test / despliegue sin límites).
	RateLimits *RateLimits
}

// RateLimits agrupa los limitadores por superficie sensible (F6).
type RateLimits struct {
	Login     *ratelimit.Limiter
	Telemetry *ratelimit.Limiter
	AI        *ratelimit.Limiter
	// TrustProxy permite usar X-Forwarded-For/X-Real-IP como clave de IP.
	// Solo debe activarse detrás de un proxy de confianza (ALB/ingress).
	TrustProxy bool
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
		opts.Modules.Stores != nil || opts.Modules.Telemetry != nil || opts.Modules.AI != nil ||
		opts.Modules.Drivers != nil
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
		if opts.Modules.AI != nil {
			opts.Modules.AI.RegisterRoutes(mux, authenticate, middleware.RequireRole)
		}
		if opts.Modules.Drivers != nil {
			opts.Modules.Drivers.RegisterRoutes(mux, authenticate, middleware.RequireRole)
		}
	} else if opts.Token == nil {
		log.Warn("router: sin JWT_SECRET/servicio de token; los módulos de negocio no se montan")
	}

	return r.logging(r.recover(tracing.TracingMiddleware(r.rateLimit(mux, opts.RateLimits, opts.Token))))
}

// rateLimit aplica el limitador correspondiente a cada superficie sensible.
// Se ejecuta antes del mux: para telemetría/IA intenta derivar la clave del
// Bearer (el token se valida de nuevo en el middleware de autenticación) y
// cae a la IP si no hay token válido. Login siempre limita por IP.
func (r *router) rateLimit(next http.Handler, limits *RateLimits, tokens *token.Service) http.Handler {
	if limits == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var (
			limiter *ratelimit.Limiter
			scope   string
		)
		switch req.URL.Path {
		case "/api/v1/auth/login":
			limiter, scope = limits.Login, "login"
		case "/api/v1/telemetry/points", "/api/v1/telemetry/bulk":
			limiter, scope = limits.Telemetry, "telemetry"
		case "/api/v1/ai/query":
			limiter, scope = limits.AI, "ai"
		default:
			next.ServeHTTP(w, req)
			return
		}
		if limiter == nil {
			next.ServeHTTP(w, req)
			return
		}

		key := ratelimit.ClientIP(req, limits.TrustProxy)
		if scope != "login" && tokens != nil {
			if raw, ok := middleware.BearerToken(req); ok {
				if p, err := middleware.PrincipalFromToken(tokens, raw); err == nil && p.UserID != "" {
					key = p.UserID
				}
			}
		}

		allowed, retryAfter := limiter.Allow(ratelimit.FixedWindowKey(scope, key))
		if !allowed {
			r.log.Warn("rate limit excedido", "scope", scope, "client", key, "retry_after", retryAfter.String())
			httperr.TooManyRequests(w, retryAfter)
			return
		}
		next.ServeHTTP(w, req)
	})
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
