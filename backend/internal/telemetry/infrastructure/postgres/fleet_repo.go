package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"tiendas-track/backend/internal/telemetry/domain"
)

// FleetRepository implementa domain.FleetRepository sobre app.vehicles y la
// hypertable de telemetría. Una sola consulta por página resuelve la última
// posición de cada vehículo (LATERAL, sin N+1).
type FleetRepository struct {
	pool *pgxpool.Pool
}

// NewFleetRepository crea el repositorio.
func NewFleetRepository(pool *pgxpool.Pool) *FleetRepository { return &FleetRepository{pool: pool} }

// scopedVehiclesCTE aplica el alcance del rol. Un identificador vacío se
// convierte en NULL y nunca coincide (lista vacía para STORE sin tienda).
const scopedVehiclesCTE = `
WITH scoped AS (
    SELECT v.id, v.plate, v.status::text AS status
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

// ListVehicles devuelve la página de vehículos del alcance y el total.
func (r *FleetRepository) ListVehicles(ctx context.Context, scope domain.FleetScope, limit, offset int) ([]domain.Vehicle, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, scopedVehiclesCTE+`
		SELECT count(*) FROM scoped`,
		scope.ProviderID, scope.StoreID, scope.DriverID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("fleet.count: %w", err)
	}

	rows, err := r.pool.Query(ctx, scopedVehiclesCTE+`
		SELECT s.id::text, s.plate, s.status,
		       p.time, p.vehicle_id::text, p.device_seq, p.order_id::text,
		       ST_Y(p.location)::float8, ST_X(p.location)::float8,
		       p.speed_kph, p.accuracy_m, p.heading, p.source::text, p.received_at
		FROM scoped s
		LEFT JOIN LATERAL (
		    SELECT tp.* FROM telemetry.points tp
		    WHERE tp.vehicle_id = s.id
		    ORDER BY tp.time DESC
		    LIMIT 1
		) p ON true
		ORDER BY s.plate ASC
		LIMIT $4 OFFSET $5`,
		scope.ProviderID, scope.StoreID, scope.DriverID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("fleet.list: %w", err)
	}
	defer rows.Close()

	vehicles := make([]domain.Vehicle, 0, limit)
	for rows.Next() {
		var v domain.Vehicle
		var (
			pTime   *time.Time
			pVeh    *string
			pSeq    *int64
			pOrder  *string
			pLat    *float64
			pLon    *float64
			pSpeed  *float64
			pAcc    *float64
			pHead   *float64
			pSource *string
			pRecv   *time.Time
		)
		if err := rows.Scan(&v.ID, &v.Plate, &v.Status,
			&pTime, &pVeh, &pSeq, &pOrder, &pLat, &pLon,
			&pSpeed, &pAcc, &pHead, &pSource, &pRecv); err != nil {
			return nil, 0, fmt.Errorf("fleet.list.scan: %w", err)
		}
		if pTime != nil {
			last := &domain.Point{
				Time:       *pTime,
				VehicleID:  derefString(pVeh),
				DeviceSeq:  derefInt64(pSeq),
				OrderID:    derefString(pOrder),
				Lat:        derefFloat(pLat),
				Lon:        derefFloat(pLon),
				SpeedKPH:   pSpeed,
				AccuracyM:  pAcc,
				Heading:    pHead,
				Source:     domain.Source(derefString(pSource)),
				ReceivedAt: derefTime(pRecv),
			}
			v.Last = last
		}
		vehicles = append(vehicles, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("fleet.list.rows: %w", err)
	}
	return vehicles, total, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt64(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}

func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// Compilación estática: el adaptador satisface el puerto.
var _ domain.FleetRepository = (*FleetRepository)(nil)
