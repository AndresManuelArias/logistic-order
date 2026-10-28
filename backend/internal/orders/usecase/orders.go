// Package usecase: casos de uso de órdenes (RF-05, RF-06) con RBAC de datos.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"tiendas-track/backend/internal/orders/domain"
	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/tracing"
)

// OrdersUseCase orquesta el ciclo de vida de las órdenes.
type OrdersUseCase struct {
	orders domain.OrderRepository
	log    *slog.Logger
	now    func() time.Time
}

// New construye el caso de uso.
func New(orders domain.OrderRepository, log *slog.Logger) *OrdersUseCase {
	return &OrdersUseCase{orders: orders, log: log, now: time.Now}
}

// CreateOrderInput es la entrada de creación.
type CreateOrderInput struct {
	ExternalRef        string
	StoreID            string
	CustomerID         *string
	Origin             domain.Point
	Destination        domain.Point
	DestinationAddress string
}

// Create registra una orden nueva (rol STORE, solo su propia tienda).
func (uc *OrdersUseCase) Create(ctx context.Context, p authz.Principal, in CreateOrderInput) (*domain.Order, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("Create: iniciando creación de orden",
		"trace_id", traceID,
		"external_ref", in.ExternalRef,
		"store_id", in.StoreID,
		"user_id", p.UserID,
	)

	if p.IsGuest() || !p.HasRole(authz.RoleStore) {
		uc.log.Error("Create: error de autorización",
			"trace_id", traceID,
			"error", domain.ErrForbiddenScope.Error(),
		)
		return nil, domain.ErrForbiddenScope
	}
	if strings.TrimSpace(in.ExternalRef) == "" {
		uc.log.Error("Create: validación fallida",
			"trace_id", traceID,
			"error", "external_ref requerido",
		)
		return nil, fmt.Errorf("%w: external_ref requerido", domain.ErrValidation)
	}
	if !in.Origin.Valid() || !in.Destination.Valid() {
		uc.log.Error("Create: validación fallida",
			"trace_id", traceID,
			"error", "coordenadas inválidas",
		)
		return nil, fmt.Errorf("%w: coordenadas inválidas", domain.ErrValidation)
	}
	// Aislamiento de datos: la orden pertenece a la tienda del principal (RF-02).
	if in.StoreID == "" {
		in.StoreID = p.StoreID
	}
	// Un principal sin tienda asociada (registro incompleto/ dato corrupto)
	// no puede operar: se rechaza como scope en lugar de fallar en SQL.
	if p.StoreID == "" || in.StoreID != p.StoreID {
		uc.log.Error("Create: error de alcance",
			"trace_id", traceID,
			"error", "store_id no coincide con el principal",
		)
		return nil, domain.ErrForbiddenScope
	}

	exists, err := uc.orders.StoreExists(ctx, in.StoreID)
	if err != nil {
		uc.log.Error("Create: error verificando tienda",
			"trace_id", traceID,
			"error", err.Error(),
		)
		return nil, err
	}
	if !exists {
		uc.log.Error("Create: tienda inexistente",
			"trace_id", traceID,
			"store_id", in.StoreID,
		)
		return nil, fmt.Errorf("%w: tienda inexistente", domain.ErrValidation)
	}

	o := &domain.Order{
		ExternalRef:        strings.TrimSpace(in.ExternalRef),
		StoreID:            in.StoreID,
		CustomerID:         in.CustomerID,
		Status:             domain.StatusCreated,
		Origin:             in.Origin,
		Destination:        in.Destination,
		DestinationAddress: strings.TrimSpace(in.DestinationAddress),
		PlacedAt:           uc.now(),
	}
	if err := uc.orders.Create(ctx, o); err != nil {
		if errors.Is(err, domain.ErrExternalRefConflict) {
			uc.log.Error("Create: conflicto de external_ref",
				"trace_id", traceID,
				"external_ref", in.ExternalRef,
			)
		} else {
			uc.log.Error("Create: error al crear orden",
				"trace_id", traceID,
				"error", err.Error(),
			)
		}
		return nil, err
	}

	uc.log.Info("Create: orden creada exitosamente",
		"trace_id", traceID,
		"order_id", o.ID,
		"store_id", o.StoreID,
	)
	return o, nil
}

// Get devuelve una orden si el principal tiene acceso (RF-02, RF-10 invitado).
func (uc *OrdersUseCase) Get(ctx context.Context, p authz.Principal, id string) (*domain.Order, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("Get: iniciando consulta de orden",
		"trace_id", traceID,
		"order_id", id,
		"user_id", p.UserID,
	)

	o, err := uc.orders.FindByID(ctx, id)
	if err != nil {
		uc.log.Error("Get: error obteniendo orden",
			"trace_id", traceID,
			"order_id", id,
			"error", err.Error(),
		)
		return nil, err
	}
	if !uc.canAccess(ctx, p, o) {
		uc.log.Error("Get: acceso denegado",
			"trace_id", traceID,
			"order_id", id,
			"principal_user", p.UserID,
		)
		return nil, domain.ErrForbiddenScope
	}

	uc.log.Info("Get: orden obtenida exitosamente",
		"trace_id", traceID,
		"order_id", o.ID,
		"status", o.Status,
	)
	return o, nil
}

// List devuelve las órdenes visibles para el rol (scope de servidor).
func (uc *OrdersUseCase) List(ctx context.Context, p authz.Principal, status *domain.Status, limit, offset int) ([]domain.Order, int, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("List: iniciando consulta de órdenes",
		"trace_id", traceID,
		"user_id", p.UserID,
		"role", p.Role,
		"limit", limit,
		"offset", offset,
	)

	f := domain.ListFilter{Limit: limit, Offset: offset, Status: status}
	switch {
	case p.IsGuest():
		uc.log.Error("List: acceso denegado para invitado",
			"trace_id", traceID,
		)
		return nil, 0, domain.ErrForbiddenScope
	case p.HasRole(authz.RoleStore):
		// Sin tienda no hay scope: listar todo rompería el aislamiento.
		if p.StoreID == "" {
			uc.log.Error("List: STORE sin tienda, scope vacío",
				"trace_id", traceID,
				"user_id", p.UserID,
			)
			return nil, 0, domain.ErrForbiddenScope
		}
		f.StoreID = p.StoreID
	case p.HasRole(authz.RoleProvider):
		if p.ProviderID == "" {
			uc.log.Error("List: PROVIDER sin identidad de scope",
				"trace_id", traceID,
				"user_id", p.UserID,
			)
			return nil, 0, domain.ErrForbiddenScope
		}
		f.ProviderID = p.ProviderID
	case p.HasRole(authz.RoleDriver):
		// Sin driver no hay scope: vería órdenes ajenas.
		if p.DriverID == "" {
			uc.log.Error("List: DRIVER sin identidad de scope",
				"trace_id", traceID,
				"user_id", p.UserID,
			)
			return nil, 0, domain.ErrForbiddenScope
		}
		f.DriverID = p.DriverID
	default:
		uc.log.Error("List: rol no autorizado",
			"trace_id", traceID,
			"role", p.Role,
		)
		return nil, 0, domain.ErrForbiddenScope
	}

	orders, total, err := uc.orders.List(ctx, f)
	if err != nil {
		uc.log.Error("List: error listando órdenes",
			"trace_id", traceID,
			"error", err.Error(),
		)
		return nil, 0, err
	}

	uc.log.Info("List: consulta completada",
		"trace_id", traceID,
		"total", total,
		"count", len(orders),
	)
	return orders, total, nil
}

// ChangeStatus aplica una transición legal del ciclo de vida (RF-05).
func (uc *OrdersUseCase) ChangeStatus(ctx context.Context, p authz.Principal, id string, to domain.Status) (*domain.Order, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("ChangeStatus: iniciando cambio de estado",
		"trace_id", traceID,
		"order_id", id,
		"target_status", to,
		"user_id", p.UserID,
	)

	if p.IsGuest() {
		uc.log.Error("ChangeStatus: invitado no puede cambiar estado",
			"trace_id", traceID,
		)
		return nil, domain.ErrForbiddenScope
	}
	o, err := uc.orders.FindByID(ctx, id)
	if err != nil {
		uc.log.Error("ChangeStatus: error obteniendo orden",
			"trace_id", traceID,
			"order_id", id,
			"error", err.Error(),
		)
		return nil, err
	}
	if !uc.canChange(ctx, p, o) {
		uc.log.Error("ChangeStatus: permiso denegado",
			"trace_id", traceID,
			"order_id", id,
			"user_id", p.UserID,
		)
		return nil, domain.ErrForbiddenScope
	}
	if !domain.ValidStatus(to) {
		uc.log.Error("ChangeStatus: estado inválido",
			"trace_id", traceID,
			"status", to,
		)
		return nil, fmt.Errorf("%w: estado desconocido", domain.ErrValidation)
	}

	at := uc.now()
	if err := o.Transition(to, at); err != nil {
		uc.log.Error("ChangeStatus: transición fallida",
			"trace_id", traceID,
			"order_id", id,
			"error", err.Error(),
		)
		return nil, err
	}
	actor := p.UserID
	event := domain.OrderEvent{
		OrderID: o.ID, Status: to, ActorUserID: &actor, OccurredAt: at,
		Payload: map[string]any{"from": string(o.Status)},
	}
	if err := uc.orders.UpdateStatus(ctx, o, event); err != nil {
		uc.log.Error("ChangeStatus: error actualizando estado",
			"trace_id", traceID,
			"order_id", id,
			"error", err.Error(),
		)
		return nil, err
	}

	uc.log.Info("ChangeStatus: estado actualizado exitosamente",
		"trace_id", traceID,
		"order_id", o.ID,
		"new_status", to,
	)
	return o, nil
}

// Assign vincula vehículo y conductor a la orden (RF-06, rol PROVIDER).
func (uc *OrdersUseCase) Assign(ctx context.Context, p authz.Principal, id, vehicleID, driverID string) (*domain.Order, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("Assign: iniciando asignación de orden",
		"trace_id", traceID,
		"order_id", id,
		"vehicle_id", vehicleID,
		"driver_id", driverID,
		"user_id", p.UserID,
	)

	if p.IsGuest() || !p.HasRole(authz.RoleProvider) {
		uc.log.Error("Assign: error de autorización",
			"trace_id", traceID,
			"error", domain.ErrForbiddenScope.Error(),
		)
		return nil, domain.ErrForbiddenScope
	}
	if strings.TrimSpace(vehicleID) == "" || strings.TrimSpace(driverID) == "" {
		uc.log.Error("Assign: validación fallida",
			"trace_id", traceID,
			"error", "vehicle_id y driver_id requeridos",
		)
		return nil, fmt.Errorf("%w: vehicle_id y driver_id requeridos", domain.ErrValidation)
	}
	o, err := uc.orders.FindByID(ctx, id)
	if err != nil {
		uc.log.Error("Assign: error obteniendo orden",
			"trace_id", traceID,
			"order_id", id,
			"error", err.Error(),
		)
		return nil, err
	}
	ok, err := uc.orders.VehicleBelongsToProvider(ctx, vehicleID, p.ProviderID)
	if err != nil {
		uc.log.Error("Assign: error verificando vehiculo",
			"trace_id", traceID,
			"vehicle_id", vehicleID,
			"error", err.Error(),
		)
		return nil, err
	}
	if !ok {
		// No revelar si el vehículo existe: mismo error de scope.
		uc.log.Error("Assign: vehiculo no pertenece al provider",
			"trace_id", traceID,
			"vehicle_id", vehicleID,
			"provider_id", p.ProviderID,
		)
		return nil, domain.ErrForbiddenScope
	}
	ok, err = uc.orders.DriverBelongsToVehicle(ctx, driverID, vehicleID)
	if err != nil {
		uc.log.Error("Assign: error verificando conductor",
			"trace_id", traceID,
			"driver_id", driverID,
			"vehicle_id", vehicleID,
			"error", err.Error(),
		)
		return nil, err
	}
	if !ok {
		uc.log.Error("Assign: conductor no asignado a ese vehículo",
			"trace_id", traceID,
			"driver_id", driverID,
			"vehicle_id", vehicleID,
		)
		return nil, fmt.Errorf("%w: conductor no asignado a ese vehículo", domain.ErrValidation)
	}

	actor := p.UserID
	event := domain.OrderEvent{
		OrderID: o.ID, Status: o.Status, ActorUserID: &actor, OccurredAt: uc.now(),
		Payload: map[string]any{"vehicle_id": vehicleID, "driver_id": driverID},
	}
	if err := uc.orders.Assign(ctx, o, vehicleID, driverID, event); err != nil {
		uc.log.Error("Assign: error asignando orden",
			"trace_id", traceID,
			"order_id", id,
			"error", err.Error(),
		)
		return nil, err
	}

	uc.log.Info("Assign: orden asignada exitosamente",
		"trace_id", traceID,
		"order_id", o.ID,
		"vehicle_id", vehicleID,
		"driver_id", driverID,
	)
	return o, nil
}

// IsAccessibleBy decide si el principal puede ver la orden (para tokens de invitado).
func (uc *OrdersUseCase) IsAccessibleBy(ctx context.Context, orderID string, p authz.Principal) (bool, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("IsAccessibleBy: verificando acceso a orden",
		"trace_id", traceID,
		"order_id", orderID,
		"user_id", p.UserID,
	)

	o, err := uc.orders.FindByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			uc.log.Info("IsAccessibleBy: orden no encontrada",
				"trace_id", traceID,
				"order_id", orderID,
			)
			return false, nil
		}
		uc.log.Error("IsAccessibleBy: error obteniendo orden",
			"trace_id", traceID,
			"order_id", orderID,
			"error", err.Error(),
		)
		return false, err
	}

	result := uc.canAccess(ctx, p, o)
	uc.log.Info("IsAccessibleBy: acceso evaluado",
		"trace_id", traceID,
		"order_id", orderID,
		"accessible", result,
	)
	return result, nil
}

// canAccess aplica el aislamiento de datos por rol (RF-02).
func (uc *OrdersUseCase) canAccess(ctx context.Context, p authz.Principal, o *domain.Order) bool {
	switch {
	case p.IsGuest():
		// El invitado solo ve la orden de su scope (order:<id>).
		guestOrder, ok := p.GuestOrderID()
		return ok && guestOrder == o.ID
	case p.HasRole(authz.RoleStore):
		return o.StoreID == p.StoreID
	case p.HasRole(authz.RoleDriver):
		return o.AssignedDriverID != nil && *o.AssignedDriverID == p.DriverID
	case p.HasRole(authz.RoleProvider):
		if o.AssignedVehicleID == nil {
			return false
		}
		ok, _ := uc.orders.VehicleBelongsToProvider(ctx, *o.AssignedVehicleID, p.ProviderID)
		return ok
	}
	return false
}

// canChange decide quién puede transicionar: conductor asignado o la tienda dueña.
func (uc *OrdersUseCase) canChange(ctx context.Context, p authz.Principal, o *domain.Order) bool {
	switch {
	case p.HasRole(authz.RoleStore):
		return o.StoreID == p.StoreID
	case p.HasRole(authz.RoleDriver):
		return o.AssignedDriverID != nil && *o.AssignedDriverID == p.DriverID
	}
	return false
}
