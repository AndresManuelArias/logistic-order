package domain

import "context"

// ListFilter acota la consulta de tiendas.
type ListFilter struct {
	// ProviderID restringe la consulta a las tiendas con acuerdo activo con
	// ese proveedor (JOIN con app.provider_store_agreements).
	ProviderID string
	// Name filtra por nombre (búsqueda parcial, case-insensitive). Vacío = sin filtro.
	Name   string
	Limit  int
	Offset int
}

// StoreRepository persiste y consulta tiendas.
type StoreRepository interface {
	// Create inserta la tienda y rellena ID y CreatedAt.
	Create(ctx context.Context, s *Store) error
	// FindByOwner devuelve la tienda del usuario (ErrNotFound si no existe).
	// Es el punto de lookup del scope del rol STORE.
	FindByOwner(ctx context.Context, userID string) (*Store, error)
	// List devuelve la página de tiendas y el total antes de paginar.
	List(ctx context.Context, f ListFilter) ([]Store, int, error)
}
