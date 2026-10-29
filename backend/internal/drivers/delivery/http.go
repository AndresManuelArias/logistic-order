// Package delivery: handler HTTP del módulo conductores. Sin lógica de negocio.
package delivery

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"tiendas-track/backend/internal/drivers/domain"
	"tiendas-track/backend/internal/drivers/usecase"
	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/middleware"
	"tiendas-track/backend/internal/tracing"
)

// Handler expone los endpoints de conductores.
type Handler struct {
	uc  *usecase.DriversUseCase
	log *slog.Logger
}

// NewHandler crea el handler.
func NewHandler(uc *usecase.DriversUseCase, log *slog.Logger) *Handler {
	return &Handler{uc: uc, log: log}
}

// RegisterRoutes monta /api/v1/drivers.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler, roles func(...string) func(http.Handler) http.Handler) {
	// GET /api/v1/drivers?vehicle_id=<uuid> — solo PROVIDER puede consultar
	mux.Handle("GET /api/v1/drivers", auth(roles("PROVIDER")(http.HandlerFunc(h.ListByVehicle))))
}

// driverResponse es el DTO público de conductor.
type driverResponse struct {
	ID            string `json:"id"`
	UserID        string `json:"user_id"`
	FullName      string `json:"full_name"`
	LicenseNumber string `json:"license_number"`
	VehicleID     string `json:"vehicle_id"`
}

func toDriverResponse(d domain.Driver) driverResponse {
	return driverResponse{
		ID:            d.ID,
		UserID:        d.UserID,
		FullName:      d.FullName,
		LicenseNumber: d.LicenseNumber,
		VehicleID:     d.VehicleID,
	}
}

// ListByVehicle maneja GET /api/v1/drivers?vehicle_id=<uuid>.
// Devuelve los conductores asignados al vehículo indicado, validando que
// el vehículo pertenece al proveedor autenticado.
func (h *Handler) ListByVehicle(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		h.log.Error("ListByVehicle: no autenticado", "trace_id", traceID)
		httperr.Unauthenticated(w, "no autenticado")
		return
	}

	vehicleID := r.URL.Query().Get("vehicle_id")
	if vehicleID == "" {
		h.log.Error("ListByVehicle: vehicle_id requerido", "trace_id", traceID)
		httperr.BadRequest(w, "VEHICLE_ID_REQUIRED", "el parámetro vehicle_id es requerido")
		return
	}

	h.log.Info("ListByVehicle: consultando conductores",
		"trace_id", traceID,
		"vehicle_id", vehicleID,
	)

	drivers, err := h.uc.ListByVehicle(r.Context(), p, vehicleID)
	if err != nil {
		h.log.Error("ListByVehicle: error en caso de uso",
			"trace_id", traceID,
			"vehicle_id", vehicleID,
			"error", err.Error(),
		)
		switch {
		case errors.Is(err, domain.ErrForbiddenScope):
			httperr.Forbidden(w, "acceso denegado: el vehículo está fuera de tu flota")
		case errors.Is(err, domain.ErrNotFound):
			httperr.NotFound(w, "vehículo no encontrado")
		default:
			httperr.Internal(w)
		}
		return
	}

	items := make([]driverResponse, 0, len(drivers))
	for _, d := range drivers {
		items = append(items, toDriverResponse(d))
	}

	h.log.Info("ListByVehicle: respondiendo",
		"trace_id", traceID,
		"vehicle_id", vehicleID,
		"count", len(items),
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
