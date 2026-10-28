// Package usecase: consulta de flota con aislamiento por rol (RF-02).
// El alcance se resuelve SIEMPRE server-side a partir del principal del token.
package usecase

import (
	"context"
	"log/slog"

	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/resilience"
	"tiendas-track/backend/internal/telemetry/domain"
)

const (
	// DefaultVehicleLimit y MaxVehicleLimit acotan la página de flota.
	DefaultVehicleLimit = 100
	MaxVehicleLimit     = 200
)

// FleetUseCase lista los vehículos visibles para un rol.
type FleetUseCase struct {
	fleet domain.FleetRepository
	reads *resilience.Breaker
	log   *slog.Logger
}

// NewFleet crea el caso de uso. reads protege la lectura de BD (RNF-05).
func NewFleet(fleet domain.FleetRepository, reads *resilience.Breaker, log *slog.Logger) *FleetUseCase {
	return &FleetUseCase{fleet: fleet, reads: reads, log: log}
}

// List devuelve la página de vehículos del alcance del principal.
// Un limit/offset inválido es un error de validación (422 en el borde HTTP).
func (uc *FleetUseCase) List(ctx context.Context, p authz.Principal, limit, offset int) ([]domain.Vehicle, int, error) {
	if p.IsGuest() {
		return nil, 0, domain.ErrForbiddenScope
	}
	if limit == 0 {
		limit = DefaultVehicleLimit
	}
	if limit < 0 || limit > MaxVehicleLimit || offset < 0 {
		return nil, 0, domain.ErrValidation
	}

	scope, ok := fleetScopeFor(p)
	if !ok {
		return nil, 0, domain.ErrForbiddenScope
	}

	page, err := resilience.DoCtx(ctx, uc.reads, func(ctx context.Context) (fleetPage, error) {
		items, total, err := uc.fleet.ListVehicles(ctx, scope, limit, offset)
		return fleetPage{items: items, total: total}, err
	})
	if err != nil {
		uc.log.Error("ListVehicles: error leyendo flota",
			"role", p.Role, "error", err.Error())
		return nil, 0, err
	}
	return page.items, page.total, nil
}

// fleetPage es el resultado del puerto a través del circuit breaker.
type fleetPage struct {
	items []domain.Vehicle
	total int
}

// fleetScopeFor traduce el rol a su columna de filtrado. Un rol sin identidad
// de scope (p. ej. STORE sin tienda) devuelve un alcance vacío, no un error:
// la respuesta correcta es una lista vacía.
func fleetScopeFor(p authz.Principal) (domain.FleetScope, bool) {
	switch {
	case p.HasRole(authz.RoleProvider):
		return domain.FleetScope{ProviderID: providerID(p)}, true
	case p.HasRole(authz.RoleStore):
		return domain.FleetScope{StoreID: p.StoreID}, true
	case p.HasRole(authz.RoleDriver):
		return domain.FleetScope{DriverID: p.DriverID}, true
	default:
		return domain.FleetScope{}, false
	}
}
