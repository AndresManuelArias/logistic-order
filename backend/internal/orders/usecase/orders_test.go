package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"tiendas-track/backend/internal/orders/domain"
	"tiendas-track/backend/internal/shared/authz"
)

// nullWriter descarta la salida del logger en tests.
type nullWriter struct{}

func (nullWriter) Write(p []byte) (n int, err error) { return len(p), nil }

var testLog = slog.New(slog.NewTextHandler(&nullWriter{}, &slog.HandlerOptions{Level: slog.LevelDebug}))

// fakeOrders implementa OrderRepository con memoria y scope observable.
type fakeOrders struct {
	byID             map[string]*domain.Order
	stores           map[string]bool
	vehicleProviders map[string]string // vehicleID -> providerID
	driverVehicles   map[string]string // driverID -> vehicleID
	lastFilter       domain.ListFilter
	statusEvents     []domain.OrderEvent
	assignCalls      int
}

func newFakeOrders() *fakeOrders {
	return &fakeOrders{
		byID:             map[string]*domain.Order{},
		stores:           map[string]bool{},
		vehicleProviders: map[string]string{},
		driverVehicles:   map[string]string{},
	}
}

func (f *fakeOrders) Create(_ context.Context, o *domain.Order) error {
	for _, e := range f.byID {
		if e.ExternalRef == o.ExternalRef {
			return domain.ErrExternalRefConflict
		}
	}
	o.ID = "order-" + o.ExternalRef
	f.byID[o.ID] = o
	return nil
}
func (f *fakeOrders) FindByID(_ context.Context, id string) (*domain.Order, error) {
	o, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *o
	return &cp, nil
}
func (f *fakeOrders) List(_ context.Context, flt domain.ListFilter) ([]domain.Order, int, error) {
	f.lastFilter = flt
	var out []domain.Order
	for _, o := range f.byID {
		// Replica el scope mínimo de los tests.
		if flt.StoreID != "" && o.StoreID != flt.StoreID {
			continue
		}
		if flt.DriverID != "" && (o.AssignedDriverID == nil || *o.AssignedDriverID != flt.DriverID) {
			continue
		}
		out = append(out, *o)
	}
	return out, len(out), nil
}
func (f *fakeOrders) UpdateStatus(_ context.Context, o *domain.Order, ev domain.OrderEvent) error {
	f.statusEvents = append(f.statusEvents, ev)
	f.byID[o.ID] = o
	return nil
}
func (f *fakeOrders) Assign(_ context.Context, o *domain.Order, vehicleID, driverID string, ev domain.OrderEvent) error {
	f.assignCalls++
	f.statusEvents = append(f.statusEvents, ev)
	o.Assign(vehicleID, driverID)
	f.byID[o.ID] = o
	return nil
}
func (f *fakeOrders) VehicleBelongsToProvider(_ context.Context, vehicleID, providerID string) (bool, error) {
	return f.vehicleProviders[vehicleID] == providerID && providerID != "", nil
}
func (f *fakeOrders) DriverBelongsToVehicle(_ context.Context, driverID, vehicleID string) (bool, error) {
	return f.driverVehicles[driverID] == vehicleID, nil
}
func (f *fakeOrders) StoreExists(_ context.Context, storeID string) (bool, error) {
	return f.stores[storeID], nil
}

// --- Tests -----------------------------------------------------------------

func storePrincipal(storeID string) authz.Principal {
	return authz.Principal{UserID: "u1", Role: authz.RoleStore, TokenType: authz.AccessToken, StoreID: storeID}
}

func validInput(ref, storeID string) CreateOrderInput {
	return CreateOrderInput{
		ExternalRef: ref, StoreID: storeID,
		Origin:      domain.Point{Lat: 4.65, Lon: -74.05},
		Destination: domain.Point{Lat: 4.67, Lon: -74.06},
	}
}

// El rol STORE solo puede crear órdenes de SU tienda (aislamiento RF-02).
func TestCreateAislamientoPorTienda(t *testing.T) {
	f := newFakeOrders()
	f.stores["store-a"] = true
	uc := New(f, testLog)
	ctx := context.Background()

	if _, err := uc.Create(ctx, storePrincipal("store-a"), validInput("REF-1", "store-a")); err != nil {
		t.Fatalf("crear orden propia: %v", err)
	}

	// Tienda B intenta crear una orden atribuida a la tienda A → 403 de scope.
	if _, err := uc.Create(ctx, storePrincipal("store-b"), validInput("REF-2", "store-a")); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Errorf("err = %v, esperado ErrForbiddenScope", err)
	}
}

// Un invitado y un conductor no pueden crear órdenes.
func TestCreateRolesNoPermitidos(t *testing.T) {
	uc := New(newFakeOrders(), testLog)
	ctx := context.Background()
	guest := authz.Principal{TokenType: authz.GuestToken, Scope: "order:x"}
	driver := authz.Principal{TokenType: authz.AccessToken, Role: authz.RoleDriver, DriverID: "d1"}

	if _, err := uc.Create(ctx, guest, validInput("R1", "")); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Errorf("invitado: err = %v", err)
	}
	if _, err := uc.Create(ctx, driver, validInput("R2", "")); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Errorf("conductor: err = %v", err)
	}
}

func TestCreateProvider(t *testing.T) {
	f := newFakeOrders()
	f.stores["store-a"] = true
	uc := New(f, testLog)
	provider := authz.Principal{Role: authz.RoleProvider, TokenType: authz.AccessToken, ProviderID: "provider-a"}

	if _, err := uc.Create(context.Background(), provider, validInput("PROVIDER-REF", "store-a")); err != nil {
		t.Fatalf("crear orden como proveedor: %v", err)
	}

	provider.ProviderID = ""
	if _, err := uc.Create(context.Background(), provider, validInput("PROVIDER-REF-2", "store-a")); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Errorf("proveedor sin scope: err = %v, esperado ErrForbiddenScope", err)
	}
}

// Coordenadas inválidas o external_ref vacío → error de validación.
func TestCreateValidacion(t *testing.T) {
	f := newFakeOrders()
	f.stores["s1"] = true
	uc := New(f, testLog)
	p := storePrincipal("s1")
	ctx := context.Background()

	in := validInput("", "s1")
	if _, err := uc.Create(ctx, p, in); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("sin ref: err = %v", err)
	}
	in = validInput("R", "s1")
	in.Origin = domain.Point{Lat: 0, Lon: 0}
	if _, err := uc.Create(ctx, p, in); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("coordenada nula: err = %v", err)
	}
}

// external_ref duplicado se traduce a conflicto de idempotencia.
func TestCreateDuplicado(t *testing.T) {
	f := newFakeOrders()
	f.stores["s1"] = true
	uc := New(f, testLog)
	p := storePrincipal("s1")
	ctx := context.Background()
	if _, err := uc.Create(ctx, p, validInput("DUP", "s1")); err != nil {
		t.Fatalf("primera: %v", err)
	}
	if _, err := uc.Create(ctx, p, validInput("DUP", "s1")); !errors.Is(err, domain.ErrExternalRefConflict) {
		t.Errorf("err = %v, esperado ErrExternalRefConflict", err)
	}
}

// Get: cada rol solo ve sus órdenes; fuera de alcance → ErrForbiddenScope.
func TestGetAislamientoPorRol(t *testing.T) {
	f := newFakeOrders()
	f.stores["s1"] = true
	f.vehicleProviders["veh-1"] = "prov-1"
	uc := New(f, testLog)
	ctx := context.Background()

	created, err := uc.Create(ctx, storePrincipal("s1"), validInput("G1", "s1"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	cases := []struct {
		name string
		p    authz.Principal
		ok   bool
	}{
		{"dueña", storePrincipal("s1"), true},
		{"tienda ajena", storePrincipal("s2"), false},
		{"conductor no asignado", authz.Principal{Role: authz.RoleDriver, TokenType: authz.AccessToken, DriverID: "d9"}, false},
		{"conductor asignado", authz.Principal{Role: authz.RoleDriver, TokenType: authz.AccessToken, DriverID: "d1"}, true},
		{"proveedor dueño del vehículo", authz.Principal{Role: authz.RoleProvider, TokenType: authz.AccessToken, ProviderID: "prov-1"}, true},
		{"proveedor ajeno", authz.Principal{Role: authz.RoleProvider, TokenType: authz.AccessToken, ProviderID: "prov-2"}, false},
		{"invitado de esta orden", authz.Principal{TokenType: authz.GuestToken, Scope: "order:" + created.ID}, true},
		{"invitado de otra orden", authz.Principal{TokenType: authz.GuestToken, Scope: "order:otra"}, false},
	}
	// Prepara asignación para conductor/vehículo.
	created.Assign("veh-1", "d1")
	f.byID[created.ID] = created

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := uc.Get(ctx, tc.p, created.ID)
			if tc.ok && err != nil {
				t.Errorf("err = %v, esperaba acceso", err)
			}
			if !tc.ok && !errors.Is(err, domain.ErrForbiddenScope) {
				t.Errorf("err = %v, esperaba ErrForbiddenScope", err)
			}
		})
	}
}

// List siempre envía el scope del rol al repositorio (nunca sin filtro).
func TestListAplicaScopeDelRol(t *testing.T) {
	f := newFakeOrders()
	uc := New(f, testLog)
	ctx := context.Background()

	if _, _, err := uc.List(ctx, storePrincipal("s1"), nil, 0, 0); err != nil {
		t.Fatalf("list store: %v", err)
	}
	if f.lastFilter.StoreID != "s1" || f.lastFilter.DriverID != "" || f.lastFilter.ProviderID != "" {
		t.Errorf("filter = %+v, esperado StoreID=s1", f.lastFilter)
	}

	driver := authz.Principal{Role: authz.RoleDriver, TokenType: authz.AccessToken, DriverID: "d7"}
	if _, _, err := uc.List(ctx, driver, nil, 0, 0); err != nil {
		t.Fatalf("list driver: %v", err)
	}
	if f.lastFilter.DriverID != "d7" || f.lastFilter.StoreID != "" {
		t.Errorf("filter = %+v, esperado DriverID=d7", f.lastFilter)
	}

	prov := authz.Principal{Role: authz.RoleProvider, TokenType: authz.AccessToken, ProviderID: "p1"}
	if _, _, err := uc.List(ctx, prov, nil, 0, 0); err != nil {
		t.Fatalf("list provider: %v", err)
	}
	if f.lastFilter.ProviderID != "p1" {
		t.Errorf("filter = %+v, esperado ProviderID=p1", f.lastFilter)
	}

	// Un invitado no puede listar.
	if _, _, err := uc.List(ctx, authz.Principal{TokenType: authz.GuestToken}, nil, 0, 0); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Errorf("invitado list: err = %v", err)
	}

	// Scope vacío = sin identidad de datos: listar sin filtro rompería el
	// aislamiento (un STORE sin tienda vería todas las órdenes).
	noScope := []authz.Principal{
		{Role: authz.RoleStore, TokenType: authz.AccessToken},    // STORE sin tienda
		{Role: authz.RoleDriver, TokenType: authz.AccessToken},   // DRIVER sin driver_id
		{Role: authz.RoleProvider, TokenType: authz.AccessToken}, // PROVIDER sin provider_id
	}
	for _, p := range noScope {
		if _, _, err := uc.List(ctx, p, nil, 0, 0); !errors.Is(err, domain.ErrForbiddenScope) {
			t.Errorf("%s sin scope: err = %v, esperado ErrForbiddenScope", p.Role, err)
		}
	}
}

// Solo el conductor asignado o la tienda dueña transicionan; y solo si es legal.
func TestChangeStatusAutorizacionYTransicion(t *testing.T) {
	f := newFakeOrders()
	f.stores["s1"] = true
	uc := New(f, testLog)
	ctx := context.Background()

	o, err := uc.Create(ctx, storePrincipal("s1"), validInput("S1", "s1"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	o.Assign("veh-1", "d1")
	f.byID[o.ID] = o

	// Tienda ajena → 403.
	if _, err := uc.ChangeStatus(ctx, storePrincipal("s2"), o.ID, domain.StatusInTransit); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Errorf("tienda ajena: err = %v", err)
	}
	// Transición ilegal → ErrInvalidTransition.
	if _, err := uc.ChangeStatus(ctx, storePrincipal("s1"), o.ID, domain.StatusDelivered); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("CREATED→DELIVERED: err = %v", err)
	}
	// Estado desconocido → validación.
	if _, err := uc.ChangeStatus(ctx, storePrincipal("s1"), o.ID, domain.Status("PAID")); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("estado desconocido: err = %v", err)
	}

	// Transición legal: la tienda dueña mueve a IN_TRANSIT y genera evento.
	got, err := uc.ChangeStatus(ctx, storePrincipal("s1"), o.ID, domain.StatusInTransit)
	if err != nil {
		t.Fatalf("CREATED→IN_TRANSIT: %v", err)
	}
	if got.Status != domain.StatusInTransit {
		t.Errorf("status = %s", got.Status)
	}
	if len(f.statusEvents) != 1 {
		t.Fatalf("eventos = %d, esperado 1", len(f.statusEvents))
	}
	if f.statusEvents[0].ActorUserID == nil || *f.statusEvents[0].ActorUserID != "u1" {
		t.Error("el evento debe registrar el actor")
	}

	// Un invitado nunca transiciona.
	if _, err := uc.ChangeStatus(ctx, authz.Principal{TokenType: authz.GuestToken, Scope: "order:" + o.ID}, o.ID, domain.StatusDelivered); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Errorf("invitado: err = %v", err)
	}
}

// Asignación: solo PROVIDER, y solo vehículos de su flota con su conductor.
func TestAssignScopeDeFlota(t *testing.T) {
	f := newFakeOrders()
	f.stores["s1"] = true
	f.vehicleProviders["veh-mio"] = "prov-1"
	f.vehicleProviders["veh-ajeno"] = "prov-2"
	f.driverVehicles["d-mio"] = "veh-mio"
	uc := New(f, testLog)
	ctx := context.Background()

	o, _ := uc.Create(ctx, storePrincipal("s1"), validInput("A1", "s1"))
	prov := authz.Principal{Role: authz.RoleProvider, TokenType: authz.AccessToken, UserID: "p1", ProviderID: "prov-1"}

	// Rol equivocado.
	driver := authz.Principal{Role: authz.RoleDriver, TokenType: authz.AccessToken, DriverID: "d-mio"}
	if _, err := uc.Assign(ctx, driver, o.ID, "veh-mio", "d-mio"); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Errorf("driver: err = %v", err)
	}
	// Vehículo de otro proveedor → scope, sin revelar existencia.
	if _, err := uc.Assign(ctx, prov, o.ID, "veh-ajeno", "d-mio"); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Errorf("vehículo ajeno: err = %v", err)
	}
	// Conductor no pertenece al vehículo → validación.
	if _, err := uc.Assign(ctx, prov, o.ID, "veh-mio", "d-otro"); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("conductor incoherente: err = %v", err)
	}
	if f.assignCalls != 0 {
		t.Fatalf("no debe haber asignaciones fallidas; calls = %d", f.assignCalls)
	}
	// Caso feliz.
	got, err := uc.Assign(ctx, prov, o.ID, "veh-mio", "d-mio")
	if err != nil {
		t.Fatalf("asignar: %v", err)
	}
	if got.AssignedVehicleID == nil || *got.AssignedVehicleID != "veh-mio" ||
		got.AssignedDriverID == nil || *got.AssignedDriverID != "d-mio" {
		t.Errorf("got = %+v, sin asignar", got)
	}
}

// IsAccessibleBy es el punto de extensión inyectado en el módulo auth.
func TestIsAccessibleBy(t *testing.T) {
	f := newFakeOrders()
	f.stores["s1"] = true
	uc := New(f, testLog)
	ctx := context.Background()

	o, _ := uc.Create(ctx, storePrincipal("s1"), validInput("IA", "s1"))

	ok, err := uc.IsAccessibleBy(ctx, o.ID, storePrincipal("s1"))
	if err != nil || !ok {
		t.Errorf("dueña: ok=%v err=%v", ok, err)
	}
	ok, err = uc.IsAccessibleBy(ctx, "no-existe", storePrincipal("s1"))
	if err != nil || ok {
		t.Errorf("inexistente: ok=%v err=%v (no debe fallar, solo false)", ok, err)
	}
	ok, err = uc.IsAccessibleBy(ctx, o.ID, storePrincipal("s2"))
	if err != nil || ok {
		t.Errorf("ajena: ok=%v err=%v", ok, err)
	}
}

// El repositorio fake usa time para los eventos del dominio.
var _ = time.Now
