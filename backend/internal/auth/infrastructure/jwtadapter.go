// Package infrastructure: adaptadores concretos del módulo auth.
// El adaptador traduce el dominio al servicio JWT compartido.
package infrastructure

import (
	"fmt"

	"tiendas-track/backend/internal/auth/domain"
	"tiendas-track/backend/internal/shared/token"
)

// JWTAdapter implementa domain.TokenService sobre shared/token.
type JWTAdapter struct {
	svc *token.Service
}

// NewJWTAdapter crea el adaptador.
func NewJWTAdapter(svc *token.Service) *JWTAdapter { return &JWTAdapter{svc: svc} }

// IssueAccess emite un token de acceso con la identidad RBAC.
func (a *JWTAdapter) IssueAccess(userID, role, storeID, providerID, driverID string) (domain.IssuedToken, error) {
	t, err := a.svc.IssueAccess(userID, token.Identity{
		Role: role, StoreID: storeID, ProviderID: providerID, DriverID: driverID,
	})
	return toDomain(t), err
}

// IssueRefresh emite un refresh token.
func (a *JWTAdapter) IssueRefresh(userID string) (domain.IssuedToken, error) {
	t, err := a.svc.IssueRefresh(userID)
	return toDomain(t), err
}

// IssueGuest emite un token de invitado con scope order:<id>.
func (a *JWTAdapter) IssueGuest(orderID string) (domain.IssuedToken, error) {
	t, err := a.svc.IssueGuest(orderID)
	return toDomain(t), err
}

// Parse valida y traduce los claims al dominio.
func (a *JWTAdapter) Parse(raw string) (*domain.TokenClaims, error) {
	c, err := a.svc.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &domain.TokenClaims{
		JTI: c.ID, Subject: c.Subject, Role: c.Role,
		StoreID: c.StoreID, ProviderID: c.ProviderID, DriverID: c.DriverID,
		Scope: c.Scope, Kind: string(c.Kind), ExpiresAt: c.ExpiresAt.Time,
	}, nil
}

func toDomain(t token.Issued) domain.IssuedToken {
	return domain.IssuedToken{Raw: t.Raw, JTI: t.JTI, ExpiresAt: t.ExpiresAt}
}

// Compile-time assertion: el adaptador cumple el puerto del dominio.
var _ domain.TokenService = (*JWTAdapter)(nil)

// ErrAdapter es un error interno del adaptador (nunca debe llegar al cliente).
var ErrAdapter = fmt.Errorf("adaptador jwt interno")
