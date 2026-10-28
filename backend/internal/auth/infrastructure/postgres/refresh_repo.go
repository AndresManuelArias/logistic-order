package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tiendas-track/backend/internal/auth/domain"
)

// RefreshTokenRepository implementa domain.RefreshTokenRepository.
type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

// NewRefreshTokenRepository crea el repositorio.
func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

// Save persiste el hash del refresh token.
func (r *RefreshTokenRepository) Save(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO app.refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1::uuid, $2, $3)`,
		userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("refresh_tokens.save: %w", err)
	}
	return nil
}

// FindActiveByHash indica si existe el hash y el token sigue vigente.
func (r *RefreshTokenRepository) FindActiveByHash(ctx context.Context, tokenHash string) (bool, error) {
	var active bool
	err := r.pool.QueryRow(ctx, `
		SELECT NOT revoked AND expires_at > now()
		FROM app.refresh_tokens WHERE token_hash = $1`, tokenHash).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("refresh_tokens.find: %w", err)
	}
	return active, nil
}

// Revoke marca el token como revocado (idempotente).
func (r *RefreshTokenRepository) Revoke(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE app.refresh_tokens SET revoked = true WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("refresh_tokens.revoke: %w", err)
	}
	return nil
}

var _ domain.RefreshTokenRepository = (*RefreshTokenRepository)(nil)
