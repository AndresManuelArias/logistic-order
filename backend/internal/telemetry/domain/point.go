// Package domain: modelo de telemetría GPS (RF-03). Sin frameworks.
package domain

import (
	"fmt"
	"time"
)

// Source distingue la ingesta individual de la sincronización en bloque.
type Source string

const (
	SourceDevice   Source = "DEVICE"
	SourceBulkSync Source = "BULK_SYNC"
)

// MaxFutureSkew tolera desfases de reloj del dispositivo GPS.
const MaxFutureSkew = 5 * time.Minute

// Point es una muestra GPS de un vehículo.
type Point struct {
	Time       time.Time
	VehicleID  string
	DeviceSeq  int64
	OrderID    string // opcional: orden que el vehículo está atendiendo
	Lat        float64
	Lon        float64
	SpeedKPH   *float64
	AccuracyM  *float64
	Heading    *float64
	Source     Source
	ReceivedAt time.Time
}

// Validate aplica las invariantes de una muestra GPS (ADR-09: PostGIS).
func (p *Point) Validate(now time.Time) error {
	if p.Time.IsZero() {
		return fmt.Errorf("%w: falta el timestamp de la muestra", ErrValidation)
	}
	if p.Time.After(now.Add(MaxFutureSkew)) {
		return fmt.Errorf("%w: el timestamp está en el futuro (desfase de reloj)", ErrValidation)
	}
	if p.VehicleID == "" {
		return fmt.Errorf("%w: falta vehicle_id", ErrValidation)
	}
	if p.DeviceSeq < 0 {
		return fmt.Errorf("%w: device_seq no puede ser negativo", ErrValidation)
	}
	if p.Lat < -90 || p.Lat > 90 || p.Lon < -180 || p.Lon > 180 {
		return fmt.Errorf("%w: coordenada fuera de rango", ErrValidation)
	}
	if p.Lat == 0 && p.Lon == 0 {
		return fmt.Errorf("%w: coordenada nula (0,0)", ErrValidation)
	}
	if p.SpeedKPH != nil && *p.SpeedKPH < 0 {
		return fmt.Errorf("%w: speed_kph no puede ser negativo", ErrValidation)
	}
	if p.AccuracyM != nil && *p.AccuracyM < 0 {
		return fmt.Errorf("%w: accuracy_m no puede ser negativo", ErrValidation)
	}
	if p.Heading != nil && (*p.Heading < 0 || *p.Heading > 360) {
		return fmt.Errorf("%w: heading debe estar entre 0 y 360", ErrValidation)
	}
	switch p.Source {
	case SourceDevice, SourceBulkSync:
	default:
		return fmt.Errorf("%w: source inválido (DEVICE|BULK_SYNC)", ErrValidation)
	}
	return nil
}

// InsertedKey identifica una muestra realmente insertada (no duplicada).
type InsertedKey struct {
	Time      time.Time
	VehicleID string
	DeviceSeq int64
}

// Receipt es el recibo idempotente de una sincronización en bloque (RF-09).
type Receipt struct {
	Key          string
	DeviceID     string
	DriverUserID string // app.users.id del conductor, si aplica
	PointsCount  int
}

// BulkResult resume el procesamiento de un lote idempotente.
type BulkResult struct {
	Inserted         []InsertedKey
	AlreadyProcessed bool
	ProcessedAt      time.Time
}

// Duplicates devuelve cuántos puntos del lote no se insertaron por duplicados.
func (r BulkResult) Duplicates(batch int) int {
	if r.AlreadyProcessed {
		return batch
	}
	return batch - len(r.Inserted)
}

// PointEvent es el hecho de dominio publicado al bus interno y entregado por
// WebSocket. Es intencionadamente plano y estable: es parte del contrato WS.
type PointEvent struct {
	VehicleID  string   `json:"vehicle_id"`
	OrderID    string   `json:"order_id,omitempty"`
	Lat        float64  `json:"lat"`
	Lon        float64  `json:"lon"`
	SpeedKPH   *float64 `json:"speed_kph,omitempty"`
	Heading    *float64 `json:"heading,omitempty"`
	Time       string   `json:"time"`
	ReceivedAt string   `json:"received_at"`
	Source     Source   `json:"source"`
}
