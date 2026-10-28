// Package usecase: casos de uso de órdenes (RF-05, RF-06) con RBAC de datos.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"tiendas-track/backend/internal/orders/domain"
	"tiendas-track/backend/internal/shared/authz"
)

// OrdersUseCase orquesta el ciclo de vida de las órdenes.
type OrdersUseCase struct {
	orders domain.OrderRepository
	now    func() time.Time
}

// New construye el caso de uso.
func New(orders domain.OrderRepository) *OrdersUseCase {
	return &OrdersUseCase{orders: orders, now: time.Now}
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
	if p.IsGuest() || !p.HasRole(authz.RoleStore) {
		return nil, domain.ErrForbiddenScope
	}
	if strings.TrimSpace(in.ExternalRef) == "" {
		return nil, fmt.Errorf("%w: external_ref requerido", domain.ErrValidation)
	}
	if !in.Origin.Valid() || !in.Destination.Valid() {
		return nil, fmt.Errorf("%w: coordenadas inválidas", domain.ErrValidation)
	}
	// Aislamiento de datos: la orden pertenece a la tienda del principal (RF-02).
	if in.StoreID == "" {
		in.StoreID = p.StoreID
	}
	// Un principal sin tienda asociada (registro incompleto/ dato corrupto)
	// no puede operar: se rechaza como scope en lugar de fallar en SQL.
	if p.StoreID == "" || in.StoreID != p.StoreID {
		return nil, domain.ErrForbiddenScope
	}

	exists, err := uc.orders.StoreExists(ctx, in.StoreID)
	if err != nil {
		return nil, err
	}
	if !exists {
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
			return nil, err
		}
		return nil, err
	}
	return o, nil
}

// Get devuelve una orden si el principal tiene acceso (RF-02, RF-10 invitado).
func (uc *OrdersUseCase) Get(ctx context.Context, p authz.Principal, id string) (*domain.Order, error) {
	o, err := uc.orders.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !uc.canAccess(ctx, p, o) {
		return nil, domain.ErrForbiddenScope
	}
	return o, nil
}

// List devuelve las órdenes visibles para el rol (scope de servidor).
func (uc *OrdersUseCase) List(ctx context.Context, p authz.Principal, status *domain.Status, limit, offset int) ([]domain.Order, int, error) {
	f := domain.ListFilter{Limit: limit, Offset: offset, Status: status}
	switch {
	case p.IsGuest():
		return nil, 0, domain.ErrForbiddenScope
	case p.HasRole(authz.RoleStore):
		f.StoreID = p.StoreID
	case p.HasRole(authz.RoleProvider):
		f.ProviderID = p.ProviderID
	case p.HasRole(authz.RoleDriver):
		f.DriverID = p.DriverID
	default:
		return nil, 0, domain.ErrForbiddenScope
	}
	return uc.orders.List(ctx, f)
}

// ChangeStatus aplica una transición legal del ciclo de vida (RF-05).
func (uc *OrdersUseCase) ChangeStatus(ctx context.Context, p authz.Principal, id string, to domain.Status) (*domain.Order, error) {
	if p.IsGuest() {
		return nil, domain.ErrForbiddenScope
	}
	o, err := uc.orders.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !uc.canChange(ctx, p, o) {
		return nil, domain.ErrForbiddenScope
	}
	if !domain.ValidStatus(to) {
		return nil, fmt.Errorf("%w: estado desconocido", domain.ErrValidation)
	}

	at := uc.now()
	if err := o.Transition(to, at); err != nil {
		return nil, err
	}
	actor := p.UserID
	event := domain.OrderEvent{
		OrderID: o.ID, Status: to, ActorUserID: &actor, OccurredAt: at,
		Payload: map[string]any{"from": string(o.Status)},
	}
	if err := uc.orders.UpdateStatus(ctx, o, event); err != nil {
		return nil, err
	}
	return o, nil
}

// Assign vincula vehículo y conductor a la orden (RF-06, rol PROVIDER).
func (uc *OrdersUseCase) Assign(ctx context.Context, p authz.Principal, id, vehicleID, driverID string) (*domain.Order, error) {
	if p.IsGuest() || !p.HasRole(authz.RoleProvider) {
		return nil, domain.ErrForbiddenScope
	}
	if strings.TrimSpace(vehicleID) == "" || strings.TrimSpace(driverID) == "" {
		return nil, fmt.Errorf("%w: vehicle_id y driver_id requeridos", domain.ErrValidation)
	}
	o, err := uc.orders.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	ok, err := uc.orders.VehicleBelongsToProvider(ctx, vehicleID, p.ProviderID)
	if err != nil {
		return nil, err
	}
	if !ok {
		// No revelar si el vehículo existe: mismo error de scope.
		return nil, domain.ErrForbiddenScope
	}
	ok, err = uc.orders.DriverBelongsToVehicle(ctx, driverID, vehicleID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: conductor no asignado a ese vehículo", domain.ErrValidation)
	}

	actor := p.UserID
	event := domain.OrderEvent{
		OrderID: o.ID, Status: o.Status, ActorUserID: &actor, OccurredAt: uc.now(),
		Payload: map[string]any{"vehicle_id": vehicleID, "driver_id": driverID},
	}
	if err := uc.orders.Assign(ctx, o, vehicleID, driverID, event); err != nil {
		return nil, err
	}
	return o, nil
}

// IsAccessibleBy decide si el principal puede ver la orden (para tokens de invitado).
func (uc *OrdersUseCase) IsAccessibleBy(ctx context.Context, orderID string, p authz.Principal) (bool, error) {
	o, err := uc.orders.FindByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return uc.canAccess(ctx, p, o), nil
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
