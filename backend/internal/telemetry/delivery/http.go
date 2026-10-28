// Package delivery: handlers HTTP y WebSocket del módulo telemetría.
// Sin lógica de negocio: DTOs, validación de forma y mapeo de errores.
package delivery

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/middleware"
	"tiendas-track/backend/internal/shared/resilience"
	"tiendas-track/backend/internal/shared/token"
	"tiendas-track/backend/internal/shared/websocket"
	"tiendas-track/backend/internal/telemetry/domain"
	"tiendas-track/backend/internal/telemetry/usecase"
	"tiendas-track/backend/internal/tracing"
)

// maxBodyBytes acota el cuerpo de un bulk sync (1000 puntos ≈ < 500 KiB).
const maxBodyBytes = 1 << 20

// Handler expone los endpoints de telemetría y monta el canal /ws.
type Handler struct {
	uc     *usecase.TelemetryUseCase
	hub    *websocket.Hub
	tokens *token.Service
	log    *slog.Logger
}

// NewHandler crea el handler.
func NewHandler(uc *usecase.TelemetryUseCase, hub *websocket.Hub, tokens *token.Service, log *slog.Logger) *Handler {
	return &Handler{uc: uc, hub: hub, tokens: tokens, log: log}
}

// RegisterRoutes monta /api/v1/telemetry/* y /api/v1/vehicles/{id}/*.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler, roles func(...string) func(http.Handler) http.Handler) {
	ingest := func(h http.HandlerFunc) http.Handler {
		return auth(roles(authz.RoleDriver, authz.RoleProvider)(h))
	}
	mux.Handle("POST /api/v1/telemetry/points", ingest(h.Ingest))
	mux.Handle("POST /api/v1/telemetry/bulk", ingest(h.Bulk))
	mux.Handle("GET /api/v1/vehicles/{id}/latest", auth(http.HandlerFunc(h.Latest)))
	mux.Handle("GET /api/v1/vehicles/{id}/track", auth(http.HandlerFunc(h.Track)))
}

// RegisterWS monta GET /ws (upgrade autenticado por Bearer o access_token).
func (h *Handler) RegisterWS(mux *http.ServeMux) {
	mux.Handle("GET /ws", h.hub.Handler(h.authenticateWS))
}

// authenticateWS acepta el token en el header (clientes nativos) o en la query
// (navegadores, que no pueden fijar headers en el handshake). Los access tokens
// viven 15 min por defecto y la sesión WS se cierra al expirar.
func (h *Handler) authenticateWS(r *http.Request) (authz.Principal, error) {
	raw, ok := middleware.BearerToken(r)
	if !ok {
		raw = strings.TrimSpace(r.URL.Query().Get("access_token"))
	}
	if raw == "" {
		return authz.Principal{}, errors.New("token requerido")
	}
	return middleware.PrincipalFromToken(h.tokens, raw)
}

type pointRequest struct {
	VehicleID string   `json:"vehicle_id"`
	OrderID   string   `json:"order_id"`
	Time      string   `json:"time"`
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	SpeedKPH  *float64 `json:"speed_kph"`
	AccuracyM *float64 `json:"accuracy_m"`
	Heading   *float64 `json:"heading"`
	DeviceSeq int64    `json:"device_seq"`
}

type bulkRequest struct {
	DeviceID string         `json:"device_id"`
	Points   []pointRequest `json:"points"`
}

type pointDTO struct {
	VehicleID  string   `json:"vehicle_id"`
	OrderID    string   `json:"order_id,omitempty"`
	DeviceSeq  int64    `json:"device_seq"`
	Lat        float64  `json:"lat"`
	Lon        float64  `json:"lon"`
	SpeedKPH   *float64 `json:"speed_kph,omitempty"`
	AccuracyM  *float64 `json:"accuracy_m,omitempty"`
	Heading    *float64 `json:"heading,omitempty"`
	Source     string   `json:"source"`
	Time       string   `json:"time"`
	ReceivedAt string   `json:"received_at"`
}

func toPointDTO(p *domain.Point) pointDTO {
	return pointDTO{
		VehicleID:  p.VehicleID,
		OrderID:    p.OrderID,
		DeviceSeq:  p.DeviceSeq,
		Lat:        p.Lat,
		Lon:        p.Lon,
		SpeedKPH:   p.SpeedKPH,
		AccuracyM:  p.AccuracyM,
		Heading:    p.Heading,
		Source:     string(p.Source),
		Time:       p.Time.UTC().Format(time.RFC3339Nano),
		ReceivedAt: p.ReceivedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (r pointRequest) toInput() (usecase.PointInput, error) {
	ts, err := time.Parse(time.RFC3339Nano, r.Time)
	if err != nil {
		return usecase.PointInput{}, domain.ErrValidation
	}
	return usecase.PointInput{
		VehicleID: r.VehicleID,
		OrderID:   r.OrderID,
		Time:      ts,
		Lat:       r.Lat,
		Lon:       r.Lon,
		SpeedKPH:  r.SpeedKPH,
		AccuracyM: r.AccuracyM,
		Heading:   r.Heading,
		DeviceSeq: r.DeviceSeq,
	}, nil
}

// Ingest maneja POST /api/v1/telemetry/points (ingesta individual, app móvil).
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxBytesPerPoint)

	var req pointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo de la petición inválido")
		return
	}
	in, err := req.toInput()
	if err != nil {
		httperr.Validation(w, "TELEMETRY_INVALID", "time debe ser RFC3339 (ISO-8601)")
		return
	}

	p, _ := middleware.PrincipalFrom(r.Context())
	pt, duplicate, err := h.uc.Ingest(r.Context(), p, in)
	if err != nil {
		h.mapError(w, traceID, "Ingest", err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"point":     toPointDTO(pt),
		"duplicate": duplicate,
	})
}

const maxBytesPerPoint = 8 << 10

// Bulk maneja POST /api/v1/telemetry/bulk (Bulk Sync offline-first, RF-09).
func (h *Handler) Bulk(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		httperr.Validation(w, "IDEMPOTENCY_KEY_REQUIRED",
			"el header Idempotency-Key es obligatorio para el bulk sync")
		return
	}

	var req bulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo de la petición inválido")
		return
	}

	points := make([]usecase.PointInput, 0, len(req.Points))
	for i, pr := range req.Points {
		in, err := pr.toInput()
		if err != nil {
			httperr.Validation(w, "TELEMETRY_INVALID",
				"punto "+strconv.Itoa(i)+": time debe ser RFC3339 (ISO-8601)")
			return
		}
		points = append(points, in)
	}

	p, _ := middleware.PrincipalFrom(r.Context())
	res, err := h.uc.IngestBulk(r.Context(), p, usecase.BulkInput{
		IdempotencyKey: key,
		DeviceID:       req.DeviceID,
		Points:         points,
	})
	if err != nil {
		h.mapError(w, traceID, "Bulk", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"accepted":          len(res.Inserted),
		"duplicates":        res.Duplicates(len(points)),
		"already_processed": res.AlreadyProcessed,
		"processed_at":      res.ProcessedAt.UTC().Format(time.RFC3339Nano),
	})
}

// Latest maneja GET /api/v1/vehicles/{id}/latest.
func (h *Handler) Latest(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	vehicleID := r.PathValue("id")

	p, _ := middleware.PrincipalFrom(r.Context())
	pt, err := h.uc.Latest(r.Context(), p, vehicleID)
	if err != nil {
		h.mapError(w, traceID, "Latest", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"vehicle_id": vehicleID,
		"point":      toPointDTO(pt),
	})
}

// Track maneja GET /api/v1/vehicles/{id}/track?from=&to=&limit=.
func (h *Handler) Track(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	vehicleID := r.PathValue("id")

	q := r.URL.Query()
	from, errFrom := time.Parse(time.RFC3339Nano, q.Get("from"))
	to, errTo := time.Parse(time.RFC3339Nano, q.Get("to"))
	if errFrom != nil || errTo != nil {
		httperr.Validation(w, "INVALID_RANGE", "from y to son obligatorios en formato RFC3339")
		return
	}

	limit := usecase.DefaultTrackLimit
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httperr.Validation(w, "INVALID_LIMIT", "limit debe ser un entero")
			return
		}
		limit = n
	}

	p, _ := middleware.PrincipalFrom(r.Context())
	points, err := h.uc.Track(r.Context(), p, vehicleID, from, to, limit)
	if err != nil {
		h.mapError(w, traceID, "Track", err)
		return
	}

	out := make([]pointDTO, 0, len(points))
	for i := range points {
		out = append(out, toPointDTO(&points[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"vehicle_id": vehicleID,
		"from":       from.UTC().Format(time.RFC3339Nano),
		"to":         to.UTC().Format(time.RFC3339Nano),
		"count":      len(out),
		"points":     out,
	})
}

// mapError traduce errores de dominio/infraestructura a HTTP.
func (h *Handler) mapError(w http.ResponseWriter, traceID, op string, err error) {
	h.log.Error("telemetry handler: error",
		"trace_id", traceID, "op", op, "error", err.Error())

	switch {
	case errors.Is(err, domain.ErrForbiddenScope):
		httperr.Forbidden(w, "acceso denegado: el vehículo está fuera de tu scope")
	case errors.Is(err, domain.ErrIdempotencyKey):
		httperr.Validation(w, "IDEMPOTENCY_KEY_INVALID", err.Error())
	case errors.Is(err, domain.ErrValidation):
		httperr.Validation(w, "TELEMETRY_INVALID", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		httperr.NotFound(w, "no hay telemetría para el recurso solicitado")
	case errors.Is(err, resilience.ErrCircuitOpen):
		w.Header().Set("Retry-After", "10")
		httperr.ServiceUnavailable(w, "CIRCUIT_OPEN",
			"el servicio de lectura está temporalmente degradado; reintenta en unos segundos")
	default:
		httperr.Internal(w)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
