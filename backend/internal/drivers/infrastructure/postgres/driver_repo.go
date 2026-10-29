// Package postgres: repositorio de conductores sobre PostgreSQL.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"tiendas-track/backend/internal/drivers/domain"
)

// DriverRepository implementa domain.DriverRepository.
type DriverRepository struct {
	pool *pgxpool.Pool
}

// NewDriverRepository crea el repositorio.
func NewDriverRepository(pool *pgxpool.Pool) *DriverRepository {
	return &DriverRepository{pool: pool}
}

// ListByVehicle devuelve todos los conductores asignados al vehículo.
// Hace JOIN con app.users para incluir el nombre completo.
func (r *DriverRepository) ListByVehicle(ctx context.Context, vehicleID string) ([]domain.Driver, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT d.id::text, d.user_id::text, u.full_name, d.license_number, d.vehicle_id::text
		FROM app.drivers d
		JOIN app.users u ON u.id = d.user_id
		WHERE d.vehicle_id = $1::uuid
		ORDER BY u.full_name ASC`,
		vehicleID)
	if err != nil {
		return nil, fmt.Errorf("drivers.list_by_vehicle: %w", err)
	}
	defer rows.Close()

	var out []domain.Driver
	for rows.Next() {
		var d domain.Driver
		if err := rows.Scan(&d.ID, &d.UserID, &d.FullName, &d.LicenseNumber, &d.VehicleID); err != nil {
			return nil, fmt.Errorf("drivers.list_by_vehicle.scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// VehicleBelongsToProvider valida el scope del proveedor (reutiliza la lógica
// ya probada en el módulo de órdenes — consulta idéntica).
func (r *DriverRepository) VehicleBelongsToProvider(ctx context.Context, vehicleID, providerID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM app.vehicles
			WHERE id = $1::uuid AND provider_id = $2::uuid
		)`, vehicleID, providerID).Scan(&ok)
	return ok, err
}

// Compilación estática: garantiza que el adaptador cumple el puerto.
var _ domain.DriverRepository = (*DriverRepository)(nil)
