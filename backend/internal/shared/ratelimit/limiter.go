// Package ratelimit implementa un limitador de peticiones en memoria,
// thread-safe y sin dependencias externas.
//
// Se usa para proteger login, ingesta de telemetría y el agente IA
// (checklist de seguridad §9.4). Es un token bucket por clave: la capacidad
// es el número de peticiones permitidas por minuto y los tokens se reponen de
// forma continua. El estado vive en el proceso; en un despliegue con varias
// réplicas el límite es por instancia (documentado en la auditoría F6).
package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Limiter es un token bucket por clave.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity float64 // peticiones por ventana (minuto)
	refill   float64 // tokens por segundo
	now      func() time.Time
	lastGC   time.Time
	ttl      time.Duration
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New crea un limitador de perMinute peticiones por minuto.
// Un valor <= 0 desactiva el limitador (Allow siempre true); el wiring evita
// construirlo en ese caso, pero se mantiene el comportamiento seguro.
func New(perMinute int) *Limiter {
	return NewWithClock(perMinute, time.Now)
}

// NewWithClock permite inyectar el reloj en pruebas.
func NewWithClock(perMinute int, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	capacity := float64(perMinute)
	if capacity < 0 {
		capacity = 0
	}
	return &Limiter{
		buckets:  make(map[string]*bucket),
		capacity: capacity,
		refill:   capacity / 60.0,
		now:      now,
		lastGC:   now(),
		ttl:      10 * time.Minute,
	}
}

// Allow consume un token para key. Si no hay tokens devuelve false y el tiempo
// mínimo que hay que esperar para que la siguiente petición sea aceptada.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	if l == nil || l.capacity <= 0 {
		return true, 0
	}

	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	l.gcLocked(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, last: now}
		l.buckets[key] = b
	} else {
		elapsed := now.Sub(b.last).Seconds()
		if elapsed > 0 {
			b.tokens = math.Min(l.capacity, b.tokens+elapsed*l.refill)
			b.last = now
		}
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	missing := 1 - b.tokens
	wait := time.Duration(math.Ceil(missing / l.refill * float64(time.Second)))
	if wait < time.Second {
		wait = time.Second
	}
	return false, wait
}

// gcLocked elimina cubos inactivos para evitar crecimiento no acotado.
// Se ejecuta como máximo una vez por minuto y siempre con el lock tomado.
func (l *Limiter) gcLocked(now time.Time) {
	if now.Sub(l.lastGC) < time.Minute {
		return
	}
	l.lastGC = now
	for k, b := range l.buckets {
		if now.Sub(b.last) > l.ttl {
			delete(l.buckets, k)
		}
	}
}

// Len devuelve el número de claves activas (útil en pruebas y diagnóstico).
func (l *Limiter) Len() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
