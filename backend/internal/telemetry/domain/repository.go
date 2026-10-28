package domain

import (
	"context"
	"time"
)

// PointRepository persiste y consulta la hypertable telemetry.points.
type PointRepository interface {
	// Insert persiste las muestras (ON CONFLICT DO NOTHING por la PK
	// time+vehicle_id+device_seq) y devuelve solo las realmente insertadas.
	Insert(ctx context.Context, points []Point) ([]InsertedKey, error)

	// ProcessBulk registra el recibo y las muestras en UNA transacción.
	// Es idempotente por receipt.Key: si el recibo ya existe no inserta nada
	// y devuelve AlreadyProcessed=true. Si el insert falla, el recibo se anula.
	ProcessBulk(ctx context.Context, r Receipt, points []Point) (BulkResult, error)

	// Latest devuelve la muestra más reciente del vehículo (ErrNotFound si no hay).
	Latest(ctx context.Context, vehicleID string) (*Point, error)

	// Track devuelve la traza histórica ordenada por tiempo descendente.
	Track(ctx context.Context, vehicleID string, from, to time.Time, limit int) ([]Point, error)
}

// VehicleScope es la identidad de datos de un vehículo (para RBAC).
type VehicleScope struct {
	VehicleID  string
	ProviderID string // app.users.id del proveedor dueño de la flota
	DriverID   string // app.drivers.id asignado, si existe
}

// OrderScope es el alcance de una orden para filtrar telemetría.
type OrderScope struct {
	OrderID           string
	StoreID           string
	Status            string
	AssignedVehicleID string
	AssignedDriverID  string
}

// ScopeRepository resuelve la identidad de vehículos y órdenes para autorizar
// la ingesta y las lecturas. Implementado sobre las tablas app.* del monolito.
type ScopeRepository interface {
	VehicleScope(ctx context.Context, vehicleID string) (*VehicleScope, error)
	// VehicleByDriver devuelve el vehículo del conductor (app.drivers.id).
	VehicleByDriver(ctx context.Context, driverID string) (*VehicleScope, error)
	OrderScope(ctx context.Context, orderID string) (*OrderScope, error)
	// VehicleInStoreScope indica si la tienda tiene una orden (entrada o salida)
	// asignada a ese vehículo (vista dual del tendero, RF-02).
	VehicleInStoreScope(ctx context.Context, vehicleID, storeID string) (bool, error)
}
