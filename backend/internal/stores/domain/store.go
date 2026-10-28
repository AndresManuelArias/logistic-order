// Package domain: modelo de tiendas del tendero (RF-02). Sin frameworks.
package domain

import "time"

// Store es la tienda de barrio de un usuario con rol STORE.
type Store struct {
	ID          string
	Name        string
	OwnerUserID string
	Address     string
	Lat         float64
	Lon         float64
	CreatedAt   time.Time
}

// Valid comprueba que la ubicación sea una coordenada geográfica real:
// dentro de rangos y nunca el punto nulo (0,0) (ADR-09: PostGIS).
func (s *Store) Valid() bool {
	if s == nil || s.Name == "" {
		return false
	}
	if s.Lat < -90 || s.Lat > 90 || s.Lon < -180 || s.Lon > 180 {
		return false
	}
	return !(s.Lat == 0 && s.Lon == 0)
}
