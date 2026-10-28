// Package resilience agrupa patrones de tolerancia a fallos (RNF-05).
// El Circuit Breaker evita que una dependencia degradada (la BD en los caminos
// de lectura) arrastre al resto del servicio: tras N fallos seguidos se abre
// el circuito y se responde 503 CIRCUIT_OPEN sin tocar la dependencia.
package resilience

import (
	"context"
	"errors"
	"time"

	"github.com/sony/gobreaker"
)

// ErrCircuitOpen indica que el circuito está abierto o saturado en half-open.
var ErrCircuitOpen = errors.New("circuito abierto: dependencia no disponible")

// Settings configura un breaker.
type Settings struct {
	Name             string
	MaxRequests      uint32        // peticiones permitidas en half-open
	Interval         time.Duration // ventana de conteo en estado cerrado (0 = sin reset)
	Timeout          time.Duration // tiempo que permanece abierto antes de half-open
	FailureThreshold uint32        // fallos consecutivos para abrir
}

// DefaultSettings son valores conservadores para lecturas de BD.
func DefaultSettings(name string) Settings {
	return Settings{
		Name:             name,
		MaxRequests:      3,
		Interval:         30 * time.Second,
		Timeout:          10 * time.Second,
		FailureThreshold: 5,
	}
}

// Breaker envuelve gobreaker y traduce sus errores al dominio del servicio.
type Breaker struct {
	cb *gobreaker.CircuitBreaker
}

// New crea un breaker con la configuración dada. Valores <= 0 usan defaults.
func New(s Settings) *Breaker {
	d := DefaultSettings(s.Name)
	if s.MaxRequests > 0 {
		d.MaxRequests = s.MaxRequests
	}
	if s.Interval > 0 {
		d.Interval = s.Interval
	}
	if s.Timeout > 0 {
		d.Timeout = s.Timeout
	}
	if s.FailureThreshold > 0 {
		d.FailureThreshold = s.FailureThreshold
	}
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        d.Name,
		MaxRequests: d.MaxRequests,
		Interval:    d.Interval,
		Timeout:     d.Timeout,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= d.FailureThreshold
		},
	})
	return &Breaker{cb: cb}
}

// Do ejecuta fn a través del circuito. Si el circuito está abierto devuelve
// ErrCircuitOpen envuelto; si fn falla, el error original se propaga.
func Do[T any](b *Breaker, fn func() (T, error)) (T, error) {
	var zero T

	v, err := b.cb.Execute(func() (any, error) { return fn() })
	if err == nil {
		out, ok := v.(T)
		if !ok {
			return zero, errors.New("resilience: tipo de resultado inesperado")
		}
		return out, nil
	}
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return zero, ErrCircuitOpen
	}
	return zero, err
}

// DoCtx ejecuta fn cancelable. Si el contexto ya está cancelado no consume
// una petición del circuito (el fallo no es de la dependencia).
func DoCtx[T any](ctx context.Context, b *Breaker, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return Do(b, func() (T, error) { return fn(ctx) })
}

// State devuelve el estado actual (closed | half-open | open) como texto.
func (b *Breaker) State() string { return b.cb.State().String() }
