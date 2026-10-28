package domain

import "context"

// StoreRepository persiste y consulta tiendas.
type StoreRepository interface {
	// Create inserta la tienda y rellena ID y CreatedAt.
	Create(ctx context.Context, s *Store) error
	// FindByOwner devuelve la tienda del usuario (ErrNotFound si no existe).
	// Es el punto de lookup del scope del rol STORE.
	FindByOwner(ctx context.Context, userID string) (*Store, error)
}
