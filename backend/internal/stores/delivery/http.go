// Package delivery: handlers HTTP del módulo stores. Sin lógica de negocio:
// solo DTOs, validación de forma y mapeo de errores de dominio a HTTP.
package delivery

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/middleware"
	"tiendas-track/backend/internal/stores/domain"
	"tiendas-track/backend/internal/stores/usecase"
	"tiendas-track/backend/internal/tracing"
)

// Handler expone los endpoints de tiendas.
type Handler struct {
	uc  *usecase.StoresUseCase
	log *slog.Logger
}

// NewHandler crea el handler.
func NewHandler(uc *usecase.StoresUseCase, log *slog.Logger) *Handler {
	return &Handler{uc: uc, log: log}
}

// RegisterRoutes monta /api/v1/stores/*.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler, roles func(...string) func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/stores", auth(roles("STORE")(http.HandlerFunc(h.Create))))
	mux.Handle("GET /api/v1/stores/me", auth(roles("STORE")(http.HandlerFunc(h.Me))))
	// GET /api/v1/stores — disponible para cualquier rol autenticado (no guest).
	mux.Handle("GET /api/v1/stores", auth(http.HandlerFunc(h.List)))
}

type storeRequest struct {
	Name     string    `json:"name"`
	Address  string    `json:"address"`
	Location *pointDTO `json:"location"`
}

// pointDTO es una coordenada WGS84 (misma forma que el payload store del registro).
type pointDTO struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type storeResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	OwnerID   string  `json:"owner_user_id"`
	Address   string  `json:"address,omitempty"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	CreatedAt string  `json:"created_at"`
}

func toStoreResponse(s *domain.Store) storeResponse {
	return storeResponse{
		ID:        s.ID,
		Name:      s.Name,
		OwnerID:   s.OwnerUserID,
		Address:   s.Address,
		Lat:       s.Lat,
		Lon:       s.Lon,
		CreatedAt: s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// Create maneja POST /api/v1/stores (alta de tienda, paso propio del registro).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())

	var req storeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid_json", "cuerpo de la petición inválido")
		return
	}

	in := usecase.CreateInput{Name: req.Name, Address: req.Address}
	if req.Location != nil { // sin location → 0,0 → rechazado por Valid() (ADR-09)
		in.Lat = req.Location.Lat
		in.Lon = req.Location.Lon
	}

	p, _ := middleware.PrincipalFrom(r.Context())
	s, err := h.uc.Create(r.Context(), p, in)
	if err != nil {
		h.mapError(w, traceID, "Create", err)
		return
	}

	w.Header().Set("Location", "/api/v1/stores/"+s.ID)
	writeJSON(w, http.StatusCreated, toStoreResponse(s))
}

// List maneja GET /api/v1/stores?name=&limit=&offset=
// Devuelve la lista paginada de tiendas disponibles.
// Accesible para cualquier rol autenticado (PROVIDER, STORE, DRIVER).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		h.log.Error("List: no autenticado", "trace_id", traceID)
		httperr.Unauthenticated(w, "no autenticado")
		return
	}

	q := r.URL.Query()
	name := q.Get("name")

	limit := 50
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 200 {
			httperr.BadRequest(w, "INVALID_LIMIT", "limit debe ser un entero entre 1 y 200")
			return
		}
		limit = n
	}

	offset := 0
	if raw := q.Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			httperr.BadRequest(w, "INVALID_OFFSET", "offset debe ser un entero no negativo")
			return
		}
		offset = n
	}

	h.log.Info("List: consultando tiendas",
		"trace_id", traceID,
		"name", name,
		"limit", limit,
		"offset", offset,
	)

	stores, total, err := h.uc.List(r.Context(), p, usecase.ListInput{
		Name:   name,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		h.mapError(w, traceID, "List", err)
		return
	}

	items := make([]storeResponse, 0, len(stores))
	for i := range stores {
		items = append(items, toStoreResponse(&stores[i]))
	}

	h.log.Info("List: respondiendo",
		"trace_id", traceID,
		"count", len(items),
		"total", total,
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Me maneja GET /api/v1/stores/me.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())

	p, _ := middleware.PrincipalFrom(r.Context())
	s, err := h.uc.Me(r.Context(), p)
	if err != nil {
		h.mapError(w, traceID, "Me", err)
		return
	}

	writeJSON(w, http.StatusOK, toStoreResponse(s))
}

// mapError traduce errores de dominio a respuestas HTTP.
func (h *Handler) mapError(w http.ResponseWriter, traceID, op string, err error) {
	h.log.Error("stores handler: error de negocio",
		"trace_id", traceID,
		"op", op,
		"error", err.Error(),
	)
	switch {
	case errors.Is(err, domain.ErrForbiddenScope):
		httperr.Forbidden(w, "acceso denegado: se requiere autenticación")
	case errors.Is(err, domain.ErrAlreadyExists):
		httperr.Conflict(w, "store_already_exists", "el usuario ya tiene una tienda registrada")
	case errors.Is(err, domain.ErrNotFound):
		httperr.NotFound(w, "el usuario aún no tiene tienda registrada")
	case errors.Is(err, domain.ErrValidation):
		httperr.Validation(w, "store_invalid", err.Error())
	default:
		httperr.Internal(w)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
