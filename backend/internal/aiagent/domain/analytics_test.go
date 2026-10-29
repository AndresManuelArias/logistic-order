package domain

import (
	"math"
	"testing"
)

func TestETAMinutes(t *testing.T) {
	cases := []struct {
		name     string
		distance float64
		speed    float64
		want     int
	}{
		{"1 km a 30 km/h", 1000, 30, 2},
		{"sin distancia", 0, 30, 0},
		{"distancia negativa", -5, 30, 0},
		{"velocidad desconocida usa default", 1000, 0, 2},
		{"velocidad negativa usa default", 1000, -3, 2},
		{"distancia NaN", math.NaN(), 30, 0},
		{"5 km a 60 km/h", 5000, 60, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ETAMinutes(tc.distance, tc.speed); got != tc.want {
				t.Errorf("ETAMinutes(%v, %v) = %d, esperado %d", tc.distance, tc.speed, got, tc.want)
			}
		})
	}
}

func TestScopeEmpty(t *testing.T) {
	if !(Scope{}).Empty() {
		t.Error("Scope vacío debería ser Empty")
	}
	if (Scope{StoreID: "s"}).Empty() {
		t.Error("Scope con StoreID no debería ser Empty")
	}
}
