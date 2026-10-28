// Package authz define el principal autenticado y el vocabulario de scope.
// Es un paquete de tipos puros (solo stdlib): lo consumen middleware y casos de uso.
package authz

// TokenType distingue un usuario autenticado de un token de invitado.
type TokenType string

const (
	AccessToken TokenType = "access"
	GuestToken  TokenType = "guest"
)

// Roles del sistema (RF-02).
const (
	RoleProvider = "PROVIDER"
	RoleStore    = "STORE"
	RoleDriver   = "DRIVER"
)

// Principal es el resultado de la autenticación disponible en el contexto.
type Principal struct {
	UserID    string
	Role      string
	TokenType TokenType

	// Identidad de scope (RBAC a nivel de datos).
	StoreID    string
	ProviderID string
	DriverID   string

	// Scope del token de invitado: "order:<id>".
	Scope string
}

// IsGuest indica si el principal proviene de un token de invitado.
func (p Principal) IsGuest() bool { return p.TokenType == GuestToken }

// GuestOrderID devuelve la orden a la que un invitado tiene acceso.
func (p Principal) GuestOrderID() (string, bool) {
	if !p.IsGuest() || len(p.Scope) <= len("order:") {
		return "", false
	}
	return p.Scope[len("order:"):], true
}

// HasRole comprueba pertenencia a un rol.
func (p Principal) HasRole(role string) bool { return p.Role == role }
