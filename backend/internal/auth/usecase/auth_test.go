package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"tiendas-track/backend/internal/auth/domain"
	"tiendas-track/backend/internal/shared/authz"
)

// --- Fakes -----------------------------------------------------------------

type fakeUsers struct {
	byEmail map[string]*domain.User
	byID    map[string]*domain.User
	created *domain.User
	storeID string // tienda creada junto al registro STORE
}

// Create almacena una copia: el caso de uso limpia PasswordHash del objeto
// que devuelve, y ese mutado no debe contaminar el estado persistido.
func (f *fakeUsers) Create(_ context.Context, u *domain.User) error {
	u.ID = "new-id"
	cp := *u
	f.created = &cp
	f.byEmail[cp.Email] = &cp
	f.byID[cp.ID] = &cp
	return nil
}

// CreateWithStore replica la semántica transaccional: usuario + tienda juntos.
func (f *fakeUsers) CreateWithStore(ctx context.Context, u *domain.User, p *domain.StoreProfile) (string, error) {
	if p == nil || !p.Valid() {
		return "", domain.ErrValidation
	}
	if err := f.Create(ctx, u); err != nil {
		return "", err
	}
	f.storeID = "store-" + u.ID
	return f.storeID, nil
}
func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	cp := *u
	return &cp, nil
}
func (f *fakeUsers) FindByID(_ context.Context, id string) (*domain.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	cp := *u
	return &cp, nil
}

// IdentityRepository: alcance de datos por rol (los fakes devuelven el alcance fijo).
func (f *fakeUsers) FindStoreIDByOwner(_ context.Context, _ string) (string, error) {
	return f.storeID, nil
}
func (f *fakeUsers) FindDriverIdentity(_ context.Context, _ string) (string, string, error) {
	return "driver-1", "vehicle-1", nil
}

type fakeRefresh struct {
	saved   map[string]bool
	revoked map[string]bool
}

func (f *fakeRefresh) Save(_ context.Context, _, hash string, _ time.Time) error {
	if f.saved == nil {
		f.saved = map[string]bool{}
	}
	f.saved[hash] = true
	return nil
}
func (f *fakeRefresh) FindActiveByHash(_ context.Context, hash string) (bool, error) {
	return f.saved[hash] && !f.revoked[hash], nil
}
func (f *fakeRefresh) Revoke(_ context.Context, hash string) error {
	if f.revoked == nil {
		f.revoked = map[string]bool{}
	}
	f.revoked[hash] = true
	return nil
}

type fakeGuests struct{ saved int }

func (f *fakeGuests) Save(_ context.Context, _, _ string, _ time.Time) error {
	f.saved++
	return nil
}

// fakeTokens emite marcadores verificables sin criptografía real.
type fakeTokens struct{ revokedRefresh string }

func (f *fakeTokens) IssueAccess(userID, role, storeID, providerID, driverID string) (domain.IssuedToken, error) {
	return domain.IssuedToken{Raw: "acc|" + userID + "|" + role, JTI: "jti-a", ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
}
func (f *fakeTokens) IssueRefresh(userID string) (domain.IssuedToken, error) {
	raw := "ref|" + userID + "|" + time.Now().Format("150405.00000000")
	return domain.IssuedToken{Raw: raw, JTI: "jti-r", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (f *fakeTokens) IssueGuest(orderID string) (domain.IssuedToken, error) {
	return domain.IssuedToken{Raw: "gst|" + orderID, JTI: "jti-g", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (f *fakeTokens) Parse(raw string) (*domain.TokenClaims, error) {
	parts := strings.Split(raw, "|")
	switch parts[0] {
	case "acc":
		return &domain.TokenClaims{Kind: "access", Subject: parts[1], Role: parts[2]}, nil
	case "ref":
		return &domain.TokenClaims{Kind: "refresh", Subject: parts[1]}, nil
	case "gst":
		return &domain.TokenClaims{Kind: "guest", Scope: "order:" + parts[1]}, nil
	}
	return nil, errors.New("token inválido")
}

type fakeHasher struct{}

func (fakeHasher) Hash(plain string) (string, error) { return "hash:" + plain, nil }
func (fakeHasher) Verify(hash, plain string) bool    { return hash == "hash:"+plain }

// --- Tests ---

// storeProfile devuelve un perfil de tienda válido para registros STORE.
func storeProfile(name string) *domain.StoreProfile {
	return &domain.StoreProfile{Name: name, Address: "Calle 1 # 2-3", Lat: 4.65, Lon: -74.05}
}

func setup(t *testing.T, allowOrder bool) (*AuthUseCase, *fakeRefresh, *fakeGuests) {
	t.Helper()
	users := &fakeUsers{byEmail: map[string]*domain.User{}, byID: map[string]*domain.User{}}
	refresh := &fakeRefresh{}
	guests := &fakeGuests{}
	checker := func(context.Context, string, authz.Principal) (bool, error) { return allowOrder, nil }
	uc := New(users, users, refresh, guests, &fakeTokens{}, fakeHasher{}, checker)
	return uc, refresh, guests
}

// Registro: solo STORE/DRIVER y con validación de email/contraseña.
func TestRegisterValidaEntrada(t *testing.T) {
	uc, _, _ := setup(t, true)

	if _, err := uc.Register(context.Background(), RegisterInput{
		Email: "a@b.co", Password: "12345678", Role: "PROVIDER",
	}); !errors.Is(err, domain.ErrInvalidRole) {
		t.Errorf("PROVIDER auto-registrado: err = %v, esperado ErrInvalidRole", err)
	}
	if _, err := uc.Register(context.Background(), RegisterInput{
		Email: "no-email", Password: "12345678", Role: "STORE",
	}); !errors.Is(err, domain.ErrInvalidEmail) {
		t.Errorf("email inválido: err = %v", err)
	}
	if _, err := uc.Register(context.Background(), RegisterInput{
		Email: "a@b.co", Password: "corta", Role: "STORE",
	}); !errors.Is(err, domain.ErrWeakPassword) {
		t.Errorf("contraseña corta: err = %v", err)
	}
	// El rol STORE exige el perfil de tienda (sin ella no hay scope de datos).
	if _, err := uc.Register(context.Background(), RegisterInput{
		Email: "sintienda@b.co", Password: "12345678", Role: "STORE",
	}); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("STORE sin perfil: err = %v, esperado ErrValidation", err)
	}
	// Coordenada inválida en el perfil de tienda (PostGIS, ADR-09).
	if _, err := uc.Register(context.Background(), RegisterInput{
		Email: "tiendanula@b.co", Password: "12345678", Role: "STORE",
		Store: &domain.StoreProfile{Name: "Sin coords", Lat: 0, Lon: 0},
	}); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("STORE coordenada nula: err = %v, esperado ErrValidation", err)
	}
}

// Registro duplicado → 409 (ErrEmailAlreadyRegistered).
func TestRegisterDuplicado(t *testing.T) {
	uc, _, _ := setup(t, true)
	in := RegisterInput{Email: "dup@b.co", Password: "12345678", Role: "STORE", Store: storeProfile("Dup")}
	if _, err := uc.Register(context.Background(), in); err != nil {
		t.Fatalf("primer registro: %v", err)
	}
	if _, err := uc.Register(context.Background(), in); !errors.Is(err, domain.ErrEmailAlreadyRegistered) {
		t.Errorf("err = %v, esperado ErrEmailAlreadyRegistered", err)
	}
}

// Login: credenciales correctas emiten par con expires_in > 0.
func TestLoginEmitePar(t *testing.T) {
	uc, refreshLogged, _ := setup(t, true)
	ctx := context.Background()
	if _, err := uc.Register(ctx, RegisterInput{Email: "x@b.co", Password: "12345678", Role: "DRIVER"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	pair, err := uc.Login(ctx, "x@b.co", "12345678")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" || pair.ExpiresIn <= 0 {
		t.Errorf("pair = %+v, incompleto", pair)
	}
	if len(refreshLogged.saved) != 1 {
		t.Error("login debe persistir el hash del refresh token")
	}
	if pair.User != nil && pair.User.PasswordHash != "" {
		t.Error("la respuesta no debe exponer el hash de contraseña")
	}
}

// Login con password erróneo devuelve la misma respuesta que email inexistente.
func TestLoginSinEnumerarUsuarios(t *testing.T) {
	uc, _, _ := setup(t, true)
	ctx := context.Background()
	_, _ = uc.Register(ctx, RegisterInput{Email: "x@b.co", Password: "12345678", Role: "STORE", Store: storeProfile("Principal")})

	_, errWrongPass := uc.Login(ctx, "x@b.co", "otra-clave")
	_, errNoUser := uc.Login(ctx, "nadie@b.co", "12345678")
	if !errors.Is(errWrongPass, domain.ErrInvalidCredentials) || !errors.Is(errNoUser, domain.ErrInvalidCredentials) {
		t.Errorf("errores = (%v, %v), ambos deben ser ErrInvalidCredentials", errWrongPass, errNoUser)
	}
	if errWrongPass.Error() != errNoUser.Error() {
		t.Error("los mensajes no deben permitir distinguir usuario inexistente")
	}
}

// Refresh rota: el token presentado queda revocado (uso único).
func TestRefreshRotaElToken(t *testing.T) {
	uc, refreshStore, _ := setup(t, true)
	ctx := context.Background()
	_, _ = uc.Register(ctx, RegisterInput{Email: "x@b.co", Password: "12345678", Role: "STORE", Store: storeProfile("Principal")})
	pair, _ := uc.Login(ctx, "x@b.co", "12345678")

	newPair, err := uc.Refresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	// Rotación: el hash del token anterior queda revocado en BD.
	oldHash := hashToken(pair.RefreshToken)
	if !refreshStore.revoked[oldHash] {
		t.Error("el refresh presentado debe quedar revocado tras rotar")
	}
	if newPair.RefreshToken == pair.RefreshToken {
		t.Error("el refresh debe rotar (token nuevo)")
	}
	if _, err := uc.Refresh(ctx, pair.RefreshToken); !errors.Is(err, domain.ErrInvalidRefreshToken) {
		t.Errorf("reutilización: err = %v, esperado ErrInvalidRefreshToken", err)
	}
}

// Logout revoca; un refresh ya revocado deja de servir.
func TestLogoutRevoca(t *testing.T) {
	uc, _, _ := setup(t, true)
	ctx := context.Background()
	_, _ = uc.Register(ctx, RegisterInput{Email: "x@b.co", Password: "12345678", Role: "STORE", Store: storeProfile("Principal")})
	pair, _ := uc.Login(ctx, "x@b.co", "12345678")

	if err := uc.Logout(ctx, pair.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := uc.Refresh(ctx, pair.RefreshToken); !errors.Is(err, domain.ErrInvalidRefreshToken) {
		t.Errorf("tras logout: err = %v", err)
	}
}

// Token de invitado: STORE/PROVIDER con orden accesible; DRIVER nunca.
func TestGuestToken(t *testing.T) {
	uc, _, guests := setup(t, true)
	ctx := context.Background()

	store := authz.Principal{TokenType: authz.AccessToken, Role: authz.RoleStore, StoreID: "s1"}
	out, err := uc.GuestToken(ctx, store, "orden-9")
	if err != nil {
		t.Fatalf("guest token STORE: %v", err)
	}
	if out.Token == "" || out.OrderID != "orden-9" {
		t.Errorf("out = %+v", out)
	}
	if guests.saved != 1 {
		t.Error("el token emitido debe registrarse en BD (auditoría)")
	}

	driver := authz.Principal{TokenType: authz.AccessToken, Role: authz.RoleDriver}
	if _, err := uc.GuestToken(ctx, driver, "orden-9"); !errors.Is(err, domain.ErrOrderAccess) {
		t.Errorf("DRIVER: err = %v, esperado ErrOrderAccess", err)
	}

	// Un invitado no puede emitir más invitados (no escalada de privilegios).
	guest := authz.Principal{TokenType: authz.GuestToken, Scope: "order:orden-9"}
	if _, err := uc.GuestToken(ctx, guest, "orden-10"); !errors.Is(err, domain.ErrOrderAccess) {
		t.Errorf("invitado: err = %v, esperado ErrOrderAccess", err)
	}
}

// Si la orden no es accesible para el rol, no se emite token.
func TestGuestTokenOrdenNoAccesible(t *testing.T) {
	uc, _, guests := setup(t, false)
	store := authz.Principal{TokenType: authz.AccessToken, Role: authz.RoleStore, StoreID: "s1"}
	if _, err := uc.GuestToken(context.Background(), store, "orden-ajena"); !errors.Is(err, domain.ErrOrderAccess) {
		t.Fatalf("err = %v, esperado ErrOrderAccess", err)
	}
	if guests.saved != 0 {
		t.Error("no debe registrarse un token no emitido")
	}
}
