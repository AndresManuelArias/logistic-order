package domain

import (
	"context"
	"errors"
	"math"
	"time"
)

// Scope es el ámbito de datos resuelto desde el token. Solo se rellena el
// campo correspondiente al rol; el repositorio lo aplica en SQL.
type Scope struct {
	ProviderID string
	StoreID    string
	DriverID   string
}

// Empty indica si el scope no permite consultar nada.
func (s Scope) Empty() bool {
	return s.ProviderID == "" && s.StoreID == "" && s.DriverID == ""
}

// StoppedVehicle es un vehículo con una parada vigente.
type StoppedVehicle struct {
	VehicleID    string    `json:"vehicle_id"`
	Plate        string    `json:"plate"`
	StoreID      string    `json:"store_id,omitempty"`
	StoreName    string    `json:"store_name,omitempty"`
	OrderID      string    `json:"order_id,omitempty"`
	ExternalRef  string    `json:"external_ref,omitempty"`
	StoppedSince time.Time `json:"stopped_since"`
	Minutes      int       `json:"minutes"`
	Lat          float64   `json:"lat"`
	Lon          float64   `json:"lon"`
}

// OrderStatus es la fotografía de una orden para el agente.
type OrderStatus struct {
	OrderID            string     `json:"order_id"`
	ExternalRef        string     `json:"external_ref"`
	Status             string     `json:"status"`
	StoreID            string     `json:"store_id"`
	StoreName          string     `json:"store_name"`
	VehiclePlate       string     `json:"vehicle_plate,omitempty"`
	DriverName         string     `json:"driver_name,omitempty"`
	DestinationAddress string     `json:"destination_address,omitempty"`
	PlacedAt           time.Time  `json:"placed_at"`
	PickedUpAt         *time.Time `json:"picked_up_at,omitempty"`
	DeliveredAt        *time.Time `json:"delivered_at,omitempty"`
}

// FleetETA es una orden en tránsito con estimación de llegada.
type FleetETA struct {
	OrderID            string    `json:"order_id"`
	ExternalRef        string    `json:"external_ref"`
	VehiclePlate       string    `json:"vehicle_plate,omitempty"`
	DestinationAddress string    `json:"destination_address,omitempty"`
	DistanceMeters     float64   `json:"distance_meters"`
	SpeedKPH           float64   `json:"speed_kph"`
	ETAMinutes         int       `json:"eta_minutes"`
	ETAAt              time.Time `json:"eta_at"`
	LastReportedAt     time.Time `json:"last_reported_at"`
}

// LateDelivery es una orden pendiente que superó la ventana configurada.
type LateDelivery struct {
	OrderID            string    `json:"order_id"`
	ExternalRef        string    `json:"external_ref"`
	Status             string    `json:"status"`
	StoreName          string    `json:"store_name"`
	DestinationAddress string    `json:"destination_address,omitempty"`
	PlacedAt           time.Time `json:"placed_at"`
	HoursPending       int       `json:"hours_pending"`
}

// AnalyticsRepository es el puerto de solo lectura del agente. Implementa
// consultas predefinidas con el scope ya resuelto; nunca ejecuta SQL libre.
type AnalyticsRepository interface {
	StoppedVehicles(ctx context.Context, scope Scope, minutes int, area string, limit int) ([]StoppedVehicle, error)
	OrderStatus(ctx context.Context, scope Scope, orderID string) (*OrderStatus, error)
	FleetETA(ctx context.Context, scope Scope, storeID string, limit int) ([]FleetETA, error)
	LateDeliveries(ctx context.Context, scope Scope, hours, limit int) ([]LateDelivery, error)
}

// AuditRepository persiste cada consulta (RF-10). Devuelve el id generado.
type AuditRepository interface {
	Record(ctx context.Context, rec AuditRecord) (string, error)
}

// ETAMinutes calcula una estimación declarada: distancia en línea recta entre
// la última posición y el destino dividida por la velocidad de crucero.
// No modela tráfico ni ruta real (ver supuestos del diseño F5).
func ETAMinutes(distanceMeters, speedKPH float64) int {
	if math.IsNaN(distanceMeters) || distanceMeters <= 0 {
		return 0
	}
	if math.IsNaN(speedKPH) || speedKPH <= 0 {
		speedKPH = DefaultETASpeedKPH
	}
	return int(math.Ceil(distanceMeters / (speedKPH * 1000 / 60)))
}

// IsNotFound permite a los adaptadores distinguir una fila ausente.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
