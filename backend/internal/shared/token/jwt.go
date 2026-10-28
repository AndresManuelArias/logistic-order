// Package token emite y valida JWT (HS256) para acceso, refresh y invitación.
// Es infraestructura compartida: los módulos consumen la interfaz definida
// en su dominio, nunca esta implementación directamente desde casos de uso.
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Kind identifica el tipo de token (claims "typ").
type Kind string

const (
	KindAccess  Kind = "access"
	KindRefresh Kind = "refresh"
	KindGuest   Kind = "guest"
)

// Identity describe los datos de autorización embebidos en el token.
type Identity struct {
	Role       string
	StoreID    string
	ProviderID string
	DriverID   string
}

// Claims son los claims personalizados de Tiendas-Track.
type Claims struct {
	Role       string `json:"role,omitempty"`
	StoreID    string `json:"store_id,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	DriverID   string `json:"driver_id,omitempty"`
	Scope      string `json:"scope,omitempty"` // guest: order:<id>
	Kind       Kind   `json:"typ"`
	jwt.RegisteredClaims
}

// Issued es un token emitido con su valor en claro y metadatos.
type Issued struct {
	Raw       string
	JTI       string
	ExpiresAt time.Time
}

// Service emite y parsea tokens.
type Service struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	guestTTL   time.Duration
	now        func() time.Time
}

// New crea el servicio. secret debe tener >= 32 bytes en producción.
func New(secret string, accessTTL, refreshTTL, guestTTL time.Duration) (*Service, error) {
	if len(secret) < 32 {
		return nil, errors.New("token: JWT_SECRET debe tener al menos 32 caracteres")
	}
	if accessTTL <= 0 || refreshTTL <= 0 || guestTTL <= 0 {
		return nil, errors.New("token: TTLs deben ser positivos")
	}
	return &Service{
		secret: []byte(secret), accessTTL: accessTTL,
		refreshTTL: refreshTTL, guestTTL: guestTTL,
		now: time.Now,
	}, nil
}

// IssueAccess emite un token de acceso de corta duración con la identidad RBAC.
func (s *Service) IssueAccess(userID string, id Identity) (Issued, error) {
	return s.issue(KindAccess, userID, s.accessTTL, func(c *Claims) {
		c.Role, c.StoreID, c.ProviderID, c.DriverID = id.Role, id.StoreID, id.ProviderID, id.DriverID
	})
}

// IssueRefresh emite un token de refresh (rotación validada en el caso de uso).
func (s *Service) IssueRefresh(userID string) (Issued, error) {
	return s.issue(KindRefresh, userID, s.refreshTTL, nil)
}

// IssueGuest emite un token de invitado limitado a una orden (scope order:<id>).
func (s *Service) IssueGuest(orderID string) (Issued, error) {
	return s.issue(KindGuest, "guest", s.guestTTL, func(c *Claims) {
		c.Scope = "order:" + orderID
	})
}

func (s *Service) issue(kind Kind, subject string, ttl time.Duration, fill func(*Claims)) (Issued, error) {
	now := s.now()
	jti := newJTI()
	claims := &Claims{
		Kind: kind,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    "tiendas-track",
		},
	}
	if fill != nil {
		fill(claims)
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return Issued{}, fmt.Errorf("token: firmando: %w", err)
	}
	return Issued{Raw: raw, JTI: jti, ExpiresAt: now.Add(ttl)}, nil
}

// Parse valida firma, expiración y devuelve los claims.
func (s *Service) Parse(raw string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de firma inesperado: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("token: inválido: %w", err)
	}
	if !tok.Valid {
		return nil, errors.New("token: inválido")
	}
	return claims, nil
}

// TTLs expuestos para respuestas de login.
func (s *Service) AccessTTL() time.Duration  { return s.accessTTL }
func (s *Service) RefreshTTL() time.Duration { return s.refreshTTL }
func (s *Service) GuestTTL() time.Duration   { return s.guestTTL }
