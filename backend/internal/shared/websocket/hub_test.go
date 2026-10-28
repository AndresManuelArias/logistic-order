package websocket

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/eventbus"
)

// fakeAuth autoriza según un mapa canal→bool.
type fakeAuth map[string]bool

func (f fakeAuth) CanSubscribe(_ context.Context, _ authz.Principal, ch authz.Channel) (bool, error) {
	return f[ch.String()], nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testPrincipal(exp time.Time) authz.Principal {
	return authz.Principal{
		UserID: "u-1", Role: authz.RoleProvider, TokenType: authz.AccessToken,
		ProviderID: "u-1", ExpiresAt: exp,
	}
}

const testVehicle = "3f2b8a1e-1c4d-4f6a-9b2e-7d8c9a0b1e2f"

// newTestServer arranca el hub con un autorizador y un principal fijos.
func newTestServer(t *testing.T, auth Authorizer, p authz.Principal) (*Hub, *eventbus.Bus, string) {
	t.Helper()
	bus := eventbus.New()
	hub := NewHub(Options{Bus: bus, Authorizer: auth, Logger: testLogger()})
	srv := httptest.NewServer(hub.Handler(func(*http.Request) (authz.Principal, error) {
		return p, nil
	}))
	t.Cleanup(func() { hub.Close(); srv.Close() })
	return hub, bus, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readEnvelope(t *testing.T, conn *websocket.Conn) outbound {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var m outbound
	if err := conn.ReadJSON(&m); err != nil {
		t.Fatalf("read: %v", err)
	}
	return m
}

// Suscripción autorizada: ack + evento del bus en < 500 ms (RNF-04).
func TestHubSuscripcionValidaRecibeEvento(t *testing.T) {
	channel := authz.Channel{Kind: authz.ChannelVehicle, ID: testVehicle}.String()
	_, bus, url := newTestServer(t, fakeAuth{channel: true}, testPrincipal(time.Now().Add(time.Minute)))
	conn := dial(t, url)

	if err := conn.WriteJSON(inbound{Action: "subscribe", Channel: channel}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if ack := readEnvelope(t, conn); ack.Type != "subscribed" || ack.Channel != channel {
		t.Fatalf("ack = %+v", ack)
	}

	start := time.Now()
	bus.Publish(channel, []byte(`{"lat":4.65,"lon":-74.05}`))
	ev := readEnvelope(t, conn)
	elapsed := time.Since(start)

	if ev.Type != "event" || ev.Channel != channel || string(ev.Data) != `{"lat":4.65,"lon":-74.05}` {
		t.Fatalf("evento = %+v", ev)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("latencia de publicación = %v, RNF-04 exige < 500 ms", elapsed)
	}
}

// Canal ajeno: el hub responde error y no entrega eventos (ADR-03).
func TestHubCanalAjenoRechazado(t *testing.T) {
	channel := authz.Channel{Kind: authz.ChannelOrder, ID: testVehicle}.String()
	_, bus, url := newTestServer(t, fakeAuth{}, testPrincipal(time.Now().Add(time.Minute)))
	conn := dial(t, url)

	if err := conn.WriteJSON(inbound{Action: "subscribe", Channel: channel}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	errMsg := readEnvelope(t, conn)
	if errMsg.Type != "error" || errMsg.Code != "FORBIDDEN_SCOPE" {
		t.Fatalf("respuesta = %+v", errMsg)
	}

	bus.Publish(channel, []byte(`{"lat":4.6}`))
	_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	var m outbound
	if err := conn.ReadJSON(&m); err == nil {
		t.Fatalf("no debía recibir eventos, recibió %+v", m)
	}
}

// Formato de canal inválido se rechaza antes de consultar autorización.
func TestHubCanalInvalido(t *testing.T) {
	_, _, url := newTestServer(t, fakeAuth{}, testPrincipal(time.Now().Add(time.Minute)))
	conn := dial(t, url)

	_ = conn.WriteJSON(inbound{Action: "subscribe", Channel: "fleet:provider:no-es-uuid"})
	if m := readEnvelope(t, conn); m.Code != "INVALID_CHANNEL" {
		t.Fatalf("respuesta = %+v", m)
	}
}

// Acción desconocida no cierra la conexión.
func TestHubAccionDesconocida(t *testing.T) {
	_, _, url := newTestServer(t, fakeAuth{}, testPrincipal(time.Now().Add(time.Minute)))
	conn := dial(t, url)

	_ = conn.WriteJSON(inbound{Action: "explode"})
	if m := readEnvelope(t, conn); m.Code != "UNKNOWN_ACTION" {
		t.Fatalf("respuesta = %+v", m)
	}
	_ = conn.WriteJSON(inbound{Action: "ping"})
	if m := readEnvelope(t, conn); m.Type != "pong" {
		t.Fatalf("respuesta = %+v", m)
	}
}

// El unsubscribe corta la entrega y libera el topic del bus.
func TestHubUnsubscribe(t *testing.T) {
	channel := authz.Channel{Kind: authz.ChannelVehicle, ID: testVehicle}.String()
	hub, bus, url := newTestServer(t, fakeAuth{channel: true}, testPrincipal(time.Now().Add(time.Minute)))
	conn := dial(t, url)

	_ = conn.WriteJSON(inbound{Action: "subscribe", Channel: channel})
	readEnvelope(t, conn) // ack
	_ = conn.WriteJSON(inbound{Action: "unsubscribe", Channel: channel})
	if m := readEnvelope(t, conn); m.Type != "unsubscribed" {
		t.Fatalf("respuesta = %+v", m)
	}

	bus.Publish(channel, []byte(`{"lat":4.6}`))
	_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	var m outbound
	if err := conn.ReadJSON(&m); err == nil {
		t.Fatalf("no debía recibir eventos tras unsubscribe: %+v", m)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, topics := hub.Stats(); topics == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("el topic del bus no se liberó tras el último unsubscribe")
}

// La sesión se cierra cuando expira el token que la autenticó.
func TestHubCierraAlExpirarToken(t *testing.T) {
	_, _, url := newTestServer(t, fakeAuth{}, testPrincipal(time.Now().Add(120*time.Millisecond)))
	conn := dial(t, url)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var m outbound
	err := conn.ReadJSON(&m)
	if err == nil && m.Code != "TOKEN_EXPIRED" {
		t.Fatalf("mensaje = %+v", m)
	}
	// Tras el aviso, el servidor cierra: la siguiente lectura falla.
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Error("la conexión debía cerrarse al expirar el token")
	}
}

// Handshake sin autenticación → 401 (no se hace upgrade).
func TestHubSinAutenticacion(t *testing.T) {
	hub := NewHub(Options{Bus: eventbus.New(), Authorizer: fakeAuth{}, Logger: testLogger()})
	srv := httptest.NewServer(hub.Handler(func(*http.Request) (authz.Principal, error) {
		return authz.Principal{}, errors.New("sin token")
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		t.Fatal("el dial debía fallar")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, esperado 401", resp)
	}
}
