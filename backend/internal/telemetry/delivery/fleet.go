package delivery

import (
	"net/http"
	"strconv"

	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/middleware"
	"tiendas-track/backend/internal/tracing"
)

// vehicleDTO es la proyección de un vehículo de flota. No expone provider_id
// ni datos internos: solo lo que el portal necesita para pintar el mapa.
type vehicleDTO struct {
	ID           string    `json:"id"`
	Plate        string    `json:"plate"`
	Status       string    `json:"status"`
	LastPosition *pointDTO `json:"last_position"`
}

type vehicleListResponse struct {
	Items []vehicleDTO `json:"items"`
	Total int          `json:"total"`
}

// ListVehicles devuelve la flota visible para el rol autenticado (RF-02).
// El alcance (proveedor, tienda o conductor) lo resuelve el caso de uso.
func (h *Handler) ListVehicles(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())

	limit, offset := 0, 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httperr.Validation(w, "INVALID_PAGINATION", "limit debe ser un entero")
			return
		}
		limit = n
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httperr.Validation(w, "INVALID_PAGINATION", "offset debe ser un entero")
			return
		}
		offset = n
	}

	p, _ := middleware.PrincipalFrom(r.Context())
	vehicles, total, err := h.fleet.List(r.Context(), p, limit, offset)
	if err != nil {
		h.mapError(w, traceID, "ListVehicles", err)
		return
	}

	out := make([]vehicleDTO, 0, len(vehicles))
	for i := range vehicles {
		v := &vehicles[i]
		dto := vehicleDTO{ID: v.ID, Plate: v.Plate, Status: v.Status}
		if v.Last != nil {
			pt := toPointDTO(v.Last)
			dto.LastPosition = &pt
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, vehicleListResponse{Items: out, Total: total})
}
