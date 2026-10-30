// Package postgres: repositorio del módulo órdenes sobre PostgreSQL + PostGIS.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tiendas-track/backend/internal/orders/domain"
)

// OrderRepository implementa domain.OrderRepository.
type OrderRepository struct {
	pool *pgxpool.Pool
}

// NewOrderRepository crea el repositorio.
func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository { return &OrderRepository{pool: pool} }

const orderCols = `
	o.id::text, o.external_ref, o.store_id::text, o.customer_id::text, o.status::text,
	o.assigned_vehicle_id::text, v.plate, o.assigned_driver_id::text,
	ST_Y(o.origin)::float8, ST_X(o.origin)::float8,
	ST_Y(o.destination)::float8, ST_X(o.destination)::float8,
	o.destination_address, o.distance_meters,
	o.placed_at, o.picked_up_at, o.delivered_at, o.created_at, o.updated_at`

// scanOrder deserializa una fila de orden (coordenadas PostGIS → lat/lon).
func scanOrder(row pgx.Row) (*domain.Order, error) {
	var o domain.Order
	var status string
	err := row.Scan(
		&o.ID, &o.ExternalRef, &o.StoreID, &o.CustomerID, &status,
		&o.AssignedVehicleID, &o.AssignedVehiclePlate, &o.AssignedDriverID,
		&o.Origin.Lat, &o.Origin.Lon, &o.Destination.Lat, &o.Destination.Lon,
		&o.DestinationAddress, &o.DistanceMeters,
		&o.PlacedAt, &o.PickedUpAt, &o.DeliveredAt, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	o.Status = domain.Status(status)
	return &o, nil
}

// Create inserta la orden con geometrías PostGIS y distancia esférica.
func (r *OrderRepository) Create(ctx context.Context, o *domain.Order) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO app.orders (external_ref, store_id, customer_id, status, origin, destination,
		                        destination_address, distance_meters, placed_at)
		VALUES ($1, $2::uuid, $3::uuid, $4::app.order_status,
		        ST_SetSRID(ST_MakePoint($5, $6), 4326),
		        ST_SetSRID(ST_MakePoint($7, $8), 4326),
		        $9, ST_DistanceSphere(
		              ST_SetSRID(ST_MakePoint($5, $6), 4326),
		              ST_SetSRID(ST_MakePoint($7, $8), 4326)), $10)
		RETURNING id::text, created_at, updated_at`,
		o.ExternalRef, o.StoreID, nullIfEmpty(o.CustomerID), string(o.Status),
		o.Origin.Lon, o.Origin.Lat, o.Destination.Lon, o.Destination.Lat,
		o.DestinationAddress, o.PlacedAt,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		// Violação de unicidad de external_ref → conflicto de idempotencia.
		if strings.Contains(err.Error(), "duplicate key") ||
			strings.Contains(err.Error(), "unique constraint") {
			return domain.ErrExternalRefConflict
		}
		return fmt.Errorf("orders.create: %w", err)
	}
	return nil
}

// FindByID busca por id.
func (r *OrderRepository) FindByID(ctx context.Context, id string) (*domain.Order, error) {
	o, err := scanOrder(r.pool.QueryRow(ctx,
		`SELECT `+orderCols+` FROM app.orders o LEFT JOIN app.vehicles v ON v.id = o.assigned_vehicle_id WHERE o.id = $1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("orders.find: %w", err)
	}
	return o, nil
}

// List consulta con scope de rol aplicado en SQL (nunca en el cliente).
func (r *OrderRepository) List(ctx context.Context, f domain.ListFilter) ([]domain.Order, int, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}

	// Scope RBAC por rol (RF-02): el filtro SIEMPRE viene del principal.
	if f.StoreID != "" {
		add("o.store_id = $%d::uuid", f.StoreID)
	}
	if f.DriverID != "" {
		add("o.assigned_driver_id = $%d::uuid", f.DriverID)
	}
	if f.ProviderID != "" {
		add(`(o.assigned_vehicle_id IS NULL OR EXISTS (SELECT 1 FROM app.vehicles v
		             WHERE v.id = o.assigned_vehicle_id AND v.provider_id = $%d::uuid))`, f.ProviderID)
	}
	if f.Status != nil {
		add("o.status = $%d::app.order_status", string(*f.Status))
	}
	if f.From != nil {
		add("o.placed_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("o.placed_at <= $%d", *f.To)
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM app.orders o WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("orders.list.count: %w", err)
	}

	limit, offset := f.Limit, f.Offset
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	listArgs := append(append([]any{}, args...), limit, offset)

	rows, err := r.pool.Query(ctx,
		`SELECT `+orderCols+` FROM app.orders o LEFT JOIN app.vehicles v ON v.id = o.assigned_vehicle_id WHERE `+cond+`
		 ORDER BY o.placed_at DESC LIMIT $`+itoa(len(args)+1)+` OFFSET $`+itoa(len(args)+2),
		listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("orders.list: %w", err)
	}
	defer rows.Close()

	var out []domain.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("orders.list.scan: %w", err)
		}
		out = append(out, *o)
	}
	return out, total, rows.Err()
}

// UpdateStatus persiste estado + evento en una transacción (consistencia).
func (r *OrderRepository) UpdateStatus(ctx context.Context, o *domain.Order, event domain.OrderEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE app.orders
		SET status = $2::app.order_status, picked_up_at = $3, delivered_at = $4, updated_at = now()
		WHERE id = $1::uuid`,
		o.ID, string(o.Status), o.PickedUpAt, o.DeliveredAt); err != nil {
		return fmt.Errorf("orders.update_status: %w", err)
	}
	if _, err := insertEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Assign persiste la asignación de flota + evento (RF-06).
func (r *OrderRepository) Assign(ctx context.Context, o *domain.Order, vehicleID, driverID string, event domain.OrderEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE app.orders
		SET assigned_vehicle_id = $2::uuid, assigned_driver_id = $3::uuid, updated_at = now()
		WHERE id = $1::uuid`,
		o.ID, vehicleID, driverID); err != nil {
		return fmt.Errorf("orders.assign: %w", err)
	}
	if _, err := insertEvent(ctx, tx, event); err != nil {
		return err
	}
	o.Assign(vehicleID, driverID)
	return tx.Commit(ctx)
}

// VehicleBelongsToProvider valida el scope del proveedor (RF-02).
func (r *OrderRepository) VehicleBelongsToProvider(ctx context.Context, vehicleID, providerID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM app.vehicles
		               WHERE id = $1::uuid AND provider_id = $2::uuid)`,
		vehicleID, providerID).Scan(&ok)
	return ok, err
}

// DriverBelongsToVehicle valida la coherencia conductor-vehículo (RF-06).
func (r *OrderRepository) DriverBelongsToVehicle(ctx context.Context, driverID, vehicleID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM app.drivers
		               WHERE id = $1::uuid AND vehicle_id = $2::uuid)`,
		driverID, vehicleID).Scan(&ok)
	return ok, err
}

// StoreExists valida la existencia de la tienda.
func (r *OrderRepository) StoreExists(ctx context.Context, storeID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM app.stores WHERE id = $1::uuid)`, storeID).Scan(&ok)
	return ok, err
}

func insertEvent(ctx context.Context, tx pgx.Tx, e domain.OrderEvent) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO app.order_events (order_id, status, actor_user_id, occurred_at, payload)
		VALUES ($1::uuid, $2::app.order_status, $3::uuid, $4, $5::jsonb)
		RETURNING id::text`,
		e.OrderID, string(e.Status), nullIfEmpty(e.ActorUserID), e.OccurredAt, toJSONb(e.Payload),
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("order_events.insert: %w", err)
	}
	return id, nil
}

var _ domain.OrderRepository = (*OrderRepository)(nil)
