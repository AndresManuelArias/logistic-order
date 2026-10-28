package domain

import (
	"errors"
	"testing"
	"time"
)

// La máquina de estados solo admite las transiciones documentadas (RF-05).
func TestTransicionesPermitidas(t *testing.T) {
	allowed := map[[2]Status]bool{
		{StatusCreated, StatusInTransit}:   true,
		{StatusCreated, StatusCancelled}:   true,
		{StatusInTransit, StatusDelivered}: true,
		{StatusInTransit, StatusCancelled}: true,
	}
	all := []Status{StatusCreated, StatusInTransit, StatusDelivered, StatusCancelled}
	for _, from := range all {
		for _, to := range all {
			want := allowed[[2]Status{from, to}]
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s, %s) = %v, esperado %v", from, to, got, want)
			}
		}
	}
}

// Los estados finales no admiten ninguna transición.
func TestEstadosFinalesInmutables(t *testing.T) {
	for _, from := range []Status{StatusDelivered, StatusCancelled} {
		if got := AllowedTransitions(from); len(got) != 0 {
			t.Errorf("AllowedTransitions(%s) = %v, esperado vacío", from, got)
		}
	}
}

// Estados desconocidos o de idénticos nunca son válidos.
func TestTransicionesInvalidas(t *testing.T) {
	if CanTransition("QUEUED", StatusDelivered) {
		t.Error("estado desconocido no debe transicionar")
	}
	if CanTransition(StatusCreated, StatusCreated) {
		t.Error("transición a idéntico estado no debe permitirse")
	}
	if CanTransition(StatusDelivered, StatusCreated) {
		t.Error("no se puede reabrir una orden entregada")
	}
}

// Transition aplica hitos temporales coherentes con el estado.
func TestTransitionHitosTemporales(t *testing.T) {
	o := &Order{Status: StatusCreated}
	at := time.Now()

	if err := o.Transition(StatusInTransit, at); err != nil {
		t.Fatalf("CREATED→IN_TRANSIT: %v", err)
	}
	if o.PickedUpAt == nil || !o.PickedUpAt.Equal(at) {
		t.Error("IN_TRANSIT debe fijar picked_up_at")
	}
	if o.DeliveredAt != nil {
		t.Error("IN_TRANSIT no debe fijar delivered_at")
	}

	if err := o.Transition(StatusDelivered, at.Add(time.Hour)); err != nil {
		t.Fatalf("IN_TRANSIT→DELIVERED: %v", err)
	}
	if o.DeliveredAt == nil {
		t.Error("DELIVERED debe fijar delivered_at")
	}
}

// Una transición prohibida devuelve ErrInvalidTransition envuelto (errors.Is).
func TestTransitionProhibida(t *testing.T) {
	o := &Order{Status: StatusDelivered}
	err := o.Transition(StatusInTransit, time.Now())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("error = %v, esperado ErrInvalidTransition", err)
	}
}

// Valid rechaza coordenadas fuera de rango y el punto nulo (0,0).
func TestPointValid(t *testing.T) {
	valid := []Point{{4.65, -74.05}, {-33.4, -70.6}, {0, -74.05}}
	for _, p := range valid {
		if !p.Valid() {
			t.Errorf("Point%v debería ser válida", p)
		}
	}
	invalid := []Point{
		{0, 0},          // nulo: sin georreferenciar
		{91, 0},         // lat fuera de rango
		{0, -181},       // lon fuera de rango
		{1e300, -74.05}, // overflow
	}
	for _, p := range invalid {
		if p.Valid() {
			t.Errorf("Point%v no debería ser válida", p)
		}
	}
}
