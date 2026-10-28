package domain

import "context"

// Vehicle es un vehículo visible para el principal autenticado.
// Last es la última muestra GPS conocida (nil si aún no reporta).
type Vehicle struct {
	ID     string
	Plate  string
	Status string
	Last   *Point
}

// FleetScope es el alcance de datos con el que se filtra la flota.
// Solo se aplica el campo correspondiente al rol (el resto va vacío).
type FleetScope struct {
	ProviderID string // app.users.id del proveedor dueño de la flota
	StoreID    string // app.stores.id del tendero
	DriverID   string // app.drivers.id del conductor
}

// FleetRepository consulta la flota con el alcance ya resuelto por el caso de uso.
// Es un puerto del dominio: no conoce HTTP ni el motor de base de datos.
type FleetRepository interface {
	// ListVehicles devuelve la página de vehículos y el total del alcance.
	// Orden estable por placa ascendente.
	ListVehicles(ctx context.Context, scope FleetScope, limit, offset int) ([]Vehicle, int, error)
}
