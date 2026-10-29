// Package domain: modelo de conductores (RF-06).
// Un Driver es la identidad operativa de un usuario DRIVER vinculada a un vehículo.
package domain

import (
	"context"
	"errors"
)

// Errores de dominio.
var (
	ErrNotFound       = errors.New("conductor no encontrado")
	ErrForbiddenScope = errors.New("fuera del scope del proveedor")
)

// Driver es el conductor asignado a un vehículo de la flota.
type Driver struct {
	ID            string // app.drivers.id (PK — este es el driverID que se usa en Assign)
	UserID        string // app.users.id
	FullName      string // app.users.full_name
	LicenseNumber string
	VehicleID     string // vehículo asignado
}

// DriverRepository es el puerto del dominio hacia la persistencia.
type DriverRepository interface {
	// ListByVehicle devuelve los conductores asignados al vehículo indicado.
	// Solo se llama cuando el vehículo ya fue validado como perteneciente al
	// proveedor autenticado (el use case hace esa validación previa).
	ListByVehicle(ctx context.Context, vehicleID string) ([]Driver, error)

	// VehicleBelongsToProvider valida que el vehículo pertenece al proveedor.
	VehicleBelongsToProvider(ctx context.Context, vehicleID, providerID string) (bool, error)
}
