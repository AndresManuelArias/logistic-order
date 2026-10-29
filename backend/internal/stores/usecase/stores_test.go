package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/stores/domain"
)

// --- Fakes -----------------------------------------------------------------

// fakeStoreRepo guarda en memoria y replica las reglas de dominio relevantes.
type fakeStoreRepo struct {
	byOwner map[string]*domain.Store
}

func newFake() *fakeStoreRepo { return &fakeStoreRepo{byOwner: map[string]*domain.Store{}} }

func (f *fakeStoreRepo) Create(_ context.Context, s *domain.Store) error {
	if _, dup := f.byOwner[s.OwnerUserID]; dup {
		return domain.ErrAlreadyExists
	}
	s.ID = "store-1"
	f.byOwner[s.OwnerUserID] = s
	return nil
}

func (f *fakeStoreRepo) FindByOwner(_ context.Context, userID string) (*domain.Store, error) {
	s, ok := f.byOwner[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return s, nil
}

func (f *fakeStoreRepo) List(_ context.Context, limit int) ([]domain.Store, error) {
	stores := make([]domain.Store, 0, len(f.byOwner))
	for _, store := range f.byOwner {
		stores = append(stores, *store)
		if len(stores) == limit {
			break
		}
	}
	return stores, nil
}

// nullWriter descarta la salida del logger en tests.
type nullWriter struct{}

func (nullWriter) Write(p []byte) (n int, err error) { return len(p), nil }

func setup(t *testing.T) (*StoresUseCase, *fakeStoreRepo) {
	t.Helper()
	repo := newFake()
	log := slog.New(slog.NewTextHandler(&nullWriter{}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return New(repo, log), repo
}

func storePrincipal() authz.Principal {
	return authz.Principal{UserID: "u-1", Role: authz.RoleStore, TokenType: authz.AccessToken}
}

// --- Tests -----------------------------------------------------------------

// Camino feliz: STORE crea su tienda (201) y GET /me la devuelve.
func TestCreateYMeHappyPath(t *testing.T) {
	uc, _ := setup(t)

	s, err := uc.Create(context.Background(), storePrincipal(), CreateInput{
		Name: "La Esquina", Address: "Calle 1 # 2-3", Lat: 4.65, Lon: -74.05,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.ID == "" || s.OwnerUserID != "u-1" {
		t.Errorf("tienda creada mal: %+v", s)
	}

	got, err := uc.Me(context.Background(), storePrincipal())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if got.Name != "La Esquina" {
		t.Errorf("Me.Name = %q", got.Name)
	}
}

// Doble alta → ErrAlreadyExists (409): una tienda por dueño.
func TestCreateDuplicado(t *testing.T) {
	uc, _ := setup(t)
	in := CreateInput{Name: "La Esquina", Lat: 4.65, Lon: -74.05}

	if _, err := uc.Create(context.Background(), storePrincipal(), in); err != nil {
		t.Fatalf("primer alta: %v", err)
	}
	if _, err := uc.Create(context.Background(), storePrincipal(), in); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Errorf("segunda alta: err = %v, esperado ErrAlreadyExists", err)
	}
}

// Rol no STORE (o invitado) → ErrForbiddenScope (403).
func TestCreateRolNoStore(t *testing.T) {
	uc, _ := setup(t)
	in := CreateInput{Name: "La Esquina", Lat: 4.65, Lon: -74.05}

	cases := []struct {
		name string
		p    authz.Principal
	}{
		{"DRIVER", authz.Principal{UserID: "u-2", Role: authz.RoleDriver, TokenType: authz.AccessToken}},
		{"PROVIDER", authz.Principal{UserID: "u-3", Role: authz.RoleProvider, TokenType: authz.AccessToken}},
		{"invitado", authz.Principal{TokenType: authz.GuestToken, Scope: "order:o-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := uc.Create(context.Background(), tc.p, in); !errors.Is(err, domain.ErrForbiddenScope) {
				t.Errorf("err = %v, esperado ErrForbiddenScope", err)
			}
			if _, err := uc.Me(context.Background(), tc.p); !errors.Is(err, domain.ErrForbiddenScope) {
				t.Errorf("Me err = %v, esperado ErrForbiddenScope", err)
			}
		})
	}
}

// Coordenadas inválidas (fuera de rango o 0,0) y nombre vacío → ErrValidation (422).
func TestCreateValidacion(t *testing.T) {
	uc, _ := setup(t)

	cases := []struct {
		name string
		in   CreateInput
	}{
		{"coordenada nula", CreateInput{Name: "X", Lat: 0, Lon: 0}},
		{"lat fuera de rango", CreateInput{Name: "X", Lat: 91, Lon: -74}},
		{"lon fuera de rango", CreateInput{Name: "X", Lat: 4, Lon: -181}},
		{"sin nombre", CreateInput{Name: "  ", Lat: 4.65, Lon: -74.05}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := uc.Create(context.Background(), storePrincipal(), tc.in); !errors.Is(err, domain.ErrValidation) {
				t.Errorf("err = %v, esperado ErrValidation", err)
			}
		})
	}
}

// Me sin tienda → ErrNotFound (404): el alta sigue pendiente.
func TestMeSinTienda(t *testing.T) {
	uc, _ := setup(t)
	if _, err := uc.Me(context.Background(), storePrincipal()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, esperado ErrNotFound", err)
	}
}
