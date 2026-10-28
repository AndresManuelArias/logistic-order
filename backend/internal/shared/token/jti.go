package token

import (
	"crypto/rand"
	"encoding/hex"
)

// newJTI genera un identificador único de token (128 bits aleatorios).
func newJTI() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand no falla en la práctica; si lo hiciera, el error debe ser visible.
		panic("token: crypto/rand no disponible: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
