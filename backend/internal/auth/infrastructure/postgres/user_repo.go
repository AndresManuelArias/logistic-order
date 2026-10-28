// Package postgres: repositorios del módulo auth sobre PostgreSQL/TimescaleDB.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tiendas-track/backend/internal/auth/domain"
)

// UserRepository implementa domain.UserRepository e IdentityRepository.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository crea el repositorio.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository { return &UserRepository{pool: pool} }

const userCols = "id::text, email, password_hash, full_name, role::text, status::text, created_at, updated_at"

// Create inserta un usuario y rellena ID y timestamps.
func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO app.users (email, password_hash, full_name, role)
		VALUES ($1, $2, $3, $4::app.role_enum)
		RETURNING id::text, created_at, updated_at`,
		u.Email, u.PasswordHash, u.FullName, u.Role,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return fmt.Errorf("users.create: %w", err)
	}
	return nil
}

// FindByEmail busca por email (case-insensitive: el caso se normaliza al llamar).
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.single(ctx, `SELECT `+userCols+` FROM app.users WHERE email = $1`, email)
}

// FindByID busca por ID.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	return r.single(ctx, `SELECT `+userCols+` FROM app.users WHERE id = $1::uuid`, id)
}

func (r *UserRepository) single(ctx context.Context, q string, arg any) (*domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx, q, arg).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("users.find: %w", err)
	}
	return &u, nil
}

// FindStoreIDByOwner devuelve la tienda del usuario (scope del rol STORE).
func (r *UserRepository) FindStoreIDByOwner(ctx context.Context, userID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text FROM app.stores WHERE owner_user_id = $1::uuid LIMIT 1`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// FindDriverIdentity devuelve el driver y su vehículo (scope del rol DRIVER).
func (r *UserRepository) FindDriverIdentity(ctx context.Context, userID string) (string, string, error) {
	var driverID, vehicleID string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, vehicle_id::text FROM app.drivers WHERE user_id = $1::uuid LIMIT 1`, userID).
		Scan(&driverID, &vehicleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	return driverID, vehicleID, err
}

var (
	_ domain.UserRepository     = (*UserRepository)(nil)
	_ domain.IdentityRepository = (*UserRepository)(nil)
)
