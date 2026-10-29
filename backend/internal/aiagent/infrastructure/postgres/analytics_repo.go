// Package postgres implementa las consultas analíticas de solo lectura del
// agente (RF-10). El scope del rol se aplica SIEMPRE en SQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tiendas-track/backend/internal/aiagent/domain"
)

// AnalyticsRepository implementa domain.AnalyticsRepository.
type AnalyticsRepository struct {
	pool *pgxpool.Pool
}

// NewAnalyticsRepository crea el repositorio.
func NewAnalyticsRepository(pool *pgxpool.Pool) *AnalyticsRepository {
	return &AnalyticsRepository{pool: pool}
}

// scopedVehiclesCTE aplica el alcance del rol a la flota. Un identificador
// vacío se convierte en NULL y nunca coincide (lista vacía, no error).
const scopedVehiclesCTE = `
WITH scoped AS (
    SELECT v.id, v.plate
    FROM app.vehicles v
    WHERE (NULLIF($1, '')::uuid IS NOT NULL AND v.provider_id = NULLIF($1, '')::uuid)
       OR (NULLIF($2, '')::uuid IS NOT NULL AND EXISTS (
              SELECT 1 FROM app.orders o
              WHERE o.store_id = NULLIF($2, '')::uuid
                AND o.assigned_vehicle_id = v.id))
       OR (NULLIF($3, '')::uuid IS NOT NULL AND v.id = (
              SELECT d.vehicle_id FROM app.drivers d
              WHERE d.id = NULLIF($3, '')::uuid))
)`

const stoppedSQL = scopedVehiclesCTE + `
SELECT s.id::text, s.plate,
       si.stop_start, si.duration_seconds,
       ST_Y(si.location)::float8, ST_X(si.location)::float8,
       COALESCE(o.id::text, ''), COALESCE(o.external_ref, ''),
       COALESCE(st.id::text, ''), COALESCE(st.name, '')
FROM telemetry.stop_intervals si
JOIN scoped s ON s.id = si.vehicle_id
LEFT JOIN app.orders o ON o.id = si.order_id
LEFT JOIN app.stores st ON st.id = o.store_id
WHERE si.duration_seconds >= $4 * 60
  AND si.stop_end >= now() - INTERVAL '3 minutes'
  -- Scope STORE: la parada debe pertenecer a una orden de la tienda del token,
  -- no basta con que el vehículo haya entregado allí en el pasado.
  AND (NULLIF($2, '')::uuid IS NULL OR o.store_id = NULLIF($2, '')::uuid)
  AND ($5 = '' OR st.name ILIKE '%' || $5 || '%' OR st.address ILIKE '%' || $5 || '%')
ORDER BY si.duration_seconds DESC
LIMIT $6`

// StoppedVehicles devuelve paradas vigentes del ámbito. `area` filtra por el
// nombre o la dirección de la tienda de la orden asociada a la parada.
func (r *AnalyticsRepository) StoppedVehicles(ctx context.Context, scope domain.Scope,
	minutes int, area string, limit int) ([]domain.StoppedVehicle, error) {
	rows, err := r.pool.Query(ctx, stoppedSQL,
		scope.ProviderID, scope.StoreID, scope.DriverID, minutes, area, limit)
	if err != nil {
		return nil, fmt.Errorf("aiagent.stopped: %w", err)
	}
	defer rows.Close()

	out := make([]domain.StoppedVehicle, 0, limit)
	for rows.Next() {
		var (
			v         domain.StoppedVehicle
			durationS int
		)
		if err := rows.Scan(&v.VehicleID, &v.Plate, &v.StoppedSince, &durationS,
			&v.Lat, &v.Lon, &v.OrderID, &v.ExternalRef, &v.StoreID, &v.StoreName); err != nil {
			return nil, fmt.Errorf("aiagent.stopped.scan: %w", err)
		}
		v.Minutes = durationS / 60
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("aiagent.stopped.rows: %w", err)
	}
	return out, nil
}

const orderStatusSQL = `
SELECT o.id::text, o.external_ref, o.status::text, o.store_id::text, st.name,
       COALESCE(v.plate, ''), COALESCE(u.full_name, ''), o.destination_address,
       o.placed_at, o.picked_up_at, o.delivered_at
FROM app.orders o
JOIN app.stores st ON st.id = o.store_id
LEFT JOIN app.vehicles v ON v.id = o.assigned_vehicle_id
LEFT JOIN app.drivers d ON d.id = o.assigned_driver_id
LEFT JOIN app.users u ON u.id = d.user_id
WHERE o.id = $4::uuid
  AND (
    (NULLIF($1, '')::uuid IS NOT NULL AND v.provider_id = NULLIF($1, '')::uuid)
    OR (NULLIF($2, '')::uuid IS NOT NULL AND o.store_id = NULLIF($2, '')::uuid)
    OR (NULLIF($3, '')::uuid IS NOT NULL AND o.assigned_driver_id = NULLIF($3, '')::uuid)
  )`

// OrderStatus devuelve una orden del ámbito. "No existe" y "fuera de scope"
// producen el mismo error (anti-enumeración).
func (r *AnalyticsRepository) OrderStatus(ctx context.Context, scope domain.Scope, orderID string) (*domain.OrderStatus, error) {
	var o domain.OrderStatus
	err := r.pool.QueryRow(ctx, orderStatusSQL,
		scope.ProviderID, scope.StoreID, scope.DriverID, orderID,
	).Scan(&o.OrderID, &o.ExternalRef, &o.Status, &o.StoreID, &o.StoreName,
		&o.VehiclePlate, &o.DriverName, &o.DestinationAddress,
		&o.PlacedAt, &o.PickedUpAt, &o.DeliveredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("aiagent.order_status: %w", err)
	}
	return &o, nil
}

const fleetETASQL = `
WITH in_transit AS (
    SELECT o.id, o.external_ref, o.destination, o.destination_address,
           v.id AS vehicle_id, v.plate
    FROM app.orders o
    LEFT JOIN app.vehicles v ON v.id = o.assigned_vehicle_id
    WHERE o.status = 'IN_TRANSIT'
      AND (
        (NULLIF($1, '')::uuid IS NOT NULL AND v.provider_id = NULLIF($1, '')::uuid)
        OR (NULLIF($2, '')::uuid IS NOT NULL AND o.store_id = NULLIF($2, '')::uuid)
        OR (NULLIF($3, '')::uuid IS NOT NULL AND o.assigned_driver_id = NULLIF($3, '')::uuid)
      )
      AND ($4 = '' OR o.store_id = NULLIF($4, '')::uuid)
),
latest AS (
    SELECT DISTINCT ON (p.vehicle_id)
           p.vehicle_id, p.location, p.time, p.speed_kph
    FROM telemetry.points p
    JOIN in_transit i ON i.vehicle_id = p.vehicle_id
    WHERE p.time >= now() - INTERVAL '30 minutes'
    ORDER BY p.vehicle_id, p.time DESC
)
SELECT i.id::text, i.external_ref, COALESCE(i.plate, ''), i.destination_address,
       ST_Distance(l.location::geography, i.destination::geography)::float8,
       COALESCE(NULLIF(l.speed_kph, 0), $5::float8)::float8,
       l.time
FROM in_transit i
JOIN latest l ON l.vehicle_id = i.vehicle_id
ORDER BY 5 ASC
LIMIT $6`

// FleetETA devuelve órdenes en tránsito con última posición reciente y una
// estimación declarada (distancia en línea recta / velocidad de crucero).
func (r *AnalyticsRepository) FleetETA(ctx context.Context, scope domain.Scope,
	storeID string, limit int) ([]domain.FleetETA, error) {
	rows, err := r.pool.Query(ctx, fleetETASQL,
		scope.ProviderID, scope.StoreID, scope.DriverID, storeID, domain.DefaultETASpeedKPH, limit)
	if err != nil {
		return nil, fmt.Errorf("aiagent.fleet_eta: %w", err)
	}
	defer rows.Close()

	out := make([]domain.FleetETA, 0, limit)
	for rows.Next() {
		var e domain.FleetETA
		if err := rows.Scan(&e.OrderID, &e.ExternalRef, &e.VehiclePlate,
			&e.DestinationAddress, &e.DistanceMeters, &e.SpeedKPH, &e.LastReportedAt); err != nil {
			return nil, fmt.Errorf("aiagent.fleet_eta.scan: %w", err)
		}
		e.ETAMinutes = domain.ETAMinutes(e.DistanceMeters, e.SpeedKPH)
		e.ETAAt = e.LastReportedAt.Add(time.Duration(e.ETAMinutes) * time.Minute)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("aiagent.fleet_eta.rows: %w", err)
	}
	return out, nil
}

const lateDeliveriesSQL = `
SELECT o.id::text, o.external_ref, o.status::text, st.name,
       o.destination_address, o.placed_at,
       GREATEST($4, FLOOR(EXTRACT(EPOCH FROM (now() - o.placed_at)) / 3600)::int)
FROM app.orders o
JOIN app.stores st ON st.id = o.store_id
LEFT JOIN app.vehicles v ON v.id = o.assigned_vehicle_id
WHERE o.status IN ('CREATED', 'IN_TRANSIT')
  AND o.placed_at <= now() - make_interval(hours => $4)
  AND (
    (NULLIF($1, '')::uuid IS NOT NULL AND v.provider_id = NULLIF($1, '')::uuid)
    OR (NULLIF($2, '')::uuid IS NOT NULL AND o.store_id = NULLIF($2, '')::uuid)
    OR (NULLIF($3, '')::uuid IS NOT NULL AND o.assigned_driver_id = NULLIF($3, '')::uuid)
  )
ORDER BY o.placed_at ASC
LIMIT $5`

// LateDeliveries devuelve órdenes pendientes que superaron la ventana `hours`.
func (r *AnalyticsRepository) LateDeliveries(ctx context.Context, scope domain.Scope,
	hours, limit int) ([]domain.LateDelivery, error) {
	rows, err := r.pool.Query(ctx, lateDeliveriesSQL,
		scope.ProviderID, scope.StoreID, scope.DriverID, hours, limit)
	if err != nil {
		return nil, fmt.Errorf("aiagent.late_deliveries: %w", err)
	}
	defer rows.Close()

	out := make([]domain.LateDelivery, 0, limit)
	for rows.Next() {
		var d domain.LateDelivery
		if err := rows.Scan(&d.OrderID, &d.ExternalRef, &d.Status, &d.StoreName,
			&d.DestinationAddress, &d.PlacedAt, &d.HoursPending); err != nil {
			return nil, fmt.Errorf("aiagent.late_deliveries.scan: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("aiagent.late_deliveries.rows: %w", err)
	}
	return out, nil
}

// Compilación estática: el adaptador satisface el puerto.
var _ domain.AnalyticsRepository = (*AnalyticsRepository)(nil)
