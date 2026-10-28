package delivery

import (
	"encoding/json"
	"strings"
	"testing"

	"tiendas-track/backend/internal/auth/domain"
	"tiendas-track/backend/internal/auth/usecase"
)

// El contrato público del par de tokens no debe filtrar el hash de contraseña
// ni usar los nombres de campo del dominio (ADR-10).
func TestToTokenPairResponseMatchesContract(t *testing.T) {
	pair := &usecase.TokenPair{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresIn:    900,
		TokenType:    "Bearer",
		User: &domain.User{
			ID:           "u-1",
			Email:        "tienda@tiendas.dev",
			PasswordHash: "$2a$10$secreto",
			FullName:     "Tienda Uno",
			Role:         "STORE",
		},
	}

	payload, err := json.Marshal(toTokenPairResponse(pair))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(payload)

	if strings.Contains(body, "PasswordHash") || strings.Contains(body, "secreto") {
		t.Fatalf("el par de tokens expone el hash de contraseña: %s", body)
	}
	if strings.Contains(body, "FullName") || strings.Contains(body, "\"ID\"") {
		t.Fatalf("el par de tokens usa nombres de campo del dominio: %s", body)
	}
	for _, want := range []string{
		`"access_token":"access"`,
		`"refresh_token":"refresh"`,
		`"expires_in":900`,
		`"token_type":"Bearer"`,
		`"id":"u-1"`,
		`"full_name":"Tienda Uno"`,
		`"role":"STORE"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("falta %s en %s", want, body)
		}
	}
}

func TestToTokenPairResponseSinUsuario(t *testing.T) {
	resp := toTokenPairResponse(&usecase.TokenPair{AccessToken: "a"})
	if resp.User != nil {
		t.Fatalf("no debe inventar un usuario: %+v", resp.User)
	}
}
