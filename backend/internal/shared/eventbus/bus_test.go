package eventbus

import (
	"testing"
	"time"
)

func recv(t *testing.T, ch <-chan Message) Message {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(time.Second):
		t.Fatal("timeout esperando mensaje")
		return Message{}
	}
}

// Publicar entrega a todos los suscriptores del topic, no a otros topics.
func TestPublishEntregaSoloAlTopic(t *testing.T) {
	b := New()
	a, cancelA := b.Subscribe("vehicle:1", 4)
	defer cancelA()
	c, cancelC := b.Subscribe("vehicle:2", 4)
	defer cancelC()

	b.Publish("vehicle:1", []byte(`{"lat":4.6}`))

	m := recv(t, a)
	if string(m.Payload) != `{"lat":4.6}` || m.Topic != "vehicle:1" {
		t.Errorf("mensaje = %+v", m)
	}
	select {
	case m := <-c:
		t.Errorf("topic ajeno recibió %+v", m)
	case <-time.After(50 * time.Millisecond):
	}
}

// Cancelar deja de entregar y elimina el topic cuando queda vacío.
func TestCancelDesuscribe(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe("order:1", 1)
	cancel()
	cancel() // idempotente

	b.Publish("order:1", []byte("x"))
	if _, ok := <-ch; ok {
		t.Error("el canal debe estar cerrado")
	}
	if b.Topics() != 0 {
		t.Errorf("topics = %d, esperado 0", b.Topics())
	}
}

// Un suscriptor lento no bloquea Publish: se descarta y se contabiliza.
func TestSuscriptorLentoNoBloquea(t *testing.T) {
	b := New()
	_, cancel := b.Subscribe("order:1", 1)
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			b.Publish("order:1", []byte("x"))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish bloqueó con un suscriptor lento")
	}
	if b.Dropped() == 0 {
		t.Error("se esperaban mensajes descartados")
	}
	if b.Published() != 10 {
		t.Errorf("Published = %d", b.Published())
	}
}

// Publish sin suscriptores es inocuo.
func TestPublishSinSuscriptores(t *testing.T) {
	b := New()
	b.Publish("nadie", []byte("x"))
	if b.Dropped() != 0 {
		t.Errorf("Dropped = %d", b.Dropped())
	}
}
