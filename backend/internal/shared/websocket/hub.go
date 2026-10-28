// Package websocket implementa el hub de tiempo real del monolito (ADR-03).
//
// Responsabilidades:
//   - Aceptar conexiones autenticadas y mantener el ciclo de vida del cliente.
//   - Autorizar CADA suscripción contra el scope del token (nunca confiar en
//     el cliente) y revalidar las concesiones mientras la sesión vive.
//   - Distribuir los eventos del bus en memoria (< 500 ms, RNF-04) solo a los
//     clientes suscritos al canal correspondiente.
//
// El hub no conoce módulos de negocio: la autorización se inyecta como
// interfaz Authorizer (implementada por el caso de uso de telemetría).
package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/eventbus"
	"tiendas-track/backend/internal/shared/httperr"
)

// Authorizer decide si un principal puede suscribirse a un canal.
type Authorizer interface {
	CanSubscribe(ctx context.Context, p authz.Principal, channel authz.Channel) (bool, error)
}

// Options configura el hub.
type Options struct {
	Bus        *eventbus.Bus
	Authorizer Authorizer
	Logger     *slog.Logger

	// AllowedOrigins son orígenes extra permitidos para el upgrade.
	// Vacío = solo clientes sin Origin (apps nativas) y mismo host.
	AllowedOrigins []string

	// GrantTTL es la validez de una autorización ya concedida antes de
	// revalidarla contra el caso de uso. Default 60 s.
	GrantTTL time.Duration
	// WriteTimeout / PongWait / PingInterval controlan la salud del socket.
	WriteTimeout time.Duration
	PongWait     time.Duration
	PingInterval time.Duration
	// MaxSubscriptions limita canales por conexión (default 32).
	MaxSubscriptions int
	// SendBuffer es la cola de salida por cliente (default 256).
	SendBuffer int
}

// Hub mantiene clientes, suscripciones y el fan-out de eventos.
type Hub struct {
	log        *slog.Logger
	bus        *eventbus.Bus
	auth       Authorizer
	upgrader   websocket.Upgrader
	opts       Options
	grantTTL   time.Duration
	sendBuffer int

	mu      sync.Mutex
	clients map[*client]struct{}
	topics  map[string]*topicState
}

type topicState struct {
	cancel  func()
	clients map[*client]struct{}
}

// NewHub crea un hub con la configuración dada (valores por defecto seguros).
func NewHub(opts Options) *Hub {
	if opts.GrantTTL <= 0 {
		opts.GrantTTL = 60 * time.Second
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = 10 * time.Second
	}
	if opts.PongWait <= 0 {
		opts.PongWait = 60 * time.Second
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = 25 * time.Second
	}
	if opts.MaxSubscriptions <= 0 {
		opts.MaxSubscriptions = 32
	}
	if opts.SendBuffer <= 0 {
		opts.SendBuffer = 256
	}

	h := &Hub{
		log:        opts.Logger,
		bus:        opts.Bus,
		auth:       opts.Authorizer,
		opts:       opts,
		grantTTL:   opts.GrantTTL,
		sendBuffer: opts.SendBuffer,
		clients:    make(map[*client]struct{}),
		topics:     make(map[string]*topicState),
	}
	h.upgrader = websocket.Upgrader{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   1024,
		WriteBufferSize:  4096,
		CheckOrigin:      h.checkOrigin,
	}
	return h
}

// Handler devuelve el handler HTTP de /ws. authenticate resuelve el principal
// a partir de la petición (Bearer o access_token en query); si falla → 401.
func (h *Hub) Handler(authenticate func(*http.Request) (authz.Principal, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := authenticate(r)
		if err != nil {
			httperr.Unauthenticated(w, "token requerido o inválido")
			return
		}

		conn, err := h.upgrader.Upgrade(w, r, nil)
		if err != nil {
			h.log.Warn("ws: upgrade fallido", "error", err)
			return
		}

		c := newClient(h, conn, p)
		h.register(c)
		h.log.Info("ws: cliente conectado",
			"user_id", p.UserID, "role", p.Role, "guest", p.IsGuest())

		go c.writeLoop()
		go c.readLoop()
		c.armExpiry()
	})
}

// Stats devuelve contadores para observabilidad (tests y /readyz extendido).
func (h *Hub) Stats() (clients, topics int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients), len(h.topics)
}

// Close cierra todas las conexiones (shutdown del servidor).
func (h *Hub) Close() {
	h.mu.Lock()
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		c.close(closeNormal, "servidor detenido")
	}
}

func (h *Hub) register(c *client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

// unregister quita al cliente de todos los topics y libera las suscripciones
// al bus que queden sin clientes.
func (h *Hub) unregister(c *client) {
	channels := c.subscribedChannels()

	h.mu.Lock()
	delete(h.clients, c)
	for _, ch := range channels {
		if ts := h.topics[ch]; ts != nil {
			delete(ts.clients, c)
			if len(ts.clients) == 0 {
				if ts.cancel != nil {
					ts.cancel()
				}
				delete(h.topics, ch)
			}
		}
	}
	h.mu.Unlock()
}

func (h *Hub) subscribe(c *client, ch authz.Channel) {
	name := ch.String()
	if c.subscriptionCount() >= h.opts.MaxSubscriptions {
		c.sendError("TOO_MANY_SUBSCRIPTIONS", "límite de canales por conexión alcanzado")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	allowed, err := h.auth.CanSubscribe(ctx, c.principal, ch)
	cancel()
	if err != nil {
		h.log.Error("ws: error autorizando suscripción",
			"channel", name, "error", err.Error())
		c.sendError("INTERNAL", "no se pudo autorizar el canal")
		return
	}
	if !allowed {
		h.log.Warn("ws: suscripción denegada",
			"user_id", c.principal.UserID, "role", c.principal.Role, "channel", name)
		c.sendError("FORBIDDEN_SCOPE", "el token no autoriza el canal "+name)
		return
	}

	h.mu.Lock()
	ts := h.topics[name]
	if ts == nil {
		msgCh, cancelSub := h.bus.Subscribe(name, eventbus.DefaultBuffer)
		ts = &topicState{cancel: cancelSub, clients: make(map[*client]struct{})}
		h.topics[name] = ts
		go h.fanout(name, msgCh)
	}
	ts.clients[c] = struct{}{}
	h.mu.Unlock()

	c.addGrant(name, time.Now().Add(h.grantTTL))
	c.sendEvent(outbound{Type: "subscribed", Channel: name})
}

func (h *Hub) unsubscribe(c *client, name string) {
	c.removeGrant(name)

	h.mu.Lock()
	ts := h.topics[name]
	if ts != nil {
		delete(ts.clients, c)
		if len(ts.clients) == 0 {
			if ts.cancel != nil {
				ts.cancel()
			}
			delete(h.topics, name)
		}
	}
	h.mu.Unlock()

	c.sendEvent(outbound{Type: "unsubscribed", Channel: name})
}

// fanout consume el bus para un canal y entrega a los clientes suscritos.
// Si la concesión de un cliente caducó, se revalida fuera del camino de entrega
// (no se añade latencia al mensaje actual).
func (h *Hub) fanout(channel string, msgs <-chan eventbus.Message) {
	for msg := range msgs {
		h.mu.Lock()
		ts := h.topics[channel]
		var targets []*client
		if ts != nil {
			targets = make([]*client, 0, len(ts.clients))
			for c := range ts.clients {
				targets = append(targets, c)
			}
		}
		h.mu.Unlock()

		for _, c := range targets {
			if c.grantValid(channel) {
				c.enqueue(outbound{
					Type:    "event",
					Channel: channel,
					Data:    msg.Payload,
					At:      msg.At.UTC().Format(time.RFC3339Nano),
				})
				continue
			}
			// Concesión caducada: se revalida en segundo plano y se descarta
			// este evento (fail-closed, no se filtra información).
			go h.revalidate(c, channel)
		}
	}
}

// revalidate vuelve a preguntar al dominio si el cliente conserva el scope.
func (h *Hub) revalidate(c *client, channel string) {
	ch, ok := authz.ParseChannel(channel)
	if !ok {
		h.unsubscribe(c, channel)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	allowed, err := h.auth.CanSubscribe(ctx, c.principal, ch)
	cancel()
	if err != nil || !allowed {
		h.log.Warn("ws: concesión revocada", "channel", channel, "user_id", c.principal.UserID)
		h.unsubscribe(c, channel)
		c.sendError("FORBIDDEN_SCOPE", "el scope del canal ya no es válido")
		return
	}
	c.addGrant(channel, time.Now().Add(h.grantTTL))
}

// checkOrigin aplica una política de origen explícita (CSRF de WebSocket).
func (h *Hub) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // apps nativas / clientes sin navegador
	}
	for _, allowed := range h.opts.AllowedOrigins {
		if allowed == "*" || strings.EqualFold(strings.TrimRight(allowed, "/"), strings.TrimRight(origin, "/")) {
			return true
		}
	}
	// Mismo host (Swagger UI y pruebas locales).
	host := strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "https://")
	return strings.EqualFold(host, r.Host)
}

// encode serializa una respuesta del servidor.
func encode(m outbound) []byte {
	b, err := json.Marshal(m)
	if err != nil {
		return []byte(`{"type":"error","code":"INTERNAL","message":"serialización"}`)
	}
	return b
}
