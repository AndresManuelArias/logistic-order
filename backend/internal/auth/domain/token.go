package domain

import "time"

// IssuedToken es un token emitido por el proveedor de tokens.
type IssuedToken struct {
	Raw       string
	JTI       string
	ExpiresAt time.Time
}

// TokenClaims es la representación de un token parseado.
type TokenClaims struct {
	JTI        string
	Subject    string
	Role       string
	StoreID    string
	ProviderID string
	DriverID   string
	Scope      string
	Kind       string // access | refresh | guest
	ExpiresAt  time.Time
}

// TokenService es el puerto de emisión/validación de tokens (infraestructura).
type TokenService interface {
	IssueAccess(userID, role, storeID, providerID, driverID string) (IssuedToken, error)
	IssueRefresh(userID string) (IssuedToken, error)
	IssueGuest(orderID string) (IssuedToken, error)
	Parse(raw string) (*TokenClaims, error)
}
