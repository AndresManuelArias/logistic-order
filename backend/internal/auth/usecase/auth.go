// Package usecase: casos de uso de autenticación (RF-01, RF-02).
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"tiendas-track/backend/internal/auth/domain"
	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/tracing"
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
	log         *slog.Logger
}

// New construye el caso de uso.
func New(users domain.UserRepository, identity domain.IdentityRepository,
	refresh domain.RefreshTokenRepository, guests domain.GuestTokenRepository,
	tokens domain.TokenService, hasher PasswordHasher,
	orderAccess OrderScopeChecker, log *slog.Logger) *AuthUseCase {
	return &AuthUseCase{
		users: users, identity: identity, refresh: refresh, guests: guests,
		tokens: tokens, hasher: hasher, orderAccess: orderAccess, log: log,
	}
}

// RegisterInput es la entrada de registro.
type RegisterInput struct {
	Email    string
	Password string
	FullName string
	Role     string

	// Store es obligatorio cuando Role == STORE (la tienda nace con su dueño).
	// Address es opcional; Name y coordenadas son requeridos (ADR-09).
	Store *domain.StoreProfile
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
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("Register: iniciando registro de usuario",
		"trace_id", traceID,
		"email", strings.TrimSpace(strings.ToLower(in.Email)),
		"role", in.Role,
	)

	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if !emailRe.MatchString(in.Email) {
		uc.log.Error("Register: email inválido",
			"trace_id", traceID,
			"email", in.Email,
		)
		return nil, domain.ErrInvalidEmail
	}
	if len(in.Password) < minPasswordLen {
		uc.log.Error("Register: contraseña muy corta",
			"trace_id", traceID,
		)
		return nil, domain.ErrWeakPassword
	}
	if !domain.CanRegisterRole(in.Role) {
		uc.log.Error("Register: rol no válido para registro",
			"trace_id", traceID,
			"role", in.Role,
		)
		return nil, domain.ErrInvalidRole
	}

	if existing, err := uc.users.FindByEmail(ctx, in.Email); err == nil && existing != nil {
		uc.log.Error("Register: email ya registrado",
			"trace_id", traceID,
			"email", in.Email,
		)
		return nil, domain.ErrEmailAlreadyRegistered
	}

	// El rol STORE nace con su tienda (RF-02): sin perfil no hay aislamiento
	// de datos posible, porque orders.store_id referencia app.stores.
	if in.Role == authz.RoleStore {
		if err := normalizeStore(in.Store); err != nil {
			uc.log.Error("Register: perfil de tienda inválido",
				"trace_id", traceID,
				"error", err.Error(),
			)
			return nil, err
		}
	}

	hash, err := uc.hasher.Hash(in.Password)
	if err != nil {
		uc.log.Error("Register: error al hashear contraseña",
			"trace_id", traceID,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("hash de contraseña: %w", err)
	}

	u := &domain.User{
		Email:        in.Email,
		PasswordHash: hash,
		FullName:     strings.TrimSpace(in.FullName),
		Role:         in.Role,
		Status:       "ACTIVE",
	}
	if in.Role == authz.RoleStore {
		// Usuario + tienda en una sola transacción (consistencia).
		if _, err := uc.users.CreateWithStore(ctx, u, in.Store); err != nil {
			uc.log.Error("Register: error creando usuario con tienda",
				"trace_id", traceID,
				"error", err.Error(),
			)
			return nil, err
		}
	} else if err := uc.users.Create(ctx, u); err != nil {
		uc.log.Error("Register: error creando usuario",
			"trace_id", traceID,
			"error", err.Error(),
		)
		return nil, err
	}
	u.PasswordHash = "" // nunca exponer el hash

	uc.log.Info("Register: usuario creado exitosamente",
		"trace_id", traceID,
		"user_id", u.ID,
		"role", u.Role,
	)
	return u, nil
}

// normalizeStore normaliza y valida el perfil de tienda del registro STORE.
func normalizeStore(s *domain.StoreProfile) error {
	if s == nil {
		return fmt.Errorf("%w: falta la información de la tienda (name, lat, lon)", domain.ErrValidation)
	}
	s.Name = strings.TrimSpace(s.Name)
	s.Address = strings.TrimSpace(s.Address)
	if !s.Valid() {
		return fmt.Errorf("%w: la tienda requiere nombre y coordenadas válidas (PostGIS, ADR-09)", domain.ErrValidation)
	}
	return nil
}

// Login valida credenciales y emite el par de tokens.
func (uc *AuthUseCase) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("Login: iniciando login",
		"trace_id", traceID,
		"email", strings.TrimSpace(strings.ToLower(email)),
	)

	email = strings.TrimSpace(strings.ToLower(email))
	u, err := uc.users.FindByEmail(ctx, email)
	if err != nil || !uc.hasher.Verify(u.PasswordHash, password) {
		// Mismo error para email inexistente y password incorrecto (sin enumeración).
		uc.log.Error("Login: credenciales inválidas",
			"trace_id", traceID,
		)
		return nil, domain.ErrInvalidCredentials
	}
	if u.Status != "ACTIVE" {
		uc.log.Error("Login: usuario suspendido",
			"trace_id", traceID,
			"user_id", u.ID,
		)
		return nil, domain.ErrUserSuspended
	}

	pair, err := uc.issuePair(ctx, u)
	if err != nil {
		uc.log.Error("Login: error emitiendo tokens",
			"trace_id", traceID,
			"user_id", u.ID,
			"error", err.Error(),
		)
		return nil, err
	}
	u.PasswordHash = ""
	pair.User = u

	uc.log.Info("Login: login exitoso",
		"trace_id", traceID,
		"user_id", u.ID,
		"role", u.Role,
	)
	return pair, nil
}

// Refresh rota el refresh token: el presentado se revoca y se emite uno nuevo.
func (uc *AuthUseCase) Refresh(ctx context.Context, rawRefresh string) (*TokenPair, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("Refresh: iniciando refresco de token",
		"trace_id", traceID,
	)

	claims, err := uc.tokens.Parse(rawRefresh)
	if err != nil || claims.Kind != "refresh" {
		uc.log.Error("Refresh: token de refresh inválido",
			"trace_id", traceID,
			"error", "token inválido o tipo incorrecto",
		)
		return nil, domain.ErrInvalidRefreshToken
	}

	hash := hashToken(rawRefresh)
	active, err := uc.refresh.FindActiveByHash(ctx, hash)
	if err != nil {
		uc.log.Error("Refresh: error verificando refresh token",
			"trace_id", traceID,
			"error", err.Error(),
		)
		return nil, err
	}
	if !active {
		uc.log.Error("Refresh: refresh token no activo o ya usado",
			"trace_id", traceID,
		)
		return nil, domain.ErrInvalidRefreshToken
	}

	u, err := uc.users.FindByID(ctx, claims.Subject)
	if err != nil {
		uc.log.Error("Refresh: usuario no encontrado",
			"trace_id", traceID,
			"user_id", claims.Subject,
		)
		return nil, domain.ErrInvalidRefreshToken
	}
	if u.Status != "ACTIVE" {
		uc.log.Error("Refresh: usuario suspendido",
			"trace_id", traceID,
			"user_id", u.ID,
		)
		return nil, domain.ErrUserSuspended
	}

	pair, err := uc.issuePair(ctx, u)
	if err != nil {
		uc.log.Error("Refresh: error emitiendo par de tokens",
			"trace_id", traceID,
			"user_id", u.ID,
			"error", err.Error(),
		)
		return nil, err
	}
	// Rotación: revocar el token presentado tras emitir el nuevo.
	if err := uc.refresh.Revoke(ctx, hash); err != nil {
		uc.log.Error("Refresh: error revocando refresh token",
			"trace_id", traceID,
			"error", err.Error(),
		)
		return nil, err
	}
	u.PasswordHash = ""
	pair.User = u

	uc.log.Info("Refresh: refresh exitoso",
		"trace_id", traceID,
		"user_id", u.ID,
	)
	return pair, nil
}

// Logout revoca el refresh token presentado.
func (uc *AuthUseCase) Logout(ctx context.Context, rawRefresh string) error {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("Logout: iniciando logout",
		"trace_id", traceID,
	)

	claims, err := uc.tokens.Parse(rawRefresh)
	if err != nil || claims.Kind != "refresh" {
		uc.log.Error("Logout: token de refresh inválido",
			"trace_id", traceID,
		)
		return domain.ErrInvalidRefreshToken
	}
	if err := uc.refresh.Revoke(ctx, hashToken(rawRefresh)); err != nil {
		uc.log.Error("Logout: error revocando refresh token",
			"trace_id", traceID,
			"error", err.Error(),
		)
		return err
	}

	uc.log.Info("Logout: logout exitoso",
		"trace_id", traceID,
		"user_id", claims.Subject,
	)
	return nil
}

// GuestToken emite un token de invitado limitado a una orden (RF-02, ADR-04).
func (uc *AuthUseCase) GuestToken(ctx context.Context, p authz.Principal, orderID string) (*GuestTokenOutput, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("GuestToken: iniciando emisión de token de invitado",
		"trace_id", traceID,
		"order_id", orderID,
		"caller_role", p.Role,
	)

	if p.IsGuest() || (p.Role != authz.RoleStore && p.Role != authz.RoleProvider) {
		uc.log.Error("GuestToken: acceso denegado",
			"trace_id", traceID,
			"role", p.Role,
		)
		return nil, domain.ErrOrderAccess
	}
	if strings.TrimSpace(orderID) == "" {
		uc.log.Error("GuestToken: order_id requerido",
			"trace_id", traceID,
		)
		return nil, domain.ErrOrderAccess
	}
	if uc.orderAccess == nil {
		uc.log.Error("GuestToken: verificador de orden no configurado",
			"trace_id", traceID,
		)
		return nil, errors.New("verificador de orden no configurado")
	}
	ok, err := uc.orderAccess(ctx, orderID, p)
	if err != nil {
		uc.log.Error("GuestToken: error verificando acceso a orden",
			"trace_id", traceID,
			"order_id", orderID,
			"error", err.Error(),
		)
		return nil, err
	}
	if !ok {
		uc.log.Error("GuestToken: acceso denegado a orden",
			"trace_id", traceID,
			"order_id", orderID,
		)
		return nil, domain.ErrOrderAccess
	}

	issued, err := uc.tokens.IssueGuest(orderID)
	if err != nil {
		uc.log.Error("GuestToken: error emitiendo token de invitado",
			"trace_id", traceID,
			"error", err.Error(),
		)
		return nil, err
	}
	if err := uc.guests.Save(ctx, orderID, hashToken(issued.Raw), issued.ExpiresAt); err != nil {
		uc.log.Error("GuestToken: error guardando token de invitado",
			"trace_id", traceID,
			"order_id", orderID,
			"error", err.Error(),
		)
		return nil, err
	}

	uc.log.Info("GuestToken: token de invitado emitido exitosamente",
		"trace_id", traceID,
		"order_id", orderID,
	)
	return &GuestTokenOutput{Token: issued.Raw, ExpiresAt: issued.ExpiresAt, OrderID: orderID}, nil
}

// Me devuelve el usuario autenticado (sin hash).
func (uc *AuthUseCase) Me(ctx context.Context, userID string) (*domain.User, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("Me: obteniendo información de usuario",
		"trace_id", traceID,
		"user_id", userID,
	)

	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		uc.log.Error("Me: error obteniendo usuario",
			"trace_id", traceID,
			"user_id", userID,
			"error", err.Error(),
		)
		return nil, err
	}
	u.PasswordHash = ""

	uc.log.Info("Me: información de usuario obtenida exitosamente",
		"trace_id", traceID,
		"user_id", u.ID,
		"role", u.Role,
	)
	return u, nil
}

func (uc *AuthUseCase) issuePair(ctx context.Context, u *domain.User) (*TokenPair, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("issuePair: emitiendo par de tokens",
		"trace_id", traceID,
		"user_id", u.ID,
	)

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
		uc.log.Error("issuePair: error emitiendo access token",
			"trace_id", traceID,
			"user_id", u.ID,
			"error", err.Error(),
		)
		return nil, err
	}
	refresh, err := uc.tokens.IssueRefresh(u.ID)
	if err != nil {
		uc.log.Error("issuePair: error emitiendo refresh token",
			"trace_id", traceID,
			"user_id", u.ID,
			"error", err.Error(),
		)
		return nil, err
	}
	if err := uc.refresh.Save(ctx, u.ID, hashToken(refresh.Raw), refresh.ExpiresAt); err != nil {
		uc.log.Error("issuePair: error guardando refresh token",
			"trace_id", traceID,
			"user_id", u.ID,
			"error", err.Error(),
		)
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
