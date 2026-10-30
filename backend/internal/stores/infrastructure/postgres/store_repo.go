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

func (r *StoreRepository) List(ctx context.Context, f domain.ListFilter) ([]domain.Store, int, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	// La consulta siempre pasa por provider_store_agreements para asegurar
	// que el proveedor solo ve sus tiendas con acuerdo ACTIVE (RF-02).
	// El filtro de nombre es opcional y se aplica como predicado adicional.
	//
	// Estructura de parámetros:
	//   $1 = provider_id  (siempre presente)
	//   $2 = nombre LIKE  (solo cuando f.Name != "")
	//   $N-1 = limit
	//   $N   = offset
	const baseFrom = `
		FROM app.provider_store_agreements psa
		JOIN app.stores s ON s.id = psa.store_id
		WHERE psa.provider_id = $1::uuid
		  AND psa.status = 'ACTIVE'`

	var (
		extraWhere string
		args       []any
	)
	args = append(args, f.ProviderID)

	if strings.TrimSpace(f.Name) != "" {
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(f.Name))+"%")
		extraWhere = fmt.Sprintf(" AND lower(s.name) LIKE $%d", len(args))
	}

	// COUNT total antes de paginar.
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*)`+baseFrom+extraWhere, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("stores.list.count: %w", err)
	}

	// Página de resultados.
	args = append(args, limit, offset)
	limitArg := fmt.Sprintf("$%d", len(args)-1)
	offsetArg := fmt.Sprintf("$%d", len(args))

	const storePrefixed = `s.id::text, s.name, s.owner_user_id::text, s.address,
		ST_Y(s.location::geometry), ST_X(s.location::geometry), s.created_at`

	rows, err := r.pool.Query(ctx,
		`SELECT `+storePrefixed+baseFrom+extraWhere+
			` ORDER BY s.name LIMIT `+limitArg+` OFFSET `+offsetArg,
		args...)
	if err != nil {
		return nil, 0, fmt.Errorf("stores.list: %w", err)
	}
	defer rows.Close()

	stores := make([]domain.Store, 0, limit)
	for rows.Next() {
		var s domain.Store
		if err := rows.Scan(&s.ID, &s.Name, &s.OwnerUserID, &s.Address, &s.Lat, &s.Lon, &s.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("stores.list.scan: %w", err)
		}
		stores = append(stores, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("stores.list.rows: %w", err)
	}
	return stores, total, nil
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		(constraint == "" || strings.Contains(pgErr.ConstraintName, constraint))
}
