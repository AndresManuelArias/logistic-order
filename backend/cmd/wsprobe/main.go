// Comando wsprobe: sonda de integración para el canal WebSocket de telemetría.
//
// No forma parte del servidor (paquete main independiente): lo usa
// scripts/e2e_f3.sh para verificar, contra un backend real:
//   - el handshake autenticado (header o query),
//   - el rechazo de canales ajenos (FORBIDDEN_SCOPE),
//   - la latencia de publicación de un evento (RNF-04: < 500 ms).
//
// Uso:
//
//	go run ./cmd/wsprobe -token "$AT" -channel "vehicle:<uuid>"
//	go run ./cmd/wsprobe -token "$AT" -channel "order:<uuid>" -expect-error FORBIDDEN_SCOPE
//	go run ./cmd/wsprobe -token "$AT" -channel "vehicle:<uuid>" \
//	    -ingest-url http://localhost:8080/api/v1/telemetry/points \
//	    -ingest-body '{"time":"...","lat":4.65,"lon":-74.08,"device_seq":900001}'
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type clientMessage struct {
	Action  string `json:"action"`
	Channel string `json:"channel"`
}

type serverMessage struct {
	Type    string          `json:"type"`
	Channel string          `json:"channel"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	At      string          `json:"at"`
}

type options struct {
	url            string
	token          string
	channel        string
	expectError    string
	tokenInQuery   bool
	ingestURL      string
	ingestBody     string
	ingestToken    string
	idempotencyKey string
	timeout        time.Duration
	maxLatency     time.Duration
}

func main() {
	opt := options{}
	flag.StringVar(&opt.url, "url", "ws://localhost:8080/ws", "URL del WebSocket")
	flag.StringVar(&opt.token, "token", "", "access token o token de invitado")
	flag.StringVar(&opt.channel, "channel", "", "canal a suscribir (fleet:provider:|fleet:store:|order:|vehicle:)")
	flag.StringVar(&opt.expectError, "expect-error", "", "código de error esperado en la suscripción")
	flag.BoolVar(&opt.tokenInQuery, "token-in-query", false, "enviar el token en ?access_token= (navegadores)")
	flag.StringVar(&opt.ingestURL, "ingest-url", "", "endpoint a invocar tras suscribirse (mide latencia al evento)")
	flag.StringVar(&opt.ingestBody, "ingest-body", "", "cuerpo JSON de la ingesta")
	flag.StringVar(&opt.ingestToken, "ingest-token", "", "token para la ingesta (por defecto el del WS)")
	flag.StringVar(&opt.idempotencyKey, "idempotency-key", "", "header Idempotency-Key de la ingesta")
	flag.DurationVar(&opt.timeout, "timeout", 5*time.Second, "tiempo máximo de espera")
	flag.DurationVar(&opt.maxLatency, "max-latency", 500*time.Millisecond, "presupuesto de latencia (RNF-04)")
	flag.Parse()

	if err := run(opt); err != nil {
		fmt.Printf("FAIL: %v\n", err)
		os.Exit(1)
	}
}

func run(opt options) error {
	if opt.channel == "" {
		return errors.New("-channel es obligatorio")
	}

	header := http.Header{}
	url := opt.url
	if opt.tokenInQuery {
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		url += sep + "access_token=" + opt.token
	} else if opt.token != "" {
		header.Set("Authorization", "Bearer "+opt.token)
	}

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.Dial(url, header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("handshake: HTTP %d (%v)", resp.StatusCode, err)
		}
		return fmt.Errorf("handshake: %w", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(clientMessage{Action: "subscribe", Channel: opt.channel}); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	msg, err := readUntil(conn, opt.timeout, func(m serverMessage) bool {
		return m.Type == "subscribed" || m.Type == "error"
	})
	if err != nil {
		return err
	}

	if opt.expectError != "" {
		if msg.Type != "error" || msg.Code != opt.expectError {
			return fmt.Errorf("esperaba error %s, recibió %+v", opt.expectError, msg)
		}
		fmt.Printf("expect-error=%s channel=%s ok\n", msg.Code, opt.channel)
		return nil
	}
	if msg.Type != "subscribed" || msg.Channel != opt.channel {
		return fmt.Errorf("suscripción no confirmada: %+v", msg)
	}

	if opt.ingestURL == "" {
		fmt.Printf("subscribed channel=%s ok\n", opt.channel)
		return nil
	}

	ingestToken := opt.ingestToken
	if ingestToken == "" {
		ingestToken = opt.token
	}
	start := time.Now()
	status, err := postJSON(opt.ingestURL, opt.ingestBody, ingestToken, opt.idempotencyKey)
	if err != nil {
		return fmt.Errorf("ingesta: %w", err)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("ingesta: HTTP %d", status)
	}

	event, err := readUntil(conn, opt.timeout, func(m serverMessage) bool {
		return m.Type == "event" || m.Type == "error"
	})
	if err != nil {
		return err
	}
	latency := time.Since(start)
	if event.Type != "event" {
		return fmt.Errorf("esperaba evento, recibió %+v", event)
	}
	if event.Channel != opt.channel {
		return fmt.Errorf("evento en canal %q, esperado %q", event.Channel, opt.channel)
	}

	fmt.Printf("event channel=%s at=%s latency_ms=%.1f status=%d\n",
		event.Channel, event.At, float64(latency.Microseconds())/1000.0, status)
	if latency > opt.maxLatency {
		return fmt.Errorf("latencia %.1f ms supera el presupuesto de %s (RNF-04)",
			float64(latency.Microseconds())/1000.0, opt.maxLatency)
	}
	return nil
}

// readUntil lee mensajes del servidor hasta que match devuelva true.
func readUntil(conn *websocket.Conn, timeout time.Duration, match func(serverMessage) bool) (serverMessage, error) {
	deadline := time.Now().Add(timeout)
	for {
		if err := conn.SetReadDeadline(deadline); err != nil {
			return serverMessage{}, err
		}
		var msg serverMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return serverMessage{}, fmt.Errorf("leyendo WS: %w", err)
		}
		if match(msg) {
			return msg, nil
		}
	}
}

func postJSON(url, body, token, idempotencyKey string) (int, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
