package infrastructure

import (
	"golang.org/x/crypto/bcrypt"
)

// PasswordHasher implementa hashing con bcrypt (cost configurable).
type PasswordHasher struct {
	cost int
}

// NewPasswordHasher crea el hasher. cost 0 = bcrypt.DefaultCost.
func NewPasswordHasher(cost int) *PasswordHasher {
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	return &PasswordHasher{cost: cost}
}

// Hash genera el hash de la contraseña.
func (h *PasswordHasher) Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	return string(b), err
}

// Verify compara contraseña en claro con el hash almacenado.
func (h *PasswordHasher) Verify(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

var _ interface {
	Hash(string) (string, error)
	Verify(string, string) bool
} = (*PasswordHasher)(nil)
