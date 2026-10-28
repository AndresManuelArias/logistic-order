package authz

import "testing"

const uuidA = "3f2b8a1e-1c4d-4f6a-9b2e-7d8c9a0b1e2f"
const uuidB = "0b7c9d2f-8e1a-4c3b-9f5d-2a6e7c8b9d0f"

// Solo se aceptan los cuatro canales del contrato con identificador UUID.
func TestParseChannelValidos(t *testing.T) {
	cases := []struct {
		raw  string
		kind string
	}{
		{"fleet:provider:" + uuidA, ChannelFleetProvider},
		{"fleet:store:" + uuidB, ChannelFleetStore},
		{"order:" + uuidA, ChannelOrder},
		{"vehicle:" + uuidB, ChannelVehicle},
	}
	for _, c := range cases {
		ch, ok := ParseChannel(c.raw)
		if !ok {
			t.Errorf("%q: rechazado, se esperaba válido", c.raw)
			continue
		}
		if ch.Kind != c.kind {
			t.Errorf("%q: kind = %q", c.raw, ch.Kind)
		}
		if ch.String() != c.raw {
			t.Errorf("%q: round-trip = %q", c.raw, ch.String())
		}
	}
}

// Cualquier variante fuera del contrato se rechaza (no hay texto libre).
func TestParseChannelInvalidos(t *testing.T) {
	cases := []string{
		"",
		uuidA,
		"order:",
		"order",
		"order:" + uuidA + ":extra",
		"unknown:" + uuidA,
		"FLEET:provider:" + uuidA,
		"order:no-es-uuid",
		"order:" + uuidA[:35],                  // falta un carácter
		"order:" + uuidA[:8] + "_" + uuidA[9:], // separador incorrecto
		"order:ZZZZZZZZ-1c4d-4f6a-9b2e-7d8c9a0b1e2f", // hexadecimal inválido
		"fleet:" + uuidA,
	}
	for _, raw := range cases {
		if _, ok := ParseChannel(raw); ok {
			t.Errorf("%q: aceptado, se esperaba inválido", raw)
		}
	}
}

// El scope del invitado solo autoriza la orden firmada en el token.
func TestPrincipalGuestOrder(t *testing.T) {
	p := Principal{TokenType: GuestToken, Scope: "order:" + uuidA}
	if !p.IsGuest() {
		t.Fatal("debe ser invitado")
	}
	id, ok := p.GuestOrderID()
	if !ok || id != uuidA {
		t.Fatalf("GuestOrderID = %q, %v", id, ok)
	}

	if _, ok := (Principal{TokenType: GuestToken}).GuestOrderID(); ok {
		t.Error("un invitado sin scope no debe tener orden")
	}
	if _, ok := (Principal{TokenType: AccessToken, Scope: "order:" + uuidA}).GuestOrderID(); ok {
		t.Error("un access token no expone GuestOrderID")
	}
}

// HasRole es exacto: no hay jerarquías implícitas entre roles.
func TestHasRole(t *testing.T) {
	p := Principal{Role: RoleProvider}
	if !p.HasRole(RoleProvider) {
		t.Error("debe tener PROVIDER")
	}
	if p.HasRole(RoleStore) || p.HasRole(RoleDriver) {
		t.Error("no debe tener otros roles")
	}
}
