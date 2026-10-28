package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tiendas-track/backend/internal/telemetry/domain"
)

// ScopeRepository implementa domain.ScopeRepository sobre las tablas app.*.
// Es el adaptador que permite al módulo telemetry autorizar sin importar los
// módulos auth/orders (frontera de módulos, ADR-01).
type ScopeRepository struct {
	pool *pgxpool.Pool
}

// NewScopeRepository crea el repositorio.
func NewScopeRepository(pool *pgxpool.Pool) *ScopeRepository { return &ScopeRepository{pool: pool} }

// VehicleScope resuelve proveedor y conductor asignado del vehículo.
func (r *ScopeRepository) VehicleScope(ctx context.Context, vehicleID string) (*domain.VehicleScope, error) {
	var vs domain.VehicleScope
	err := r.pool.QueryRow(ctx, `
		SELECT v.id::text, v.provider_id::text, COALESCE(d.id::text, '')
		FROM app.vehicles v
		LEFT JOIN app.drivers d ON d.vehicle_id = v.id
		WHERE v.id = $1::uuid
		LIMIT 1`, vehicleID).Scan(&vs.VehicleID, &vs.ProviderID, &vs.DriverID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry.vehicle_scope: %w", err)
	}
	return &vs, nil
}

// VehicleByDriver devuelve el vehículo asignado al conductor (app.drivers.id).
func (r *ScopeRepository) VehicleByDriver(ctx context.Context, driverID string) (*domain.VehicleScope, error) {
	var vs domain.VehicleScope
	err := r.pool.QueryRow(ctx, `
		SELECT v.id::text, v.provider_id::text, d.id::text
		FROM app.drivers d
		JOIN app.vehicles v ON v.id = d.vehicle_id
		WHERE d.id = $1::uuid
		LIMIT 1`, driverID).Scan(&vs.VehicleID, &vs.ProviderID, &vs.DriverID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry.vehicle_by_driver: %w", err)
	}
	return &vs, nil
}

// OrderScope resuelve la tienda y la asignación de flota de una orden.
func (r *ScopeRepository) OrderScope(ctx context.Context, orderID string) (*domain.OrderScope, error) {
	var os domain.OrderScope
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, store_id::text, status::text,
		       COALESCE(assigned_vehicle_id::text, ''), COALESCE(assigned_driver_id::text, '')
		FROM app.orders
		WHERE id = $1::uuid`, orderID).
		Scan(&os.OrderID, &os.StoreID, &os.Status, &os.AssignedVehicleID, &os.AssignedDriverID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry.order_scope: %w", err)
	}
	return &os, nil
}

// VehicleInStoreScope indica si algún pedido de la tienda tiene asignado el
// vehículo (vista dual del tendero: lo que entra y lo que sale de su tienda).
func (r *ScopeRepository) VehicleInStoreScope(ctx context.Context, vehicleID, storeID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM app.orders
			WHERE store_id = $2::uuid AND assigned_vehicle_id = $1::uuid
		)`, vehicleID, storeID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("telemetry.vehicle_in_store: %w", err)
	}
	return ok, nil
}
