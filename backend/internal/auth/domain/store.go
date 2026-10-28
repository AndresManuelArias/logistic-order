package domain

// StoreProfile son los datos de la tienda que se crean junto al registro
// de un usuario con rol STORE (RF-01 + RF-02).
//
// Supuesto registrado en el plan: el tendero nace con su tienda; sin ella no
// podría operar porque orders.store_id referencia app.stores. La ubicación es
// obligatoria porque toda coordenada del sistema se persiste con PostGIS
// (ADR-09): no existe tienda sin georreferenciar.
type StoreProfile struct {
	Name    string
	Address string
	Lat     float64
	Lon     float64
}

// Valid comprueba los mínimos del perfil de tienda: nombre no vacío y
// coordenadas dentro de rangos geográficos (nunca el punto nulo 0,0).
func (s *StoreProfile) Valid() bool {
	if s == nil || s.Name == "" {
		return false
	}
	if s.Lat < -90 || s.Lat > 90 || s.Lon < -180 || s.Lon > 180 {
		return false
	}
	return !(s.Lat == 0 && s.Lon == 0)
}
