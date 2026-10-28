// Package usecase: casos de uso de autenticación (RF-01, RF-02).
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"tiendas-track/backend/internal/auth/domain"
	"tiendas-track/backend/internal/shared/authz"
)

// PasswordHasher es el puerto de hash de contraseñas (infraestructura: bcrypt).
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Verify(hash, plain string) bool
}

// OrderScopeChecker verifica que el llamante pueda invitar a rastrear una orden.
// Se inyecta desde afuera (orders) para no acoplar módulos.
type OrderScopeChecker func(ctx context.Context, orderID string, p authz.Principal) (bool, error)

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const minPasswordLen = 8

// AuthUseCase orquesta registro, login, rotación de refresh y tokens de invitado.
type AuthUseCase struct {
	users       domain.UserRepository
	identity    domain.IdentityRepository
	refresh     domain.RefreshTokenRepository
	guests      domain.GuestTokenRepository
	tokens      domain.TokenService
	hasher      PasswordHasher
	orderAccess OrderScopeChecker
}

// New construye el caso de uso.
func New(users domain.UserRepository, identity domain.IdentityRepository,
	refresh domain.RefreshTokenRepository, guests domain.GuestTokenRepository,
	tokens domain.TokenService, hasher PasswordHasher,
	orderAccess OrderScopeChecker) *AuthUseCase {
	return &AuthUseCase{
		users: users, identity: identity, refresh: refresh, guests: guests,
		tokens: tokens, hasher: hasher, orderAccess: orderAccess,
	}
}

// RegisterInput es la entrada de registro.
type RegisterInput struct {
	Email    string
	Password string
	FullName string
	Role     string
}

// TokenPair es la salida de login/refresh.
type TokenPair struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"` // segundos del access token
	TokenType    string       `json:"token_type"`
	User         *domain.User `json:"user,omitempty"`
}

// GuestTokenOutput es la salida de emisión de token de invitado.
type GuestTokenOutput struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	OrderID   string    `json:"order_id"`
}

// Register crea un usuario con rol STORE o DRIVER (PROVIDER solo por seed).
func (uc *AuthUseCase) Register(ctx context.Context, in RegisterInput) (*domain.User, error) {
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if !emailRe.MatchString(in.Email) {
		return nil, domain.ErrInvalidEmail
	}
	if len(in.Password) < minPasswordLen {
		return nil, domain.ErrWeakPassword
	}
	if !domain.CanRegisterRole(in.Role) {
		return nil, domain.ErrInvalidRole
	}

	if existing, err := uc.users.FindByEmail(ctx, in.Email); err == nil && existing != nil {
		return nil, domain.ErrEmailAlreadyRegistered
	}

	hash, err := uc.hasher.Hash(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash de contraseña: %w", err)
	}

	u := &domain.User{
		Email:        in.Email,
		PasswordHash: hash,
		FullName:     strings.TrimSpace(in.FullName),
		Role:         in.Role,
		Status:       "ACTIVE",
	}
	if err := uc.users.Create(ctx, u); err != nil {
		return nil, err
	}
	u.PasswordHash = "" // nunca exponer el hash
	return u, nil
}

// Login valida credenciales y emite el par de tokens.
func (uc *AuthUseCase) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	u, err := uc.users.FindByEmail(ctx, email)
	if err != nil || !uc.hasher.Verify(u.PasswordHash, password) {
		// Mismo error para email inexistente y password incorrecto (sin enumeración).
		return nil, domain.ErrInvalidCredentials
	}
	if u.Status != "ACTIVE" {
		return nil, domain.ErrUserSuspended
	}

	pair, err := uc.issuePair(ctx, u)
	if err != nil {
		return nil, err
	}
	u.PasswordHash = ""
	pair.User = u
	return pair, nil
}

// Refresh rota el refresh token: el presentado se revoca y se emite uno nuevo.
func (uc *AuthUseCase) Refresh(ctx context.Context, rawRefresh string) (*TokenPair, error) {
	claims, err := uc.tokens.Parse(rawRefresh)
	if err != nil || claims.Kind != "refresh" {
		return nil, domain.ErrInvalidRefreshToken
	}

	hash := hashToken(rawRefresh)
	active, err := uc.refresh.FindActiveByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, domain.ErrInvalidRefreshToken
	}

	u, err := uc.users.FindByID(ctx, claims.Subject)
	if err != nil {
		return nil, domain.ErrInvalidRefreshToken
	}
	if u.Status != "ACTIVE" {
		return nil, domain.ErrUserSuspended
	}

	pair, err := uc.issuePair(ctx, u)
	if err != nil {
		return nil, err
	}
	// Rotación: revocar el token presentado tras emitir el nuevo.
	if err := uc.refresh.Revoke(ctx, hash); err != nil {
		return nil, err
	}
	u.PasswordHash = ""
	pair.User = u
	return pair, nil
}

// Logout revoca el refresh token presentado.
func (uc *AuthUseCase) Logout(ctx context.Context, rawRefresh string) error {
	claims, err := uc.tokens.Parse(rawRefresh)
	if err != nil || claims.Kind != "refresh" {
		return domain.ErrInvalidRefreshToken
	}
	return uc.refresh.Revoke(ctx, hashToken(rawRefresh))
}

// GuestToken emite un token de invitado limitado a una orden (RF-02, ADR-04).
func (uc *AuthUseCase) GuestToken(ctx context.Context, p authz.Principal, orderID string) (*GuestTokenOutput, error) {
	if p.IsGuest() || (p.Role != authz.RoleStore && p.Role != authz.RoleProvider) {
		return nil, domain.ErrOrderAccess
	}
	if strings.TrimSpace(orderID) == "" {
		return nil, domain.ErrOrderAccess
	}
	if uc.orderAccess == nil {
		return nil, errors.New("verificador de orden no configurado")
	}
	ok, err := uc.orderAccess(ctx, orderID, p)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrOrderAccess
	}

	issued, err := uc.tokens.IssueGuest(orderID)
	if err != nil {
		return nil, err
	}
	if err := uc.guests.Save(ctx, orderID, hashToken(issued.Raw), issued.ExpiresAt); err != nil {
		return nil, err
	}
	return &GuestTokenOutput{Token: issued.Raw, ExpiresAt: issued.ExpiresAt, OrderID: orderID}, nil
}

// Me devuelve el usuario autenticado (sin hash).
func (uc *AuthUseCase) Me(ctx context.Context, userID string) (*domain.User, error) {
	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	u.PasswordHash = ""
	return u, nil
}

func (uc *AuthUseCase) issuePair(ctx context.Context, u *domain.User) (*TokenPair, error) {
	storeID, providerID, driverID := "", "", ""
	switch u.Role {
	case authz.RoleStore:
		storeID, _ = uc.identity.FindStoreIDByOwner(ctx, u.ID)
	case authz.RoleDriver:
		driverID, _, _ = uc.identity.FindDriverIdentity(ctx, u.ID)
	case authz.RoleProvider:
		providerID = u.ID
	}

	access, err := uc.tokens.IssueAccess(u.ID, u.Role, storeID, providerID, driverID)
	if err != nil {
		return nil, err
	}
	refresh, err := uc.tokens.IssueRefresh(u.ID)
	if err != nil {
		return nil, err
	}
	if err := uc.refresh.Save(ctx, u.ID, hashToken(refresh.Raw), refresh.ExpiresAt); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  access.Raw,
		RefreshToken: refresh.Raw,
		ExpiresIn:    int64(time.Until(access.ExpiresAt).Seconds()),
		TokenType:    "Bearer",
	}, nil
}

// hashToken: SHA-256 en hex. Los tokens se guardan solo como hash.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
