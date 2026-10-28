package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Tras N fallos consecutivos el circuito abre y deja de llamar a la dependencia.
func TestBreakerAbreYRechaza(t *testing.T) {
	b := New(Settings{Name: "test", FailureThreshold: 2, Timeout: time.Minute, MaxRequests: 1})

	boom := errors.New("bd caída")
	for i := 0; i < 2; i++ {
		if _, err := Do(b, func() (int, error) { return 0, boom }); !errors.Is(err, boom) {
			t.Fatalf("intento %d: err = %v", i, err)
		}
	}
	if b.State() != "open" {
		t.Fatalf("state = %s, esperado open", b.State())
	}

	called := false
	_, err := Do(b, func() (int, error) { called = true; return 1, nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("err = %v, esperado ErrCircuitOpen", err)
	}
	if called {
		t.Error("no debe llamarse a la dependencia con el circuito abierto")
	}
}

// El camino feliz devuelve el valor y no abre el circuito.
func TestBreakerCaminoFeliz(t *testing.T) {
	b := New(DefaultSettings("ok"))
	v, err := Do(b, func() (string, error) { return "punto", nil })
	if err != nil || v != "punto" {
		t.Fatalf("v=%q err=%v", v, err)
	}
	if b.State() != "closed" {
		t.Errorf("state = %s", b.State())
	}
}

// DoCtx no consume el circuito si el contexto ya está cancelado.
func TestDoCtxCancelado(t *testing.T) {
	b := New(Settings{Name: "ctx", FailureThreshold: 1})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := DoCtx(ctx, b, func(context.Context) (int, error) { return 1, nil })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, esperado context.Canceled", err)
	}
	if b.State() != "closed" {
		t.Errorf("state = %s, el circuito no debe abrirse", b.State())
	}
}
