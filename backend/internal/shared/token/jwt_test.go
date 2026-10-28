package token

import (
	"strings"
	"testing"
	"time"
)

func newSvc(t *testing.T) *Service {
	t.Helper()
	svc, err := New("secreto-de-desarrollo-con-mas-de-32-caracteres", 15*time.Minute, 720*time.Hour, 48*time.Hour)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

// Un secreto corto es un error de configuración, no una advertencia.
func TestNewRechazaSecretoCorto(t *testing.T) {
	if _, err := New("corto", time.Minute, time.Hour, time.Hour); err == nil {
		t.Fatal("se esperaba error con secreto < 32 caracteres")
	}
}

// El par emitido-parseado conserva identidad, rol y tipo de token.
func TestEmitirYParsearAccess(t *testing.T) {
	svc := newSvc(t)
	issued, err := svc.IssueAccess("user-1", Identity{Role: "STORE", StoreID: "store-9"})
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}
	claims, err := svc.Parse(issued.Raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.Kind != KindAccess || claims.Subject != "user-1" ||
		claims.Role != "STORE" || claims.StoreID != "store-9" || claims.ID != issued.JTI {
		t.Errorf("claims = %+v, no conserva la identidad emitida", claims)
	}
}

// El token de invitado lleva scope order:<id> y sin rol de usuario.
func TestEmitirGuestScope(t *testing.T) {
	svc := newSvc(t)
	issued, err := svc.IssueGuest("orden-42")
	if err != nil {
		t.Fatalf("IssueGuest: %v", err)
	}
	claims, err := svc.Parse(issued.Raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.Kind != KindGuest || claims.Scope != "order:orden-42" {
		t.Errorf("claims = %+v, esperado guest con scope order:orden-42", claims)
	}
	if claims.Role != "" {
		t.Error("un invitado no debe recibir rol")
	}
}

// Firma manipulada o secreto ajeno → rechazo (nunca claims sin validar).
func TestParseRechazaTokenManipulado(t *testing.T) {
	svc := newSvc(t)
	other, _ := New("otro-secreto-tambien-largo-1234567890abcd", time.Minute, time.Hour, time.Hour)

	issued, _ := svc.IssueAccess("user-1", Identity{Role: "DRIVER"})
	if _, err := other.Parse(issued.Raw); err == nil {
		t.Fatal("un secreto distinto no debe validar la firma")
	}

	tampered := issued.Raw[:len(issued.Raw)-3] + "abc"
	if _, err := svc.Parse(tampered); err == nil {
		t.Fatal("un token alterado debe rechazarse")
	}
	if _, err := svc.Parse("no-es-un-jwt"); err == nil {
		t.Fatal("basura debe rechazarse")
	}
}

// Un token con typ de refresh no debe pasar como access.
func TestParseDistingueTipo(t *testing.T) {
	svc := newSvc(t)
	r, _ := svc.IssueRefresh("user-1")
	claims, err := svc.Parse(r.Raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.Kind != KindRefresh {
		t.Errorf("Kind = %q, esperado refresh", claims.Kind)
	}
	if claims.Role != "" {
		t.Error("el refresh no debe llevar rol")
	}
}

// El JTI es aleatorio: dos emisiones nunca comparten ID.
func TestJTIDistintos(t *testing.T) {
	svc := newSvc(t)
	a, _ := svc.IssueRefresh("u")
	b, _ := svc.IssueRefresh("u")
	if a.JTI == b.JTI || a.Raw == b.Raw {
		t.Error("dos emisiones deben ser únicas")
	}
	if len(a.JTI) < 16 {
		t.Errorf("JTI demasiado corto: %q", a.JTI)
	}
}

// Los TTL expuestos se usan en la respuesta de login (expires_in).
func TestTTLsExpuestos(t *testing.T) {
	svc := newSvc(t)
	if svc.AccessTTL() != 15*time.Minute || svc.GuestTTL() != 48*time.Hour {
		t.Errorf("TTLs = %v / %v", svc.AccessTTL(), svc.GuestTTL())
	}
	if !strings.HasPrefix("Bearer ", "Bearer ") {
		t.Fatal("sanity")
	}
}
