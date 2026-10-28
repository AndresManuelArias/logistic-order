// Package delivery: handlers HTTP del módulo órdenes. Sin lógica de negocio.
package delivery

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tiendas-track/backend/internal/orders/domain"
	"tiendas-track/backend/internal/orders/usecase"
	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/middleware"
)

// Handler expone los endpoints de órdenes.
type Handler struct {
	uc *usecase.OrdersUseCase
}

// NewHandler crea el handler.
func NewHandler(uc *usecase.OrdersUseCase) *Handler { return &Handler{uc: uc} }

// RegisterRoutes monta /api/v1/orders/*. GET /{id} permite invitados (scope).
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler, roles func(...string) func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/orders", auth(roles("STORE")(http.HandlerFunc(h.Create))))
	mux.Handle("GET /api/v1/orders", auth(http.HandlerFunc(h.List)))
	mux.Handle("GET /api/v1/orders/{id}", auth(http.HandlerFunc(h.Get)))
	mux.Handle("PATCH /api/v1/orders/{id}/status", auth(http.HandlerFunc(h.ChangeStatus)))
	mux.Handle("POST /api/v1/orders/{id}/assignment", auth(roles("PROVIDER")(http.HandlerFunc(h.Assign))))
}

type pointDTO struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type createOrderRequest struct {
	ExternalRef        string   `json:"external_ref"`
	StoreID            string   `json:"store_id"`
	CustomerID         *string  `json:"customer_id"`
	Origin             pointDTO `json:"origin"`
	Destination        pointDTO `json:"destination"`
	DestinationAddress string   `json:"destination_address"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		httperr.Unauthenticated(w, "no autenticado")
		return
	}
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}
	o, err := h.uc.Create(r.Context(), p, usecase.CreateOrderInput{
		ExternalRef:        req.ExternalRef,
		StoreID:            req.StoreID,
		CustomerID:         req.CustomerID,
		Origin:             domain.Point{Lat: req.Origin.Lat, Lon: req.Origin.Lon},
		Destination:        domain.Point{Lat: req.Destination.Lat, Lon: req.Destination.Lon},
		DestinationAddress: req.DestinationAddress,
	})
	if err != nil {
		mapOrderError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/orders/"+o.ID)
	writeJSON(w, http.StatusCreated, toOrderResponse(o))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		httperr.Unauthenticated(w, "no autenticado")
		return
	}
	q := r.URL.Query()
	var status *domain.Status
	if s := strings.TrimSpace(q.Get("status")); s != "" {
		st := domain.Status(strings.ToUpper(s))
		if !domain.ValidStatus(st) {
			httperr.Validation(w, "INVALID_STATUS", "estado desconocido")
			return
		}
		status = &st
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	orders, total, err := h.uc.List(r.Context(), p, status, limit, offset)
	if err != nil {
		mapOrderError(w, err)
		return
	}
	items := make([]orderResponse, 0, len(orders))
	for i := range orders {
		items = append(items, toOrderResponse(&orders[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		httperr.Unauthenticated(w, "no autenticado")
		return
	}
	o, err := h.uc.Get(r.Context(), p, r.PathValue("id"))
	if err != nil {
		mapOrderError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(o))
}

type statusRequest struct {
	Status string `json:"status"`
}

func (h *Handler) ChangeStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		httperr.Unauthenticated(w, "no autenticado")
		return
	}
	var req statusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}
	o, err := h.uc.ChangeStatus(r.Context(), p, r.PathValue("id"), domain.Status(strings.ToUpper(req.Status)))
	if err != nil {
		mapOrderError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(o))
}

type assignRequest struct {
	VehicleID string `json:"vehicle_id"`
	DriverID  string `json:"driver_id"`
}

func (h *Handler) Assign(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		httperr.Unauthenticated(w, "no autenticado")
		return
	}
	var req assignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}
	o, err := h.uc.Assign(r.Context(), p, r.PathValue("id"), req.VehicleID, req.DriverID)
	if err != nil {
		mapOrderError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(o))
}

// orderResponse define el contrato público (ADR-10).
type orderResponse struct {
	ID                 string   `json:"id"`
	ExternalRef        string   `json:"external_ref"`
	StoreID            string   `json:"store_id"`
	CustomerID         *string  `json:"customer_id,omitempty"`
	Status             string   `json:"status"`
	AssignedVehicleID  *string  `json:"assigned_vehicle_id,omitempty"`
	AssignedDriverID   *string  `json:"assigned_driver_id,omitempty"`
	Origin             pointDTO `json:"origin"`
	Destination        pointDTO `json:"destination"`
	DestinationAddress string   `json:"destination_address"`
	DistanceMeters     *float64 `json:"distance_meters,omitempty"`
	PlacedAt           string   `json:"placed_at"`
	PickedUpAt         *string  `json:"picked_up_at,omitempty"`
	DeliveredAt        *string  `json:"delivered_at,omitempty"`
}

func toOrderResponse(o *domain.Order) orderResponse {
	r := orderResponse{
		ID: o.ID, ExternalRef: o.ExternalRef, StoreID: o.StoreID, CustomerID: o.CustomerID,
		Status: string(o.Status), AssignedVehicleID: o.AssignedVehicleID, AssignedDriverID: o.AssignedDriverID,
		Origin:             pointDTO{Lat: o.Origin.Lat, Lon: o.Origin.Lon},
		Destination:        pointDTO{Lat: o.Destination.Lat, Lon: o.Destination.Lon},
		DestinationAddress: o.DestinationAddress, DistanceMeters: o.DistanceMeters,
		PlacedAt: o.PlacedAt.UTC().Format(time.RFC3339),
	}
	if o.PickedUpAt != nil {
		s := o.PickedUpAt.UTC().Format(time.RFC3339)
		r.PickedUpAt = &s
	}
	if o.DeliveredAt != nil {
		s := o.DeliveredAt.UTC().Format(time.RFC3339)
		r.DeliveredAt = &s
	}
	return r
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func mapOrderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httperr.NotFound(w, "orden no encontrada")
	case errors.Is(err, domain.ErrForbiddenScope):
		httperr.Forbidden(w, "sin acceso a la orden")
	case errors.Is(err, domain.ErrInvalidTransition):
		httperr.Conflict(w, "ORDER_INVALID_TRANSITION", "transición de estado no permitida")
	case errors.Is(err, domain.ErrExternalRefConflict):
		httperr.Conflict(w, "DUPLICATE_EXTERNAL_REF", "external_ref ya registrado")
	case errors.Is(err, domain.ErrValidation):
		httperr.Validation(w, "VALIDATION_ERROR", err.Error())
	default:
		httperr.Internal(w)
	}
}
