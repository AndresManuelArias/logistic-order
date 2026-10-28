package domain

import (
	"context"
	"time"
)

// UserRepository persiste usuarios.
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	// CreateWithStore crea el usuario y su tienda en la misma transacción
	// (registro de rol STORE): o ambas existen, o ninguna.
	// El storeID resultante se devuelve; profile nunca puede ser nil.
	CreateWithStore(ctx context.Context, u *User, profile *StoreProfile) (storeID string, err error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id string) (*User, error)
}

// RefreshTokenRepository gestiona refresh tokens rotativos (hash, nunca en claro).
type RefreshTokenRepository interface {
	// Save persiste el hash del refresh token.
	Save(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	// FindActiveByHash devuelve el token si existe, no está revocado ni expirado.
	FindActiveByHash(ctx context.Context, tokenHash string) (exists bool, err error)
	// Revoke revoca el token dado.
	Revoke(ctx context.Context, tokenHash string) error
}

// GuestTokenRepository registra tokens de invitado emitidos (auditoría + revocación).
type GuestTokenRepository interface {
	Save(ctx context.Context, orderID, tokenHash string, expiresAt time.Time) error
}

// IdentityRepository resuelve la identidad RBAC almacenada (scope de datos).
type IdentityRepository interface {
	// FindStoreIDByOwner devuelve la tienda cuyo dueño es el usuario (rol STORE).
	FindStoreIDByOwner(ctx context.Context, userID string) (string, error)
	// FindDriverIdentity devuelve el driver y su vehículo asignado (rol DRIVER).
	FindDriverIdentity(ctx context.Context, userID string) (driverID string, vehicleID string, err error)
}
