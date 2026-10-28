// Package postgres: persistencia de telemetría sobre TimescaleDB/PostGIS.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tiendas-track/backend/internal/telemetry/domain"
)

// PointRepository implementa domain.PointRepository.
type PointRepository struct {
	pool *pgxpool.Pool
}

// NewPointRepository crea el repositorio.
func NewPointRepository(pool *pgxpool.Pool) *PointRepository { return &PointRepository{pool: pool} }

// pointJSON es la fila que se envía como jsonb y se expande en SQL.
type pointJSON struct {
	Time      time.Time `json:"time"`
	VehicleID string    `json:"vehicle_id"`
	DeviceSeq int64     `json:"device_seq"`
	OrderID   *string   `json:"order_id"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	SpeedKPH  *float64  `json:"speed_kph"`
	AccuracyM *float64  `json:"accuracy_m"`
	Heading   *float64  `json:"heading"`
	Source    string    `json:"source"`
}

// insertSQL expande el lote con jsonb_to_recordset (una sola ida a la BD) y
// deja que la PK (time, vehicle_id, device_seq) descarte duplicados.
const insertSQL = `
INSERT INTO telemetry.points
    (time, vehicle_id, device_seq, order_id, location, speed_kph, accuracy_m, heading, source)
SELECT r.time, r.vehicle_id, r.device_seq, r.order_id,
       ST_SetSRID(ST_MakePoint(r.lon, r.lat), 4326),
       r.speed_kph, r.accuracy_m, r.heading, r.source
FROM jsonb_to_recordset($1::jsonb) AS r(
    time        timestamptz,
    vehicle_id  uuid,
    device_seq  bigint,
    order_id    uuid,
    lat         double precision,
    lon         double precision,
    speed_kph   double precision,
    accuracy_m  double precision,
    heading     double precision,
    source      app.source_enum)
ON CONFLICT DO NOTHING
RETURNING time, vehicle_id::text, device_seq`

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func encodePoints(points []domain.Point) (string, error) {
	rows := make([]pointJSON, 0, len(points))
	for _, p := range points {
		row := pointJSON{
			Time:      p.Time,
			VehicleID: p.VehicleID,
			DeviceSeq: p.DeviceSeq,
			Lat:       p.Lat,
			Lon:       p.Lon,
			SpeedKPH:  p.SpeedKPH,
			AccuracyM: p.AccuracyM,
			Heading:   p.Heading,
			Source:    string(p.Source),
		}
		row.OrderID = nullIfEmpty(p.OrderID)
		rows = append(rows, row)
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("telemetry.encode: %w", err)
	}
	return string(b), nil
}

func insertPoints(ctx context.Context, q querier, points []domain.Point) ([]domain.InsertedKey, error) {
	if len(points) == 0 {
		return nil, nil
	}
	payload, err := encodePoints(points)
	if err != nil {
		return nil, err
	}

	rows, err := q.Query(ctx, insertSQL, payload)
	if err != nil {
		return nil, fmt.Errorf("telemetry.insert: %w", err)
	}
	defer rows.Close()

	var inserted []domain.InsertedKey
	for rows.Next() {
		var k domain.InsertedKey
		if err := rows.Scan(&k.Time, &k.VehicleID, &k.DeviceSeq); err != nil {
			return nil, fmt.Errorf("telemetry.insert.scan: %w", err)
		}
		inserted = append(inserted, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry.insert.rows: %w", err)
	}
	return inserted, nil
}

// Insert persiste el lote sin recibo (ingesta individual).
func (r *PointRepository) Insert(ctx context.Context, points []domain.Point) ([]domain.InsertedKey, error) {
	return insertPoints(ctx, r.pool, points)
}

// ProcessBulk registra el recibo y las muestras en una sola transacción.
// Si la clave ya existe no se inserta nada (idempotencia del Bulk Sync, RF-09).
func (r *PointRepository) ProcessBulk(ctx context.Context, rc domain.Receipt, points []domain.Point) (domain.BulkResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.BulkResult{}, fmt.Errorf("telemetry.bulk.begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // el commit es el camino feliz

	var receiptID string
	err = tx.QueryRow(ctx, `
		INSERT INTO app.sync_receipts (idempotency_key, device_id, driver_id, points_count)
		VALUES ($1, $2, $3::uuid, $4)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id::text`,
		rc.Key, rc.DeviceID, nullUUID(rc.DriverUserID), rc.PointsCount,
	).Scan(&receiptID)

	if errors.Is(err, pgx.ErrNoRows) {
		// Reintento del mismo lote: se devuelve el recibo original.
		var processedAt time.Time
		if err := tx.QueryRow(ctx, `
			SELECT processed_at FROM app.sync_receipts WHERE idempotency_key = $1`,
			rc.Key).Scan(&processedAt); err != nil {
			return domain.BulkResult{}, fmt.Errorf("telemetry.bulk.receipt: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.BulkResult{}, fmt.Errorf("telemetry.bulk.commit: %w", err)
		}
		return domain.BulkResult{AlreadyProcessed: true, ProcessedAt: processedAt}, nil
	}
	if err != nil {
		return domain.BulkResult{}, fmt.Errorf("telemetry.bulk.claim: %w", err)
	}

	inserted, err := insertPoints(ctx, tx, points)
	if err != nil {
		return domain.BulkResult{}, err
	}

	var processedAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT processed_at FROM app.sync_receipts WHERE id = $1::uuid`,
		receiptID).Scan(&processedAt); err != nil {
		return domain.BulkResult{}, fmt.Errorf("telemetry.bulk.processed_at: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.BulkResult{}, fmt.Errorf("telemetry.bulk.commit: %w", err)
	}
	return domain.BulkResult{Inserted: inserted, ProcessedAt: processedAt}, nil
}

const pointCols = `
	time, vehicle_id::text, device_seq, COALESCE(order_id::text, ''),
	ST_Y(location)::float8, ST_X(location)::float8,
	speed_kph, accuracy_m, heading, source::text, received_at`

func scanPoint(row pgx.Row) (*domain.Point, error) {
	var p domain.Point
	var source string
	err := row.Scan(&p.Time, &p.VehicleID, &p.DeviceSeq, &p.OrderID,
		&p.Lat, &p.Lon, &p.SpeedKPH, &p.AccuracyM, &p.Heading, &source, &p.ReceivedAt)
	if err != nil {
		return nil, err
	}
	p.Source = domain.Source(source)
	return &p, nil
}

// Latest devuelve la muestra más reciente del vehículo.
func (r *PointRepository) Latest(ctx context.Context, vehicleID string) (*domain.Point, error) {
	p, err := scanPoint(r.pool.QueryRow(ctx, `
		SELECT `+pointCols+`
		FROM telemetry.points
		WHERE vehicle_id = $1::uuid
		ORDER BY time DESC
		LIMIT 1`, vehicleID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry.latest: %w", err)
	}
	return p, nil
}

// Track devuelve la traza del vehículo en [from, to], más reciente primero.
func (r *PointRepository) Track(ctx context.Context, vehicleID string, from, to time.Time, limit int) ([]domain.Point, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+pointCols+`
		FROM telemetry.points
		WHERE vehicle_id = $1::uuid AND time >= $2 AND time <= $3
		ORDER BY time DESC
		LIMIT $4`, vehicleID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("telemetry.track: %w", err)
	}
	defer rows.Close()

	var points []domain.Point
	for rows.Next() {
		p, err := scanPoint(rows)
		if err != nil {
			return nil, fmt.Errorf("telemetry.track.scan: %w", err)
		}
		points = append(points, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry.track.rows: %w", err)
	}
	if len(points) == 0 {
		return nil, domain.ErrNotFound
	}
	return points, nil
}

// nullIfEmpty convierte un string vacío en NULL para columnas uuid.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nullUUID devuelve un uuid válido o nil (SQL NULL).
func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}
