package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/resilience"
	"tiendas-track/backend/internal/telemetry/domain"
)

// --- dobles de prueba -------------------------------------------------------

type fakePoints struct {
	mu          sync.Mutex
	dups        map[domain.InsertedKey]bool
	bulkDone    map[string]bool
	inserted    []domain.Point
	err         error
	latest      *domain.Point
	track       []domain.Point
	insertCalls int
	bulkCalls   int
	latestCalls int
	trackCalls  int
}

func newFakePoints() *fakePoints {
	return &fakePoints{
		dups:     make(map[domain.InsertedKey]bool),
		bulkDone: make(map[string]bool),
	}
}

func (f *fakePoints) Insert(_ context.Context, points []domain.Point) ([]domain.InsertedKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.insertCalls++
	if f.err != nil {
		return nil, f.err
	}
	var out []domain.InsertedKey
	for _, p := range points {
		k := domain.InsertedKey{Time: p.Time, VehicleID: p.VehicleID, DeviceSeq: p.DeviceSeq}
		if f.dups[k] {
			continue
		}
		f.dups[k] = true
		f.inserted = append(f.inserted, p)
		out = append(out, k)
	}
	return out, nil
}

func (f *fakePoints) ProcessBulk(_ context.Context, r domain.Receipt, points []domain.Point) (domain.BulkResult, error) {
	f.mu.Lock()
	f.bulkCalls++
	already := f.bulkDone[r.Key]
	if f.err != nil {
		f.mu.Unlock()
		return domain.BulkResult{}, f.err
	}
	if already {
		f.mu.Unlock()
		return domain.BulkResult{AlreadyProcessed: true, ProcessedAt: time.Now().UTC()}, nil
	}
	f.bulkDone[r.Key] = true
	f.mu.Unlock()

	inserted, err := f.Insert(context.Background(), points)
	if err != nil {
		return domain.BulkResult{}, err
	}
	return domain.BulkResult{Inserted: inserted, ProcessedAt: time.Now().UTC()}, nil
}

func (f *fakePoints) Latest(_ context.Context, _ string) (*domain.Point, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.latestCalls++
	if f.err != nil {
		return nil, f.err
	}
	if f.latest == nil {
		return nil, domain.ErrNotFound
	}
	return f.latest, nil
}

func (f *fakePoints) Track(_ context.Context, _ string, _, _ time.Time, _ int) ([]domain.Point, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trackCalls++
	if f.err != nil {
		return nil, f.err
	}
	if len(f.track) == 0 {
		return nil, domain.ErrNotFound
	}
	return f.track, nil
}

type fakeScopes struct {
	vehicles     map[string]*domain.VehicleScope
	byDriver     map[string]string // app.drivers.id → vehicle id
	orders       map[string]*domain.OrderScope
	storeVehicle map[string]bool // "vehicleID|storeID"
	err          error
}

func (f *fakeScopes) VehicleScope(_ context.Context, id string) (*domain.VehicleScope, error) {
	if f.err != nil {
		return nil, f.err
	}
	if vs, ok := f.vehicles[id]; ok {
		return vs, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeScopes) VehicleByDriver(_ context.Context, driverID string) (*domain.VehicleScope, error) {
	if f.err != nil {
		return nil, f.err
	}
	vid, ok := f.byDriver[driverID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	vs, ok := f.vehicles[vid]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return vs, nil
}

func (f *fakeScopes) OrderScope(_ context.Context, id string) (*domain.OrderScope, error) {
	if f.err != nil {
		return nil, f.err
	}
	if os, ok := f.orders[id]; ok {
		return os, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeScopes) VehicleInStoreScope(_ context.Context, vehicleID, storeID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.storeVehicle[vehicleID+"|"+storeID], nil
}

type published struct {
	topic   string
	payload []byte
}

type fakeBus struct {
	mu   sync.Mutex
	msgs []published
}

func (b *fakeBus) Publish(topic string, payload []byte) {
	b.mu.Lock()
	b.msgs = append(b.msgs, published{topic: topic, payload: payload})
	b.mu.Unlock()
}

func (b *fakeBus) topics() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(b.msgs))
	for _, m := range b.msgs {
		out = append(out, m.topic)
	}
	return out
}

func (b *fakeBus) reset() {
	b.mu.Lock()
	b.msgs = nil
	b.mu.Unlock()
}

// --- helpers ---------------------------------------------------------------

const (
	prov1  = "11111111-1111-4111-8111-111111111111"
	prov2  = "22222222-2222-4222-8222-222222222222"
	drv1   = "33333333-3333-4333-8333-333333333333"
	drv2   = "44444444-4444-4444-8444-444444444444"
	veh1   = "55555555-5555-4555-8555-555555555555"
	veh2   = "66666666-6666-4666-8666-666666666666"
	store1 = "77777777-7777-4777-8777-777777777777"
	store2 = "88888888-8888-4888-8888-888888888888"
	ord1   = "99999999-9999-4999-8999-999999999999"
	ord2   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newFixture(t *testing.T) (*TelemetryUseCase, *fakePoints, *fakeScopes, *fakeBus) {
	t.Helper()
	points := newFakePoints()
	scopes := &fakeScopes{
		vehicles: map[string]*domain.VehicleScope{
			veh1: {VehicleID: veh1, ProviderID: prov1, DriverID: drv1},
			veh2: {VehicleID: veh2, ProviderID: prov2, DriverID: drv2},
		},
		byDriver: map[string]string{drv1: veh1, drv2: veh2},
		orders: map[string]*domain.OrderScope{
			ord1: {OrderID: ord1, StoreID: store1, Status: "IN_TRANSIT", AssignedVehicleID: veh1, AssignedDriverID: drv1},
			ord2: {OrderID: ord2, StoreID: store2, Status: "CREATED", AssignedVehicleID: veh2, AssignedDriverID: drv2},
		},
		storeVehicle: map[string]bool{
			veh1 + "|" + store1: true,
			veh2 + "|" + store2: true,
		},
	}
	bus := &fakeBus{}
	uc := New(points, scopes, bus, resilience.New(resilience.DefaultSettings("test")), testLogger())
	uc.now = func() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }
	return uc, points, scopes, bus
}

func driverP() authz.Principal {
	return authz.Principal{
		UserID: "user-drv1", Role: authz.RoleDriver, TokenType: authz.AccessToken,
		DriverID: drv1, ProviderID: prov1,
	}
}

func providerP() authz.Principal {
	return authz.Principal{
		UserID: prov1, Role: authz.RoleProvider, TokenType: authz.AccessToken, ProviderID: prov1,
	}
}

func storeP() authz.Principal {
	return authz.Principal{
		UserID: "user-store1", Role: authz.RoleStore, TokenType: authz.AccessToken, StoreID: store1,
	}
}

func validInput(vehicle string) PointInput {
	return PointInput{
		VehicleID: vehicle,
		Time:      time.Date(2026, 10, 28, 11, 59, 0, 0, time.UTC),
		Lat:       4.6533,
		Lon:       -74.0836,
		DeviceSeq: 1,
	}
}

func hasTopic(topics []string, want string) bool {
	for _, t := range topics {
		if t == want {
			return true
		}
	}
	return false
}

// --- ingesta individual -----------------------------------------------------

// Un conductor que reporta su propio vehículo inserta y publica en su canal
// de vehículo y en el de la flota de su proveedor (ADR-08).
func TestIngestValidaPublica(t *testing.T) {
	uc, points, _, bus := newFixture(t)

	pt, dup, err := uc.Ingest(context.Background(), driverP(), validInput(veh1))
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if dup {
		t.Error("no debía ser duplicado")
	}
	if pt.VehicleID != veh1 || pt.Source != domain.SourceDevice {
		t.Errorf("point = %+v", pt)
	}
	if len(points.inserted) != 1 {
		t.Errorf("insertados = %d", len(points.inserted))
	}

	topics := bus.topics()
	if !hasTopic(topics, "vehicle:"+veh1) || !hasTopic(topics, "fleet:provider:"+prov1) {
		t.Errorf("topics = %v", topics)
	}
	if hasTopic(topics, "order:"+ord1) {
		t.Error("sin order_id no debe publicarse en el canal de orden")
	}
}

// La app del conductor no necesita conocer su vehicle_id: se deriva del scope.
func TestIngestDerivaVehiculoDelConductor(t *testing.T) {
	uc, _, _, bus := newFixture(t)

	in := validInput("")
	pt, dup, err := uc.Ingest(context.Background(), driverP(), in)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if dup || pt.VehicleID != veh1 {
		t.Errorf("vehicle = %q dup=%v", pt.VehicleID, dup)
	}
	if !hasTopic(bus.topics(), "vehicle:"+veh1) {
		t.Errorf("topics = %v", bus.topics())
	}
}

// La ingesta con order_id publica además en los canales de orden y tienda.
func TestIngestConOrdenPublicaCanalesDeOrden(t *testing.T) {
	uc, _, _, bus := newFixture(t)

	in := validInput(veh1)
	in.OrderID = ord1
	if _, _, err := uc.Ingest(context.Background(), driverP(), in); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	topics := bus.topics()
	for _, want := range []string{"vehicle:" + veh1, "fleet:provider:" + prov1, "order:" + ord1, "fleet:store:" + store1} {
		if !hasTopic(topics, want) {
			t.Errorf("falta topic %q en %v", want, topics)
		}
	}
}

// Un reintento del dispositivo (misma PK) no inserta ni reemite el evento.
func TestIngestDuplicadoNoPublica(t *testing.T) {
	uc, points, _, bus := newFixture(t)
	in := validInput(veh1)
	k := domain.InsertedKey{Time: in.Time, VehicleID: in.VehicleID, DeviceSeq: in.DeviceSeq}
	points.dups[k] = true

	_, dup, err := uc.Ingest(context.Background(), driverP(), in)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if !dup {
		t.Error("debía marcarse duplicado")
	}
	if len(bus.topics()) != 0 {
		t.Errorf("no debía publicar, publicó %v", bus.topics())
	}
}

// STORE no ingesta telemetría (solo consulta su vista dual).
func TestIngestRolNoPermitido(t *testing.T) {
	uc, points, _, _ := newFixture(t)

	_, _, err := uc.Ingest(context.Background(), storeP(), validInput(veh1))
	if !errors.Is(err, domain.ErrForbiddenScope) {
		t.Fatalf("err = %v, esperado ErrForbiddenScope", err)
	}
	if points.insertCalls != 0 {
		t.Error("no debe tocar la BD sin permiso")
	}
}

// El invitado nunca ingesta.
func TestIngestInvitadoDenegado(t *testing.T) {
	uc, _, _, _ := newFixture(t)
	guest := authz.Principal{TokenType: authz.GuestToken, Scope: "order:" + ord1}

	if _, _, err := uc.Ingest(context.Background(), guest, validInput(veh1)); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Fatalf("err = %v", err)
	}
}

// Un conductor no puede reportar el vehículo de otro (aislamiento server-side).
func TestIngestVehiculoAjeno(t *testing.T) {
	uc, points, _, bus := newFixture(t)

	_, _, err := uc.Ingest(context.Background(), driverP(), validInput(veh2))
	if !errors.Is(err, domain.ErrForbiddenScope) {
		t.Fatalf("err = %v, esperado ErrForbiddenScope", err)
	}
	if points.insertCalls != 0 || len(bus.topics()) != 0 {
		t.Error("no debe insertar ni publicar sobre un vehículo ajeno")
	}
}

// El proveedor solo ingesta vehículos de su flota.
func TestIngestProveedorFlotaAjena(t *testing.T) {
	uc, _, _, _ := newFixture(t)
	if _, _, err := uc.Ingest(context.Background(), providerP(), validInput(veh2)); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := uc.Ingest(context.Background(), providerP(), validInput(veh1)); err != nil {
		t.Fatalf("su propio vehículo: %v", err)
	}
}

// order_id debe estar asignada al vehículo que reporta (422, no 403: es dato).
func TestIngestOrdenNoAsignada(t *testing.T) {
	uc, _, _, _ := newFixture(t)
	in := validInput(veh1)
	in.OrderID = ord2

	_, _, err := uc.Ingest(context.Background(), driverP(), in)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("err = %v, esperado ErrValidation", err)
	}
}

// Una orden inexistente es error de validación (no filtra existencia ajena).
func TestIngestOrdenInexistente(t *testing.T) {
	uc, _, _, _ := newFixture(t)
	in := validInput(veh1)
	in.OrderID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"

	if _, _, err := uc.Ingest(context.Background(), driverP(), in); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("err = %v", err)
	}
}

// Invariantes de la muestra GPS (ADR-09).
func TestIngestValidaciones(t *testing.T) {
	uc, points, _, _ := newFixture(t)
	now := uc.now()

	cases := []struct {
		name string
		mod  func(*PointInput)
	}{
		{"lat fuera de rango", func(in *PointInput) { in.Lat = 91 }},
		{"lon fuera de rango", func(in *PointInput) { in.Lon = -181 }},
		{"coordenada nula", func(in *PointInput) { in.Lat, in.Lon = 0, 0 }},
		{"timestamp futuro", func(in *PointInput) { in.Time = now.Add(time.Hour) }},
		{"timestamp vacío", func(in *PointInput) { in.Time = time.Time{} }},
		{"velocidad negativa", func(in *PointInput) { v := -1.0; in.SpeedKPH = &v }},
		{"heading fuera de rango", func(in *PointInput) { h := 361.0; in.Heading = &h }},
		{"device_seq negativo", func(in *PointInput) { in.DeviceSeq = -1 }},
	}
	for _, c := range cases {
		in := validInput(veh1)
		c.mod(&in)
		if _, _, err := uc.Ingest(context.Background(), driverP(), in); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("%s: err = %v, esperado ErrValidation", c.name, err)
		}
	}
	if points.insertCalls != 0 {
		t.Errorf("insertCalls = %d, ninguna muestra inválida debe llegar a la BD", points.insertCalls)
	}
}

// --- bulk sync -------------------------------------------------------------

func bulkOf(t *testing.T, n int) BulkInput {
	t.Helper()
	in := BulkInput{IdempotencyKey: "device-abc-0001", DeviceID: "device-1"}
	base := time.Date(2026, 10, 28, 11, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		in.Points = append(in.Points, PointInput{
			VehicleID: veh1,
			Time:      base.Add(time.Duration(i) * time.Minute),
			Lat:       4.65,
			Lon:       -74.08,
			DeviceSeq: int64(i + 1),
		})
	}
	return in
}

// El mismo Idempotency-Key no reprocesa ni republica (RF-09).
func TestBulkIdempotente(t *testing.T) {
	uc, _, _, bus := newFixture(t)
	in := bulkOf(t, 3)

	res, err := uc.IngestBulk(context.Background(), driverP(), in)
	if err != nil {
		t.Fatalf("IngestBulk: %v", err)
	}
	if res.AlreadyProcessed || len(res.Inserted) != 3 {
		t.Fatalf("resultado = %+v", res)
	}
	first := len(bus.topics())
	if first == 0 {
		t.Fatal("debía publicar los puntos insertados")
	}

	bus.reset()
	res2, err := uc.IngestBulk(context.Background(), driverP(), in)
	if err != nil {
		t.Fatalf("reintento: %v", err)
	}
	if !res2.AlreadyProcessed {
		t.Error("el reintento debe ser idempotente")
	}
	if got := res2.Duplicates(len(in.Points)); got != len(in.Points) {
		t.Errorf("duplicates = %d", got)
	}
	if len(bus.topics()) != 0 {
		t.Errorf("el reintento no debe republicar: %v", bus.topics())
	}
}

// Puntos repetidos dentro de un lote nuevo se cuentan como duplicados.
func TestBulkDuplicadosParciales(t *testing.T) {
	uc, _, _, _ := newFixture(t)

	if _, err := uc.IngestBulk(context.Background(), driverP(), bulkOf(t, 2)); err != nil {
		t.Fatalf("primer lote: %v", err)
	}

	second := bulkOf(t, 3)
	second.IdempotencyKey = "device-abc-0002"
	res, err := uc.IngestBulk(context.Background(), driverP(), second)
	if err != nil {
		t.Fatalf("segundo lote: %v", err)
	}
	if res.AlreadyProcessed || len(res.Inserted) != 1 {
		t.Errorf("resultado = %+v", res)
	}
	if got := res.Duplicates(len(second.Points)); got != 2 {
		t.Errorf("duplicates = %d, esperado 2", got)
	}
}

// El bulk exige Idempotency-Key con longitud razonable.
func TestBulkIdempotencyKey(t *testing.T) {
	uc, _, _, _ := newFixture(t)

	for _, key := range []string{"", "corta"} {
		in := bulkOf(t, 1)
		in.IdempotencyKey = key
		if _, err := uc.IngestBulk(context.Background(), driverP(), in); !errors.Is(err, domain.ErrIdempotencyKey) {
			t.Errorf("key %q: err = %v", key, err)
		}
	}
	in := bulkOf(t, 1)
	in.IdempotencyKey = string(make([]byte, MaxKeyLen+1))
	if _, err := uc.IngestBulk(context.Background(), driverP(), in); !errors.Is(err, domain.ErrIdempotencyKey) {
		t.Errorf("key larga: err = %v", err)
	}
}

// Un lote vacío o sobredimensionado se rechaza antes de tocar la BD.
func TestBulkTamanio(t *testing.T) {
	uc, points, _, _ := newFixture(t)
	empty := bulkOf(t, 0)
	if _, err := uc.IngestBulk(context.Background(), driverP(), empty); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("vacío: err = %v", err)
	}

	big := bulkOf(t, MaxBatchPoints+1)
	if _, err := uc.IngestBulk(context.Background(), driverP(), big); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("grande: err = %v", err)
	}
	if points.bulkCalls != 0 {
		t.Errorf("bulkCalls = %d", points.bulkCalls)
	}
}

// Un punto inválido invalida el lote completo (todo o nada).
func TestBulkPuntoInvalido(t *testing.T) {
	uc, points, _, _ := newFixture(t)
	in := bulkOf(t, 3)
	in.Points[1].Lat = 120

	if _, err := uc.IngestBulk(context.Background(), driverP(), in); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("err = %v", err)
	}
	if points.bulkCalls != 0 {
		t.Error("no debe procesarse un lote con puntos inválidos")
	}
}

// El bulk también valida el scope del vehículo.
func TestBulkVehiculoAjeno(t *testing.T) {
	uc, points, _, _ := newFixture(t)
	in := bulkOf(t, 1)
	in.Points[0].VehicleID = veh2

	if _, err := uc.IngestBulk(context.Background(), driverP(), in); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Fatalf("err = %v", err)
	}
	if points.bulkCalls != 0 {
		t.Error("no debe procesarse")
	}
}

// El recibo guarda el usuario del conductor para auditoría.
func TestBulkRegistraConductor(t *testing.T) {
	uc, points, _, _ := newFixture(t)
	var got domain.Receipt
	uc.points = receiptSpy{points: points, onBulk: func(r domain.Receipt) { got = r }}

	if _, err := uc.IngestBulk(context.Background(), driverP(), bulkOf(t, 2)); err != nil {
		t.Fatalf("IngestBulk: %v", err)
	}
	if got.DriverUserID != "user-drv1" || got.DeviceID != "device-1" || got.PointsCount != 2 {
		t.Errorf("recibo = %+v", got)
	}
}

// receiptSpy envuelve fakePoints solo para capturar el recibo.
type receiptSpy struct {
	points *fakePoints
	onBulk func(domain.Receipt)
}

func (s receiptSpy) Insert(ctx context.Context, pts []domain.Point) ([]domain.InsertedKey, error) {
	return s.points.Insert(ctx, pts)
}

func (s receiptSpy) ProcessBulk(ctx context.Context, r domain.Receipt, pts []domain.Point) (domain.BulkResult, error) {
	s.onBulk(r)
	return s.points.ProcessBulk(ctx, r, pts)
}

func (s receiptSpy) Latest(ctx context.Context, id string) (*domain.Point, error) {
	return s.points.Latest(ctx, id)
}

func (s receiptSpy) Track(ctx context.Context, id string, from, to time.Time, limit int) ([]domain.Point, error) {
	return s.points.Track(ctx, id, from, to, limit)
}

// --- consultas y resiliencia ----------------------------------------------

// Tras N fallos consecutivos las lecturas responden CIRCUIT_OPEN (RNF-05).
func TestLatestCircuitoAbierto(t *testing.T) {
	uc, points, _, _ := newFixture(t)
	uc.reads = resilience.New(resilience.Settings{Name: "reads", FailureThreshold: 1, Timeout: time.Minute})
	points.err = errors.New("bd caída")

	if _, err := uc.Latest(context.Background(), driverP(), veh1); err == nil {
		t.Fatal("el primer fallo debe propagarse")
	}

	_, err := uc.Latest(context.Background(), driverP(), veh1)
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("err = %v, esperado ErrCircuitOpen", err)
	}
	if points.latestCalls != 1 {
		t.Errorf("latestCalls = %d, el circuito abierto no debe llegar a la BD", points.latestCalls)
	}
}

func TestLatestOk(t *testing.T) {
	uc, points, _, _ := newFixture(t)
	points.latest = &domain.Point{VehicleID: veh1, Lat: 4.6, Lon: -74.0}

	pt, err := uc.Latest(context.Background(), driverP(), veh1)
	if err != nil || pt.VehicleID != veh1 {
		t.Fatalf("pt=%+v err=%v", pt, err)
	}
}

// El tendero ve la última posición de un vehículo de sus órdenes (vista dual).
func TestLatestTenderoEnScope(t *testing.T) {
	uc, points, _, _ := newFixture(t)
	points.latest = &domain.Point{VehicleID: veh1, Lat: 4.6, Lon: -74.0}

	if _, err := uc.Latest(context.Background(), storeP(), veh1); err != nil {
		t.Fatalf("vehículo de su orden: %v", err)
	}
	if _, err := uc.Latest(context.Background(), storeP(), veh2); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Fatalf("vehículo ajeno: err = %v", err)
	}
}

// El invitado no consulta REST: solo su canal de orden en WS.
func TestLatestInvitadoDenegado(t *testing.T) {
	uc, points, _, _ := newFixture(t)
	guest := authz.Principal{TokenType: authz.GuestToken, Scope: "order:" + ord1}

	if _, err := uc.Latest(context.Background(), guest, veh1); !errors.Is(err, domain.ErrForbiddenScope) {
		t.Fatalf("err = %v", err)
	}
	if points.latestCalls != 0 {
		t.Error("no debe consultar la BD")
	}
}

// Un vehículo inexistente se responde como ajeno (anti-enumeración).
func TestLatestVehiculoInexistente(t *testing.T) {
	uc, _, _, _ := newFixture(t)
	_, err := uc.Latest(context.Background(), driverP(), "cccccccc-cccc-4ccc-8ccc-cccccccccccc")
	if !errors.Is(err, domain.ErrForbiddenScope) {
		t.Fatalf("err = %v", err)
	}
}

func TestTrackRangos(t *testing.T) {
	uc, points, _, _ := newFixture(t)
	from := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)

	cases := []struct {
		name  string
		from  time.Time
		to    time.Time
		limit int
	}{
		{"sin rango", time.Time{}, to, 10},
		{"to antes de from", to, from, 10},
		{"rango excesivo", from, from.Add(MaxTrackRange + time.Minute), 10},
		{"limit negativo", from, to, -1},
		{"limit excesivo", from, to, MaxTrackLimit + 1},
	}
	for _, c := range cases {
		if _, err := uc.Track(context.Background(), driverP(), veh1, c.from, c.to, c.limit); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
	if points.trackCalls != 0 {
		t.Errorf("trackCalls = %d", points.trackCalls)
	}

	points.track = []domain.Point{{VehicleID: veh1, Lat: 4.6, Lon: -74.0}}
	if _, err := uc.Track(context.Background(), driverP(), veh1, from, to, 10); err != nil {
		t.Fatalf("rango válido: %v", err)
	}
}

// --- autorización de canales WS -------------------------------------------

func TestCanSubscribe(t *testing.T) {
	uc, _, _, _ := newFixture(t)
	guest := authz.Principal{TokenType: authz.GuestToken, Scope: "order:" + ord1}

	cases := []struct {
		name string
		p    authz.Principal
		ch   authz.Channel
		want bool
	}{
		{"proveedor su flota", providerP(), authz.Channel{Kind: authz.ChannelFleetProvider, ID: prov1}, true},
		{"proveedor flota ajena", providerP(), authz.Channel{Kind: authz.ChannelFleetProvider, ID: prov2}, false},
		{"proveedor sin provider_id", authz.Principal{Role: authz.RoleProvider, TokenType: authz.AccessToken}, authz.Channel{Kind: authz.ChannelFleetProvider, ID: prov1}, false},
		{"tendero su tienda", storeP(), authz.Channel{Kind: authz.ChannelFleetStore, ID: store1}, true},
		{"tendero tienda ajena", storeP(), authz.Channel{Kind: authz.ChannelFleetStore, ID: store2}, false},
		{"conductor no ve flotas", driverP(), authz.Channel{Kind: authz.ChannelFleetProvider, ID: prov1}, false},
		{"conductor su orden", driverP(), authz.Channel{Kind: authz.ChannelOrder, ID: ord1}, true},
		{"conductor orden ajena", driverP(), authz.Channel{Kind: authz.ChannelOrder, ID: ord2}, false},
		{"tendero orden de su tienda", storeP(), authz.Channel{Kind: authz.ChannelOrder, ID: ord1}, true},
		{"tendero orden ajena", storeP(), authz.Channel{Kind: authz.ChannelOrder, ID: ord2}, false},
		{"proveedor orden de su flota", providerP(), authz.Channel{Kind: authz.ChannelOrder, ID: ord1}, true},
		{"proveedor orden ajena", providerP(), authz.Channel{Kind: authz.ChannelOrder, ID: ord2}, false},
		{"invitado su orden", guest, authz.Channel{Kind: authz.ChannelOrder, ID: ord1}, true},
		{"invitado orden ajena", guest, authz.Channel{Kind: authz.ChannelOrder, ID: ord2}, false},
		{"invitado no ve flota", guest, authz.Channel{Kind: authz.ChannelFleetProvider, ID: prov1}, false},
		{"invitado no ve vehículo", guest, authz.Channel{Kind: authz.ChannelVehicle, ID: veh1}, false},
		{"conductor su vehículo", driverP(), authz.Channel{Kind: authz.ChannelVehicle, ID: veh1}, true},
		{"conductor vehículo ajeno", driverP(), authz.Channel{Kind: authz.ChannelVehicle, ID: veh2}, false},
		{"tendero vehículo de su orden", storeP(), authz.Channel{Kind: authz.ChannelVehicle, ID: veh1}, true},
		{"canal desconocido", providerP(), authz.Channel{Kind: "unknown", ID: prov1}, false},
	}
	for _, c := range cases {
		got, err := uc.CanSubscribe(context.Background(), c.p, c.ch)
		if err != nil {
			t.Errorf("%s: err = %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %v, esperado %v", c.name, got, c.want)
		}
	}
}

// Si el repositorio de scope falla, la autorización falla cerrada con error.
func TestCanSubscribeErrorFallaCerrado(t *testing.T) {
	uc, _, scopes, _ := newFixture(t)
	scopes.err = fmt.Errorf("bd caída")

	ok, err := uc.CanSubscribe(context.Background(), driverP(),
		authz.Channel{Kind: authz.ChannelOrder, ID: ord1})
	if err == nil || ok {
		t.Fatalf("ok=%v err=%v, debía fallar cerrado", ok, err)
	}
}
