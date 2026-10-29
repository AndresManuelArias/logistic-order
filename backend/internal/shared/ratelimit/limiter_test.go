package ratelimit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestAllowHastaCapacidadYLuegoBloquea(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	l := NewWithClock(3, clock.now)

	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("k"); !ok {
			t.Fatalf("petición %d debería permitirse", i+1)
		}
	}
	ok, retry := l.Allow("k")
	if ok {
		t.Fatal("la cuarta petición debe bloquearse")
	}
	if retry <= 0 {
		t.Fatalf("retryAfter = %v, esperado > 0", retry)
	}
}

func TestAllowReponeTokensConElTiempo(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	l := NewWithClock(60, clock.now) // 1 token/s

	for i := 0; i < 60; i++ {
		l.Allow("k")
	}
	if ok, _ := l.Allow("k"); ok {
		t.Fatal("sin tokens no debe permitir")
	}
	clock.advance(time.Second)
	if ok, _ := l.Allow("k"); !ok {
		t.Fatal("tras 1s debe permitir una petición")
	}
	if ok, _ := l.Allow("k"); ok {
		t.Fatal("solo se repone un token por segundo")
	}
}

func TestClavesIndependientes(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	l := NewWithClock(1, clock.now)

	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("clave a debe permitir")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("clave b es independiente y debe permitir")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("clave a ya consumió su token")
	}
}

func TestCapacidadCeroSiemprePermite(t *testing.T) {
	l := New(0)
	for i := 0; i < 100; i++ {
		if ok, _ := l.Allow("k"); !ok {
			t.Fatal("capacidad 0 desactiva el limitador")
		}
	}
}

func TestGCEliminaClavesInactivas(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	l := NewWithClock(10, clock.now)
	l.Allow("vieja")
	if l.Len() != 1 {
		t.Fatalf("Len = %d, esperado 1", l.Len())
	}
	// Avanza más allá del TTL y fuerza una pasada de GC con otra clave.
	clock.advance(11 * time.Minute)
	l.Allow("nueva")
	if got := l.Len(); got != 1 {
		t.Fatalf("Len tras GC = %d, esperado 1 (solo la nueva)", got)
	}
}

func TestMiddleware429YRetryAfter(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	l := NewWithClock(1, clock.now)
	called := 0
	h := Middleware(l, func(*http.Request) string { return "ip:1.2.3.4" })(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called++
			w.WriteHeader(http.StatusOK)
		}))

	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/", nil))
	if first.Code != http.StatusOK || called != 1 {
		t.Fatalf("primera petición: status=%d called=%d", first.Code, called)
	}

	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, esperado 429", second.Code)
	}
	if called != 1 {
		t.Fatal("el handler no debe ejecutarse cuando se limita")
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("falta la cabecera Retry-After")
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if body.Error.Code != "RATE_LIMITED" {
		t.Fatalf("code = %q, esperado RATE_LIMITED", body.Error.Code)
	}
}

func TestClientIPNoConfiaEnXFFPorDefecto(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")

	if got := ClientIP(r, false); got != "10.0.0.5" {
		t.Fatalf("ClientIP sin trustProxy = %q", got)
	}
	if got := ClientIP(r, true); got != "203.0.113.9" {
		t.Fatalf("ClientIP con trustProxy = %q", got)
	}
}
