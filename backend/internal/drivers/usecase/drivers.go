// Package usecase: lista de conductores de la flota del proveedor (RF-06).
package usecase

import (
	"context"
	"log/slog"
	"strings"

	"tiendas-track/backend/internal/drivers/domain"
	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/tracing"
)

// DriversUseCase expone la consulta de conductores de un vehículo.
type DriversUseCase struct {
	repo domain.DriverRepository
	log  *slog.Logger
}

// New construye el use case.
func New(repo domain.DriverRepository, log *slog.Logger) *DriversUseCase {
	return &DriversUseCase{repo: repo, log: log}
}

// ListByVehicle devuelve los conductores asignados a un vehículo.
// Solo el PROVIDER dueño del vehículo puede consultar esta lista.
func (uc *DriversUseCase) ListByVehicle(ctx context.Context, p authz.Principal, vehicleID string) ([]domain.Driver, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("ListByVehicle: iniciando consulta",
		"trace_id", traceID,
		"vehicle_id", vehicleID,
		"user_id", p.UserID,
	)

	// Solo el rol PROVIDER puede consultar conductores de su flota.
	if p.IsGuest() || !p.HasRole(authz.RoleProvider) {
		uc.log.Error("ListByVehicle: acceso denegado",
			"trace_id", traceID,
			"role", p.Role,
		)
		return nil, domain.ErrForbiddenScope
	}

	if strings.TrimSpace(vehicleID) == "" {
		return nil, domain.ErrForbiddenScope
	}

	// Validar que el vehículo pertenece al proveedor autenticado (RBAC de datos).
	ok, err := uc.repo.VehicleBelongsToProvider(ctx, vehicleID, p.ProviderID)
	if err != nil {
		uc.log.Error("ListByVehicle: error verificando vehículo",
			"trace_id", traceID,
			"vehicle_id", vehicleID,
			"error", err.Error(),
		)
		return nil, err
	}
	if !ok {
		uc.log.Error("ListByVehicle: vehículo fuera del scope",
			"trace_id", traceID,
			"vehicle_id", vehicleID,
			"provider_id", p.ProviderID,
		)
		// No revelar si el vehículo existe: mismo error de scope.
		return nil, domain.ErrForbiddenScope
	}

	drivers, err := uc.repo.ListByVehicle(ctx, vehicleID)
	if err != nil {
		uc.log.Error("ListByVehicle: error consultando conductores",
			"trace_id", traceID,
			"vehicle_id", vehicleID,
			"error", err.Error(),
		)
		return nil, err
	}

	uc.log.Info("ListByVehicle: consulta completada",
		"trace_id", traceID,
		"vehicle_id", vehicleID,
		"count", len(drivers),
	)
	return drivers, nil
}
