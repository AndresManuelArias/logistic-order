// Package domain: modelo de órdenes. Reglas de negocio puras (RF-05, RF-06).
package domain

import (
	"fmt"
	"math"
	"time"
)

// Status es el estado del ciclo de vida de una orden (RF-05).
type Status string

const (
	StatusCreated   Status = "CREATED"
	StatusInTransit Status = "IN_TRANSIT"
	StatusDelivered Status = "DELIVERED"
	StatusCancelled Status = "CANCELLED"
)

// ValidStatus indica si el estado es conocido.
func ValidStatus(s Status) bool {
	switch s {
	case StatusCreated, StatusInTransit, StatusDelivered, StatusCancelled:
		return true
	}
	return false
}

// transitions es la máquina de estados autorizada (inmutable por diseño).
var transitions = map[Status][]Status{
	StatusCreated:   {StatusInTransit, StatusCancelled},
	StatusInTransit: {StatusDelivered, StatusCancelled},
	StatusDelivered: {},
	StatusCancelled: {},
}

// CanTransition indica si la transición es legal.
func CanTransition(from, to Status) bool {
	if !ValidStatus(from) || !ValidStatus(to) {
		return false
	}
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// AllowedTransitions expone las transiciones válidas desde un estado (docs/UI).
func AllowedTransitions(from Status) []Status {
	out := make([]Status, len(transitions[from]))
	copy(out, transitions[from])
	return out
}

// Point es una coordenada WGS84 (ADR-09: se persiste como PostGIS).
type Point struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Valid comprueba rangos geográficos.
func (p Point) Valid() bool {
	return !math.IsNaN(p.Lat) && !math.IsNaN(p.Lon) &&
		p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180 &&
		!(p.Lat == 0 && p.Lon == 0)
}

// Order es la entidad central de logística.
type Order struct {
	ID                   string
	ExternalRef          string
	StoreID              string
	CustomerID           *string
	Status               Status
	AssignedVehicleID    *string
	AssignedVehiclePlate *string
	AssignedDriverID     *string
	Origin               Point
	Destination          Point
	DestinationAddress   string
	DistanceMeters       *float64
	PlacedAt             time.Time
	PickedUpAt           *time.Time
	DeliveredAt          *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// CanTransitionTo aplica la máquina de estados sobre la entidad.
func (o *Order) CanTransitionTo(to Status) error {
	if !CanTransition(o.Status, to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, o.Status, to)
	}
	return nil
}

// Transition aplica el cambio de estado y sus hitos temporales.
func (o *Order) Transition(to Status, at time.Time) error {
	if err := o.CanTransitionTo(to); err != nil {
		return err
	}
	o.Status = to
	switch to {
	case StatusInTransit:
		o.PickedUpAt = &at
	case StatusDelivered:
		o.DeliveredAt = &at
	}
	return nil
}

// Assign vincula vehículo y conductor (RF-06).
func (o *Order) Assign(vehicleID, driverID string) {
	o.AssignedVehicleID = &vehicleID
	o.AssignedDriverID = &driverID
}

// OrderEvent es un registro inmutable del historial (append-only en BD).
type OrderEvent struct {
	ID          string
	OrderID     string
	Status      Status
	ActorUserID *string
	OccurredAt  time.Time
	Payload     map[string]any
}
