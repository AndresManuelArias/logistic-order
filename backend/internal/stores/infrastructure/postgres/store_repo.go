// Package postgres: repositorios del módulo stores sobre PostgreSQL/PostGIS.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"tiendas-track/backend/internal/stores/domain"
)

// StoreRepository implementa domain.StoreRepository.
type StoreRepository struct {
	pool *pgxpool.Pool
}

// NewStoreRepository crea el repositorio.
func NewStoreRepository(pool *pgxpool.Pool) *StoreRepository { return &StoreRepository{pool: pool} }

const storeCols = "id::text, name, owner_user_id::text, address, ST_Y(location::geometry), ST_X(location::geometry), created_at"

// Create inserta la tienda (punto PostGIS 4326, ADR-09) y rellena ID y CreatedAt.
func (r *StoreRepository) Create(ctx context.Context, s *domain.Store) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO app.stores (name, owner_user_id, address, location)
		VALUES ($1, $2::uuid, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326))
		RETURNING id::text, created_at`,
		s.Name, s.OwnerUserID, s.Address, s.Lon, s.Lat,
	).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		// Viola la unicidad de owner_user_id (migración 0006): carrera entre dos
		// altas simultáneas. El nombre del índice aparece en ConstraintName.
		if isUniqueViolation(err, "idx_stores_owner_unique") {
			return fmt.Errorf("%w: %s", domain.ErrAlreadyExists, err)
		}
		return fmt.Errorf("stores.create: %w", err)
	}
	return nil
}

// FindByOwner devuelve la tienda del usuario (ErrNotFound si no existe).
func (r *StoreRepository) FindByOwner(ctx context.Context, userID string) (*domain.Store, error) {
	var s domain.Store
	err := r.pool.QueryRow(ctx, `
		SELECT `+storeCols+` FROM app.stores WHERE owner_user_id = $1::uuid LIMIT 1`, userID).
		Scan(&s.ID, &s.Name, &s.OwnerUserID, &s.Address, &s.Lat, &s.Lon, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stores.find_by_owner: %w", err)
	}
	return &s, nil
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		(constraint == "" || strings.Contains(pgErr.ConstraintName, constraint))
}
