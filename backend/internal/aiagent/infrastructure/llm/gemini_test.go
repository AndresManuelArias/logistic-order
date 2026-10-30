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

// respuesta Gemini con una functionCall (el modelo quiere llamar una herramienta).
const geminiToolCallBody = `{
	"candidates": [{
		"content": {
			"role": "model",
			"parts": [{
				"functionCall": {
					"name": "get_stopped_vehicles",
					"args": {"minutes": 20}
				}
			}]
		}
	}],
	"modelVersion": "gemini-1.5-flash-001"
}`

// respuesta Gemini con texto plano (respuesta final del agente).
const geminiTextBody = `{
	"candidates": [{
		"content": {
			"role": "model",
			"parts": [{"text": "No hay vehículos detenidos."}]
		}
	}],
	"modelVersion": "gemini-1.5-flash-001"
}`

func TestGeminiCompleteParseaToolCall(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// La key va en la query, no en el header.
		if r.URL.Query().Get("key") != "gemini-secret" {
			t.Errorf("key query param = %q", r.URL.Query().Get("key"))
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("no debe enviar Authorization header en Gemini")
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, geminiToolCallBody)
	}))
	defer srv.Close()

	client := NewGemini(Config{BaseURL: srv.URL, APIKey: "gemini-secret", Timeout: time.Second})
	resp, err := client.Complete(context.Background(), domain.ChatRequest{
		Model: "gemini-1.5-flash",
		Messages: []domain.Message{
			{Role: domain.RoleSystem, Content: "Eres un agente de logística."},
			{Role: domain.RoleUser, Content: "¿Qué vehículos están detenidos?"},
		},
		Tools: []domain.ToolDefinition{{
			Name:        "get_stopped_vehicles",
			Description: "Vehículos detenidos",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"minutes":{"type":"integer","minimum":1}},"additionalProperties":false}`),
		}},
		Temperature: 0,
		MaxTokens:   400,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("se esperaba 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	call := resp.Message.ToolCalls[0]
	if call.Name != "get_stopped_vehicles" {
		t.Errorf("tool name = %q", call.Name)
	}
	if !strings.Contains(call.Arguments, "20") {
		t.Errorf("arguments = %q, debe contener 20", call.Arguments)
	}
	if !strings.HasPrefix(call.ID, "gemini-") {
		t.Errorf("ID = %q, debe empezar con gemini-", call.ID)
	}
	if resp.Model != "gemini-1.5-flash-001" {
		t.Errorf("model = %q", resp.Model)
	}

	// Verifica que el system prompt fue a systemInstruction (no a contents).
	if si, ok := gotBody["systemInstruction"].(map[string]any); !ok {
		t.Error("systemInstruction ausente en la petición")
	} else {
		parts, _ := si["parts"].([]any)
		if len(parts) == 0 {
			t.Error("systemInstruction sin parts")
		}
	}
	// Verifica que las herramientas se enviaron como functionDeclarations.
	tools, _ := gotBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools en petición = %d, esperado 1", len(tools))
	}
	toolObj, _ := tools[0].(map[string]any)
	declarations, _ := toolObj["functionDeclarations"].([]any)
	if len(declarations) != 1 {
		t.Fatal("tools debe tener una función")
	}
	declaration, _ := declarations[0].(map[string]any)
	parameters, _ := declaration["parameters"].(map[string]any)
	if _, ok := parameters["additionalProperties"]; ok {
		t.Error("Gemini no acepta additionalProperties")
	}
	properties, _ := parameters["properties"].(map[string]any)
	minutes, _ := properties["minutes"].(map[string]any)
	if _, ok := minutes["minimum"]; ok {
		t.Error("Gemini no acepta minimum")
	}
}

func TestGeminiCompleteTextoPlano(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, geminiTextBody)
	}))
	defer srv.Close()

	client := NewGemini(Config{BaseURL: srv.URL, APIKey: "k", Timeout: time.Second})
	resp, err := client.Complete(context.Background(), domain.ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "¿Hay paradas?"}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp.Message.Content != "No hay vehículos detenidos." {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if len(resp.Message.ToolCalls) != 0 {
		t.Errorf("no debe haber tool calls")
	}
}

func TestGeminiCompleteErrorHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"quota exceeded"}}`)
	}))
	defer srv.Close()

	client := NewGemini(Config{BaseURL: srv.URL, APIKey: "k", Timeout: time.Second})
	_, err := client.Complete(context.Background(), domain.ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "hola"}},
	})
	if !errors.Is(err, domain.ErrProvider) {
		t.Fatalf("error = %v, esperado ErrProvider", err)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("el error debe incluir el status 429: %v", err)
	}
}

func TestGeminiCompleteTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, geminiTextBody)
	}))
	defer srv.Close()

	client := NewGemini(Config{BaseURL: srv.URL, APIKey: "k", Timeout: 10 * time.Millisecond})
	_, err := client.Complete(context.Background(), domain.ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "hola"}},
	})
	if !errors.Is(err, domain.ErrTimeout) {
		t.Fatalf("error = %v, esperado ErrTimeout", err)
	}
}

func TestGeminiCompleteSinCandidatesEsErrorProveedor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"candidates":[]}`)
	}))
	defer srv.Close()

	client := NewGemini(Config{BaseURL: srv.URL, APIKey: "k", Timeout: time.Second})
	_, err := client.Complete(context.Background(), domain.ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "hola"}},
	})
	if !errors.Is(err, domain.ErrProvider) {
		t.Fatalf("error = %v, esperado ErrProvider", err)
	}
}

// ── Tests de la factory (provider.go) ─────────────────────────────────────

func TestNewProviderOpenAI(t *testing.T) {
	m, err := NewProvider(ProviderOpenAI, Config{BaseURL: "http://localhost", APIKey: "k", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewProvider openai: %v", err)
	}
	if _, ok := m.(*Client); !ok {
		t.Errorf("esperado *Client, got %T", m)
	}
}

func TestNewProviderVacioEsOpenAI(t *testing.T) {
	m, err := NewProvider("", Config{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewProvider vacío: %v", err)
	}
	if _, ok := m.(*Client); !ok {
		t.Errorf("esperado *Client para provider vacío, got %T", m)
	}
}

func TestNewProviderGemini(t *testing.T) {
	m, err := NewProvider(ProviderGemini, Config{APIKey: "k", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewProvider gemini: %v", err)
	}
	if _, ok := m.(*GeminiClient); !ok {
		t.Errorf("esperado *GeminiClient, got %T", m)
	}
}

func TestNewProviderDesconocidoEsError(t *testing.T) {
	_, err := NewProvider("anthropic", Config{})
	if err == nil {
		t.Fatal("se esperaba error para proveedor desconocido")
	}
	if !strings.Contains(err.Error(), "anthropic") {
		t.Errorf("el error debe mencionar el nombre del proveedor: %v", err)
	}
}
