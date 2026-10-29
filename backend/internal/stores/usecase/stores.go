// Package usecase: casos de uso del alta y consulta de tiendas (RF-02).
// El alta de tienda es un paso propio del registro (una tienda por dueño).
package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/stores/domain"
	"tiendas-track/backend/internal/tracing"
)

// StoresUseCase orquesta el alta y la consulta de la tienda del tendero.
type StoresUseCase struct {
	stores domain.StoreRepository
	log    *slog.Logger
}

// New crea el caso de uso.
func New(repo domain.StoreRepository, log *slog.Logger) *StoresUseCase {
	return &StoresUseCase{stores: repo, log: log}
}

// CreateInput es la entrada de alta de tienda.
type CreateInput struct {
	Name    string
	Address string
	Lat     float64
	Lon     float64
}

// Create da de alta la tienda del usuario con rol STORE.
// Reglas: solo rol STORE, una tienda por dueño (409) y coordenadas válidas (ADR-09).
func (uc *StoresUseCase) Create(ctx context.Context, p authz.Principal, in CreateInput) (*domain.Store, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("Create: iniciando alta de tienda",
		"trace_id", traceID,
		"user_id", p.UserID,
		"role", p.Role,
	)

	if p.IsGuest() || !p.HasRole(authz.RoleStore) {
		uc.log.Error("Create: rol sin permiso para operar tiendas",
			"trace_id", traceID,
			"role", p.Role,
		)
		return nil, domain.ErrForbiddenScope
	}

	s := &domain.Store{
		Name:        strings.TrimSpace(in.Name),
		Address:     strings.TrimSpace(in.Address),
		OwnerUserID: p.UserID,
		Lat:         in.Lat,
		Lon:         in.Lon,
	}
	if !s.Valid() {
		uc.log.Error("Create: datos de tienda inválidos",
			"trace_id", traceID,
			"name", s.Name,
		)
		return nil, fmt.Errorf("%w: la tienda requiere nombre y coordenadas válidas (PostGIS, ADR-09)", domain.ErrValidation)
	}

	if existing, err := uc.stores.FindByOwner(ctx, p.UserID); err == nil && existing != nil {
		uc.log.Error("Create: el usuario ya tiene tienda",
			"trace_id", traceID,
			"user_id", p.UserID,
		)
		return nil, domain.ErrAlreadyExists
	}

	if err := uc.stores.Create(ctx, s); err != nil {
		uc.log.Error("Create: error creando tienda",
			"trace_id", traceID,
			"error", err.Error(),
		)
		return nil, err
	}

	uc.log.Info("Create: tienda creada exitosamente",
		"trace_id", traceID,
		"store_id", s.ID,
		"user_id", p.UserID,
	)
	return s, nil
}

// ListInput son los parámetros de la consulta de tiendas disponibles.
type ListInput struct {
	Name   string
	Limit  int
	Offset int
}

// List devuelve la lista paginada de tiendas disponibles.
// Cualquier usuario autenticado puede consultarla (PROVIDER, STORE, DRIVER).
// Los invitados (guest tokens) no tienen acceso.
func (uc *StoresUseCase) List(ctx context.Context, p authz.Principal, in ListInput) ([]domain.Store, int, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("List: consultando tiendas",
		"trace_id", traceID,
		"user_id", p.UserID,
		"role", p.Role,
		"name_filter", in.Name,
		"limit", in.Limit,
		"offset", in.Offset,
	)

	if p.IsGuest() {
		uc.log.Error("List: invitado sin acceso a listado de tiendas", "trace_id", traceID)
		return nil, 0, domain.ErrForbiddenScope
	}

	stores, total, err := uc.stores.List(ctx, domain.ListFilter{
		Name:   in.Name,
		Limit:  in.Limit,
		Offset: in.Offset,
	})
	if err != nil {
		uc.log.Error("List: error consultando tiendas",
			"trace_id", traceID,
			"error", err.Error(),
		)
		return nil, 0, err
	}

	uc.log.Info("List: consulta completada",
		"trace_id", traceID,
		"total", total,
		"count", len(stores),
	)
	return stores, total, nil
}

func (uc *StoresUseCase) Me(ctx context.Context, p authz.Principal) (*domain.Store, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)
	uc.log.Info("Me: consultando tienda del usuario",
		"trace_id", traceID,
		"user_id", p.UserID,
		"role", p.Role,
	)

	if p.IsGuest() || !p.HasRole(authz.RoleStore) {
		uc.log.Error("Me: rol sin permiso para operar tiendas",
			"trace_id", traceID,
			"role", p.Role,
		)
		return nil, domain.ErrForbiddenScope
	}

	s, err := uc.stores.FindByOwner(ctx, p.UserID)
	if err != nil {
		uc.log.Error("Me: error consultando tienda",
			"trace_id", traceID,
			"error", err.Error(),
		)
		return nil, err
	}
	return s, nil
}
