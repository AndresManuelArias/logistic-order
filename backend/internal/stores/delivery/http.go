// Package delivery: handlers HTTP del módulo stores. Sin lógica de negocio:
// solo DTOs, validación de forma y mapeo de errores de dominio a HTTP.
package delivery

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

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

// RegisterRoutes monta /api/v1/stores/*. Ambos endpoints requieren rol STORE.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler, roles func(...string) func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/stores", auth(roles("STORE")(http.HandlerFunc(h.Create))))
	mux.Handle("GET /api/v1/stores/me", auth(roles("STORE")(http.HandlerFunc(h.Me))))
	mux.Handle("GET /api/v1/stores", auth(roles("PROVIDER")(http.HandlerFunc(h.List))))
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toStoreResponse(s))
}

// List maneja GET /api/v1/stores para proveedores.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p, _ := middleware.PrincipalFrom(r.Context())
	stores, err := h.uc.List(r.Context(), p)
	if err != nil {
		h.mapError(w, r.Header.Get("X-Correlation-ID"), "List", err)
		return
	}
	response := make([]storeResponse, 0, len(stores))
	for i := range stores {
		response = append(response, toStoreResponse(&stores[i]))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toStoreResponse(s))
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
		httperr.Forbidden(w, "acceso denegado: se requiere rol STORE")
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
