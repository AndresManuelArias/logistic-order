package domain

import (
	"context"
	"time"
)

// ListFilter acota la consulta. El repositorio SIEMPRE aplica el scope del rol:
// StoreID/ProviderID/DriverID se rellenan desde el principal autenticado.
type ListFilter struct {
	StoreID    string
	ProviderID string
	DriverID   string
	Status     *Status
	From       *time.Time
	To         *time.Time
	Limit      int
	Offset     int
}

// OrderRepository persiste y consulta órdenes.
type OrderRepository interface {
	Create(ctx context.Context, o *Order) error
	FindByID(ctx context.Context, id string) (*Order, error)
	List(ctx context.Context, f ListFilter) ([]Order, int, error)
	// UpdateStatus persiste el estado y registra el evento en la misma transacción.
	UpdateStatus(ctx context.Context, o *Order, event OrderEvent) error
	// Assign persiste la asignación de flota y registra el evento (RF-06).
	Assign(ctx context.Context, o *Order, vehicleID, driverID string, event OrderEvent) error
	// VehicleBelongsToProvider valida el scope del proveedor sobre un vehículo.
	VehicleBelongsToProvider(ctx context.Context, vehicleID, providerID string) (bool, error)
	// DriverBelongsToVehicle valida coherencia conductor-vehículo.
	DriverBelongsToVehicle(ctx context.Context, driverID, vehicleID string) (bool, error)
	// StoreExists valida la existencia de la tienda.
	StoreExists(ctx context.Context, storeID string) (bool, error)
}
