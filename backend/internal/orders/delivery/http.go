// Package delivery: handlers HTTP del módulo órdenes. Sin lógica de negocio.
package delivery

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tiendas-track/backend/internal/orders/domain"
	"tiendas-track/backend/internal/orders/usecase"
	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/middleware"
	"tiendas-track/backend/internal/tracing"
)

// Handler expone los endpoints de órdenes.
type Handler struct {
	uc  *usecase.OrdersUseCase
	log *slog.Logger
}

// NewHandler crea el handler.
func NewHandler(uc *usecase.OrdersUseCase, log *slog.Logger) *Handler {
	return &Handler{uc: uc, log: log}
}

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
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		h.log.Error("Create: no autenticado", "trace_id", traceID)
		httperr.Unauthenticated(w, "no autenticado")
		return
	}

	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("Create: JSON inválido", "trace_id", traceID, "error", err.Error())
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}

	h.log.Info("Create: procesando solicitud",
		"trace_id", traceID,
		"external_ref", req.ExternalRef,
		"store_id", req.StoreID,
	)

	o, err := h.uc.Create(r.Context(), p, usecase.CreateOrderInput{
		ExternalRef:        req.ExternalRef,
		StoreID:            req.StoreID,
		CustomerID:         req.CustomerID,
		Origin:             domain.Point{Lat: req.Origin.Lat, Lon: req.Origin.Lon},
		Destination:        domain.Point{Lat: req.Destination.Lat, Lon: req.Destination.Lon},
		DestinationAddress: req.DestinationAddress,
	})
	if err != nil {
		h.log.Error("Create: error en caso de uso",
			"trace_id", traceID,
			"error", err.Error(),
		)
		mapOrderError(w, err)
		return
	}

	h.log.Info("Create: orden creada exitosamente",
		"trace_id", traceID,
		"order_id", o.ID,
	)

	w.Header().Set("Location", "/api/v1/orders/"+o.ID)
	writeJSON(w, http.StatusCreated, toOrderResponse(o))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		h.log.Error("List: no autenticado", "trace_id", traceID)
		httperr.Unauthenticated(w, "no autenticado")
		return
	}
	q := r.URL.Query()
	var status *domain.Status
	if s := strings.TrimSpace(q.Get("status")); s != "" {
		st := domain.Status(strings.ToUpper(s))
		if !domain.ValidStatus(st) {
			h.log.Error("List: estado inválido", "trace_id", traceID, "status", s)
			httperr.Validation(w, "INVALID_STATUS", "estado desconocido")
			return
		}
		status = &st
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	h.log.Info("List: procesando solicitud",
		"trace_id", traceID,
		"limit", limit,
		"offset", offset,
	)

	orders, total, err := h.uc.List(r.Context(), p, status, limit, offset)
	if err != nil {
		h.log.Error("List: error en caso de uso",
			"trace_id", traceID,
			"error", err.Error(),
		)
		mapOrderError(w, err)
		return
	}
	items := make([]orderResponse, 0, len(orders))
	for i := range orders {
		items = append(items, toOrderResponse(&orders[i]))
	}

	h.log.Info("List: consulta completada",
		"trace_id", traceID,
		"total", total,
		"count", len(orders),
	)

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		h.log.Error("Get: no autenticado", "trace_id", traceID)
		httperr.Unauthenticated(w, "no autenticado")
		return
	}

	orderID := r.PathValue("id")

	h.log.Info("Get: obteniendo orden",
		"trace_id", traceID,
		"order_id", orderID,
	)

	o, err := h.uc.Get(r.Context(), p, orderID)
	if err != nil {
		h.log.Error("Get: error en caso de uso",
			"trace_id", traceID,
			"order_id", orderID,
			"error", err.Error(),
		)
		mapOrderError(w, err)
		return
	}

	h.log.Info("Get: orden obtenida exitosamente",
		"trace_id", traceID,
		"order_id", o.ID,
	)

	writeJSON(w, http.StatusOK, toOrderResponse(o))
}

type statusRequest struct {
	Status string `json:"status"`
}

func (h *Handler) ChangeStatus(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		h.log.Error("ChangeStatus: no autenticado", "trace_id", traceID)
		httperr.Unauthenticated(w, "no autenticado")
		return
	}

	orderID := r.PathValue("id")

	var req statusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("ChangeStatus: JSON inválido", "trace_id", traceID, "error", err.Error())
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}

	h.log.Info("ChangeStatus: cambiando estado",
		"trace_id", traceID,
		"order_id", orderID,
		"new_status", req.Status,
	)

	o, err := h.uc.ChangeStatus(r.Context(), p, orderID, domain.Status(strings.ToUpper(req.Status)))
	if err != nil {
		h.log.Error("ChangeStatus: error en caso de uso",
			"trace_id", traceID,
			"order_id", orderID,
			"error", err.Error(),
		)
		mapOrderError(w, err)
		return
	}

	h.log.Info("ChangeStatus: estado actualizado exitosamente",
		"trace_id", traceID,
		"order_id", o.ID,
		"new_status", o.Status,
	)

	writeJSON(w, http.StatusOK, toOrderResponse(o))
}

type assignRequest struct {
	VehicleID string `json:"vehicle_id"`
	DriverID  string `json:"driver_id"`
}

func (h *Handler) Assign(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		h.log.Error("Assign: no autenticado", "trace_id", traceID)
		httperr.Unauthenticated(w, "no autenticado")
		return
	}

	orderID := r.PathValue("id")

	var req assignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("Assign: JSON inválido", "trace_id", traceID, "error", err.Error())
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}

	h.log.Info("Assign: procesando asignación",
		"trace_id", traceID,
		"order_id", orderID,
		"vehicle_id", req.VehicleID,
		"driver_id", req.DriverID,
	)

	o, err := h.uc.Assign(r.Context(), p, orderID, req.VehicleID, req.DriverID)
	if err != nil {
		h.log.Error("Assign: error en caso de uso",
			"trace_id", traceID,
			"order_id", orderID,
			"error", err.Error(),
		)
		mapOrderError(w, err)
		return
	}

	h.log.Info("Assign: orden asignada exitosamente",
		"trace_id", traceID,
		"order_id", o.ID,
		"vehicle_id", req.VehicleID,
		"driver_id", req.DriverID,
	)

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
