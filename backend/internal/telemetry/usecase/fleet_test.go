package usecase

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/resilience"
	"tiendas-track/backend/internal/telemetry/domain"
)

// fakeFleet registra el alcance recibido para comprobar que el RBAC se aplica
// antes de tocar la base de datos.
type fakeFleet struct {
	mu       sync.Mutex
	calls    int
	scope    domain.FleetScope
	limit    int
	offset   int
	vehicles []domain.Vehicle
	total    int
	err      error
}

func (f *fakeFleet) ListVehicles(_ context.Context, scope domain.FleetScope, limit, offset int) ([]domain.Vehicle, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.scope, f.limit, f.offset = scope, limit, offset
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.vehicles, f.total, nil
}

func newFleetUC(repo domain.FleetRepository) *FleetUseCase {
	return NewFleet(repo, resilience.New(resilience.DefaultSettings("test-fleet")),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// Cada rol consulta solo su columna de alcance; los identificadores ajenos
// nunca llegan al repositorio.
func TestFleetListScopePorRol(t *testing.T) {
	cases := []struct {
		name  string
		p     authz.Principal
		check func(t *testing.T, s domain.FleetScope)
	}{
		{
			name: "PROVIDER",
			p:    authz.Principal{Role: authz.RoleProvider, ProviderID: "prov-1", StoreID: "otra", DriverID: "otro"},
			check: func(t *testing.T, s domain.FleetScope) {
				if s.ProviderID != "prov-1" || s.StoreID != "" || s.DriverID != "" {
					t.Fatalf("alcance provider = %+v", s)
				}
			},
		},
		{
			name: "STORE",
			p:    authz.Principal{Role: authz.RoleStore, ProviderID: "otro", StoreID: "store-1", DriverID: "otro"},
			check: func(t *testing.T, s domain.FleetScope) {
				if s.StoreID != "store-1" || s.ProviderID != "" || s.DriverID != "" {
					t.Fatalf("alcance store = %+v", s)
				}
			},
		},
		{
			name: "DRIVER",
			p:    authz.Principal{Role: authz.RoleDriver, ProviderID: "otro", StoreID: "otro", DriverID: "drv-1"},
			check: func(t *testing.T, s domain.FleetScope) {
				if s.DriverID != "drv-1" || s.ProviderID != "" || s.StoreID != "" {
					t.Fatalf("alcance driver = %+v", s)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeFleet{vehicles: []domain.Vehicle{{ID: "v1", Plate: "ABC123", Status: "ACTIVE"}}, total: 1}
			got, total, err := newFleetUC(repo).List(context.Background(), c.p, 0, 0)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != 1 || total != 1 {
				t.Fatalf("resultado = %d vehículos, total %d", len(got), total)
			}
			c.check(t, repo.scope)
			if repo.limit != DefaultVehicleLimit {
				t.Errorf("limit por defecto = %d, se esperaba %d", repo.limit, DefaultVehicleLimit)
			}
			if repo.offset != 0 {
				t.Errorf("offset = %d", repo.offset)
			}
		})
	}
}

// El invitado no tiene flota: 403 sin consultar la BD.
func TestFleetListInvitadoDenegado(t *testing.T) {
	repo := &fakeFleet{}
	_, _, err := newFleetUC(repo).List(context.Background(),
		authz.Principal{Role: authz.RoleStore, TokenType: authz.GuestToken, Scope: "order:x"}, 0, 0)
	if !errors.Is(err, domain.ErrForbiddenScope) {
		t.Fatalf("err = %v, se esperaba ErrForbiddenScope", err)
	}
	if repo.calls != 0 {
		t.Errorf("el repositorio no debe consultarse para un invitado (calls=%d)", repo.calls)
	}
}

// Límites de paginación: negativos o fuera de rango → 422.
func TestFleetListPaginacionInvalida(t *testing.T) {
	cases := []struct{ limit, offset int }{
		{-1, 0},
		{MaxVehicleLimit + 1, 0},
		{10, -1},
	}
	for _, c := range cases {
		repo := &fakeFleet{}
		_, _, err := newFleetUC(repo).List(context.Background(),
			authz.Principal{Role: authz.RoleProvider, ProviderID: "p"}, c.limit, c.offset)
		if !errors.Is(err, domain.ErrValidation) {
			t.Errorf("limit=%d offset=%d: err = %v, se esperaba ErrValidation", c.limit, c.offset, err)
		}
		if repo.calls != 0 {
			t.Errorf("limit=%d offset=%d: no debe consultarse la BD", c.limit, c.offset)
		}
	}
}

// Un rol desconocido no recibe datos (defensa en profundidad).
func TestFleetListRolDesconocido(t *testing.T) {
	repo := &fakeFleet{}
	_, _, err := newFleetUC(repo).List(context.Background(), authz.Principal{Role: "AUDITOR"}, 0, 0)
	if !errors.Is(err, domain.ErrForbiddenScope) {
		t.Fatalf("err = %v, se esperaba ErrForbiddenScope", err)
	}
	if repo.calls != 0 {
		t.Errorf("calls = %d", repo.calls)
	}
}

// Los errores del repositorio se propagan sin transformarlos.
func TestFleetListErrorRepositorio(t *testing.T) {
	boom := errors.New("boom")
	_, _, err := newFleetUC(&fakeFleet{err: boom}).List(context.Background(),
		authz.Principal{Role: authz.RoleProvider, ProviderID: "p"}, 10, 20)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, se esperaba boom", err)
	}
}

// STORE sin tienda no es un error de autorización: lista vacía.
func TestFleetListStoreSinTiendaListaVacia(t *testing.T) {
	repo := &fakeFleet{vehicles: []domain.Vehicle{}, total: 0}
	got, total, err := newFleetUC(repo).List(context.Background(),
		authz.Principal{Role: authz.RoleStore}, 0, 0)
	if err != nil || total != 0 || len(got) != 0 {
		t.Fatalf("resultado = %v, %d, %v", got, total, err)
	}
	if repo.calls != 1 {
		t.Errorf("calls = %d", repo.calls)
	}
}
