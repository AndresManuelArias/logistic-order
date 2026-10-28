package websocket

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"tiendas-track/backend/internal/shared/authz"
)

// Límites del protocolo WS. El read limit es corto: solo llegan comandos.
const (
	maxMessageBytes = 4096
	closeNormal     = websocket.CloseNormalClosure
	closeTokenExp   = 4401
	closeSlow       = websocket.ClosePolicyViolation
)

// inbound es un mensaje del cliente.
type inbound struct {
	Action  string `json:"action"`
	Channel string `json:"channel,omitempty"`
}

// outbound es un mensaje del servidor.
type outbound struct {
	Type    string          `json:"type"`
	Channel string          `json:"channel,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Code    string          `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	At      string          `json:"at,omitempty"`
}

// client es una conexión WS autenticada.
type client struct {
	hub       *Hub
	conn      *websocket.Conn
	principal authz.Principal

	out  chan []byte
	done chan struct{}

	closeOnce sync.Once
	mu        sync.Mutex
	grants    map[string]time.Time // canal → expiración de la concesión
}

func newClient(h *Hub, conn *websocket.Conn, p authz.Principal) *client {
	return &client{
		hub:       h,
		conn:      conn,
		principal: p,
		out:       make(chan []byte, h.sendBuffer),
		done:      make(chan struct{}),
		grants:    make(map[string]time.Time),
	}
}

// readLoop procesa SUBSCRIBE/UNSUBSCRIBE/PING del cliente.
func (c *client) readLoop() {
	c.conn.SetReadLimit(maxMessageBytes)
	_ = c.conn.SetReadDeadline(time.Now().Add(c.hub.opts.PongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(c.hub.opts.PongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.close(closeNormal, "lectura terminada")
			return
		}

		var in inbound
		if err := json.Unmarshal(data, &in); err != nil {
			c.sendError("INVALID_MESSAGE", "mensaje JSON inválido")
			continue
		}

		switch in.Action {
		case "subscribe":
			ch, ok := authz.ParseChannel(in.Channel)
			if !ok {
				c.sendError("INVALID_CHANNEL", "canal inválido (formato <tipo>:<uuid>)")
				continue
			}
			c.hub.subscribe(c, ch)
		case "unsubscribe":
			if _, ok := authz.ParseChannel(in.Channel); !ok {
				c.sendError("INVALID_CHANNEL", "canal inválido (formato <tipo>:<uuid>)")
				continue
			}
			c.hub.unsubscribe(c, in.Channel)
		case "ping":
			c.enqueue(outbound{Type: "pong"})
		default:
			c.sendError("UNKNOWN_ACTION", "acción no soportada (subscribe|unsubscribe|ping)")
		}
	}
}

// writeLoop es el ÚNICO escritor del socket (gorilla no permite escrituras
// concurrentes): eventos, pongs y pings de keepalive.
func (c *client) writeLoop() {
	ticker := time.NewTicker(c.hub.opts.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case msg := <-c.out:
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.hub.opts.WriteTimeout))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				c.close(closeNormal, "escritura fallida")
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.hub.opts.WriteTimeout))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.close(closeNormal, "ping fallido")
				return
			}
		case <-c.done:
			return
		}
	}
}

// armExpiry cierra la sesión cuando expira el token que la autenticó.
func (c *client) armExpiry() {
	if c.principal.ExpiresAt.IsZero() {
		return
	}
	d := time.Until(c.principal.ExpiresAt)
	if d <= 0 {
		c.sendError("TOKEN_EXPIRED", "el token de la sesión expiró")
		c.close(closeTokenExp, "token expirado")
		return
	}
	time.AfterFunc(d, func() {
		c.sendError("TOKEN_EXPIRED", "el token de la sesión expiró; reconecta")
		c.close(closeTokenExp, "token expirado")
	})
}

// close cierra la conexión una sola vez y desregistra al cliente.
func (c *client) close(code int, reason string) {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, reason),
			time.Now().Add(time.Second),
		)
		_ = c.conn.Close()
		c.hub.unregister(c)
	})
}

// enqueue encola una respuesta sin bloquear; si el cliente no consume, se cierra.
func (c *client) enqueue(m outbound) {
	select {
	case c.out <- encode(m):
	case <-c.done:
	default:
		c.hub.log.Warn("ws: cliente lento, se cierra",
			"user_id", c.principal.UserID)
		c.close(closeSlow, "consumidor lento")
	}
}

func (c *client) sendEvent(m outbound) { c.enqueue(m) }

func (c *client) sendError(code, message string) {
	c.enqueue(outbound{Type: "error", Code: code, Message: message})
}

func (c *client) addGrant(channel string, exp time.Time) {
	c.mu.Lock()
	c.grants[channel] = exp
	c.mu.Unlock()
}

func (c *client) removeGrant(channel string) {
	c.mu.Lock()
	delete(c.grants, channel)
	c.mu.Unlock()
}

func (c *client) grantValid(channel string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	exp, ok := c.grants[channel]
	return ok && time.Now().Before(exp)
}

func (c *client) subscriptionCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.grants)
}

func (c *client) subscribedChannels() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.grants))
	for ch := range c.grants {
		out = append(out, ch)
	}
	return out
}
