package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"tiendas-track/backend/internal/auth/domain"
)

// GuestTokenRepository implementa domain.GuestTokenRepository.
type GuestTokenRepository struct {
	pool *pgxpool.Pool
}

// NewGuestTokenRepository crea el repositorio.
func NewGuestTokenRepository(pool *pgxpool.Pool) *GuestTokenRepository {
	return &GuestTokenRepository{pool: pool}
}

// Save registra un token de invitado emitido (auditoría y revocación).
func (r *GuestTokenRepository) Save(ctx context.Context, orderID, tokenHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO app.guest_tokens (order_id, token_hash, expires_at)
		VALUES ($1::uuid, $2, $3)`,
		orderID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("guest_tokens.save: %w", err)
	}
	return nil
}

var _ domain.GuestTokenRepository = (*GuestTokenRepository)(nil)
