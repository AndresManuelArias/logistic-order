// Package eventbus implementa un bus de eventos en memoria con Go channels.
// Es el mecanismo de desacoplamiento interno del monolito (ADR-01): los
// módulos publican hechos de dominio y otros se suscriben sin conocerse.
//
// Reglas de diseño:
//   - Publish nunca bloquea: si un suscriptor está lento se descarta el mensaje
//     (se contabiliza) para no propagar la lentitud al camino de escritura.
//   - Cada suscriptor recibe su propia copia del payload ([]byte inmutable).
//   - No hay persistencia: los eventos en vivo no reemplazan a la BD.
package eventbus

import (
	"sync"
	"sync/atomic"
	"time"
)

// DefaultBuffer es la capacidad por suscriptor (mensajes en vuelo).
const DefaultBuffer = 256

// Message es un evento publicado en un topic.
type Message struct {
	Topic   string
	Payload []byte
	At      time.Time
}

// Bus es un pub/sub en memoria, seguro para uso concurrente.
type Bus struct {
	mu           sync.RWMutex
	topics       map[string]*topic
	nextID       uint64
	dropped      atomic.Uint64
	publishCount atomic.Uint64
}

type topic struct {
	subs map[uint64]*subscription
}

type subscription struct {
	ch chan Message
}

// New crea un bus vacío.
func New() *Bus {
	return &Bus{topics: make(map[string]*topic)}
}

// Subscribe registra un suscriptor en el topic y devuelve su canal y la
// función de cancelación (idempotente). buffer <= 0 usa DefaultBuffer.
func (b *Bus) Subscribe(topicName string, buffer int) (<-chan Message, func()) {
	if buffer <= 0 {
		buffer = DefaultBuffer
	}
	sub := &subscription{ch: make(chan Message, buffer)}

	b.mu.Lock()
	t := b.topics[topicName]
	if t == nil {
		t = &topic{subs: make(map[uint64]*subscription)}
		b.topics[topicName] = t
	}
	id := b.nextID
	b.nextID++
	t.subs[id] = sub
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			if t := b.topics[topicName]; t != nil {
				delete(t.subs, id)
				if len(t.subs) == 0 {
					delete(b.topics, topicName)
				}
			}
			b.mu.Unlock()
			close(sub.ch)
		})
	}
	return sub.ch, cancel
}

// Publish entrega el mensaje a los suscriptores actuales del topic sin bloquear.
// Los suscriptores lentos (buffer lleno) pierden el mensaje; se cuenta en Dropped.
func (b *Bus) Publish(topicName string, payload []byte) {
	msg := Message{Topic: topicName, Payload: payload, At: time.Now()}
	b.publishCount.Add(1)

	b.mu.RLock()
	t := b.topics[topicName]
	if t == nil {
		b.mu.RUnlock()
		return
	}
	targets := make([]*subscription, 0, len(t.subs))
	for _, s := range t.subs {
		targets = append(targets, s)
	}
	b.mu.RUnlock()

	for _, s := range targets {
		select {
		case s.ch <- msg:
		default:
			b.dropped.Add(1)
		}
	}
}

// Dropped devuelve cuántos mensajes se descartaron por saturación.
func (b *Bus) Dropped() uint64 { return b.dropped.Load() }

// Published devuelve cuántos mensajes se publicaron (incluidos los descartados).
func (b *Bus) Published() uint64 { return b.publishCount.Load() }

// Topics devuelve cuántos topics tienen suscriptores (para observabilidad).
func (b *Bus) Topics() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.topics)
}
