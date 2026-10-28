package authz

import "strings"

// Canales de tiempo real (ADR-03, ADR-08). El nombre codifica el scope:
// la autorización se valida contra el principal al suscribirse y al publicar.
const (
	ChannelFleetProvider = "fleet:provider" // flota B2B del proveedor
	ChannelFleetStore    = "fleet:store"    // vista dual del tendero (entrada/salida)
	ChannelOrder         = "order"          // seguimiento de una orden (invitado incluido)
	ChannelVehicle       = "vehicle"        // posición de un vehículo concreto
)

// Channel es un canal parseado: tipo + identificador (UUID).
type Channel struct {
	Kind string
	ID   string
}

// String reconstruye el nombre canónico del canal.
func (c Channel) String() string { return c.Kind + ":" + c.ID }

// ParseChannel valida el formato "<kind>:<uuid>" de un canal de suscripción.
// Los canales de flota llevan dos segmentos de tipo ("fleet:provider:<uuid>").
// Devuelve ok=false para cualquier otro formato (nunca se acepta texto libre).
func ParseChannel(raw string) (Channel, bool) {
	kind, rest, found := strings.Cut(raw, ":")
	if !found {
		return Channel{}, false
	}
	if kind == "fleet" {
		sub, id, ok := strings.Cut(rest, ":")
		if !ok {
			return Channel{}, false
		}
		kind, rest = "fleet:"+sub, id
	}
	if !isUUID(rest) {
		return Channel{}, false
	}
	switch kind {
	case ChannelFleetProvider, ChannelFleetStore, ChannelOrder, ChannelVehicle:
		return Channel{Kind: kind, ID: rest}, true
	default:
		return Channel{}, false
	}
}

// isUUID valida la forma 8-4-4-4-12 en hexadecimal (suficiente para no
// aceptar identificadores arbitrarios; la BD valida el valor real).
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch i {
		case 8, 13, 18, 23:
			if s[i] != '-' {
				return false
			}
		default:
			c := s[i]
			isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}
