package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tiendas-track/backend/internal/aiagent/domain"
)

func TestCompleteParseaToolCall(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("body inválido: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"model": "fake-1",
			"choices": [{
				"message": {
					"role": "assistant",
					"content": null,
					"tool_calls": [{
						"id": "call-1",
						"type": "function",
						"function": {
							"name": "get_stopped_vehicles",
							"arguments": "{\"minutes\":20}"
						}
					}]
				}
			}]
		}`)
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, APIKey: "secret", Timeout: time.Second})
	resp, err := client.Complete(context.Background(), domain.ChatRequest{
		Model:       "fake-1",
		Temperature: 0,
		MaxTokens:   400,
		Messages:    []domain.Message{{Role: domain.RoleUser, Content: "hola"}},
		Tools: []domain.ToolDefinition{{
			Name:        "get_stopped_vehicles",
			Description: "detenidos",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp.Model != "fake-1" || len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("respuesta = %+v", resp)
	}
	call := resp.Message.ToolCalls[0]
	if call.ID != "call-1" || call.Name != "get_stopped_vehicles" || call.Arguments != `{"minutes":20}` {
		t.Errorf("tool call = %+v", call)
	}

	if got["model"] != "fake-1" {
		t.Errorf("model enviado = %v", got["model"])
	}
	if got["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %v", got["tool_choice"])
	}
	tools, ok := got["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Errorf("tools enviadas = %v", got["tools"])
	}
}

func TestCompleteErrorProveedor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"rate limit"}}`)
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, Timeout: time.Second})
	_, err := client.Complete(context.Background(), domain.ChatRequest{
		Model:    "fake-1",
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "hola"}},
	})
	if !errors.Is(err, domain.ErrProvider) {
		t.Fatalf("error = %v, esperado ErrProvider", err)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("el error debe incluir el status: %v", err)
	}
}

func TestCompleteSinChoicesEsErrorProveedor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"model":"fake-1","choices":[]}`)
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, Timeout: time.Second})
	_, err := client.Complete(context.Background(), domain.ChatRequest{Model: "fake-1"})
	if !errors.Is(err, domain.ErrProvider) {
		t.Fatalf("error = %v, esperado ErrProvider", err)
	}
}

func TestCompleteTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"tarde"}}]}`)
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, Timeout: 10 * time.Millisecond})
	_, err := client.Complete(context.Background(), domain.ChatRequest{Model: "fake-1"})
	if !errors.Is(err, domain.ErrTimeout) {
		t.Fatalf("error = %v, esperado ErrTimeout", err)
	}
}

func TestCompleteSinAPIKeyNoEnviaAuthorization(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization inesperado: %q", r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, Timeout: time.Second})
	if _, err := client.Complete(context.Background(), domain.ChatRequest{Model: "fake-1"}); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
}

func TestNewNormalizaURL(t *testing.T) {
	for _, base := range []string{
		"http://localhost:11434/v1/",
		"http://localhost:11434/v1/chat/completions",
	} {
		c := New(Config{BaseURL: base})
		if c.endpoint != "http://localhost:11434/v1/chat/completions" {
			t.Errorf("base %q: endpoint = %q", base, c.endpoint)
		}
	}
}
