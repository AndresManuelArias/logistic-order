// Package llm implementa el puerto domain.ChatModel sobre una API HTTP
// compatible con OpenAI (/chat/completions + function calling). No usa SDK:
// el mismo adaptador sirve para OpenAI, Groq, Ollama o vLLM.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"tiendas-track/backend/internal/aiagent/domain"
)

const (
	maxResponseBytes = 1 << 20 // 1 MiB
	maxErrorBytes    = 4 << 10 // 4 KiB
)

// Config configura el adaptador. BaseURL es la raíz de la API (sin
// /chat/completions); APIKey vacía no se envía (Ollama local).
type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// Client implementa domain.ChatModel.
type Client struct {
	endpoint string
	apiKey   string
	http     *http.Client
}

// New construye el adaptador. Normaliza la URL y aplica un timeout de red como
// respaldo del timeout de contexto del caso de uso.
func New(cfg Config) *Client {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if strings.HasSuffix(base, "/chat/completions") {
		base = strings.TrimSuffix(base, "/chat/completions")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &Client{
		endpoint: base + "/chat/completions",
		apiKey:   strings.TrimSpace(cfg.APIKey),
		http:     &http.Client{Timeout: timeout},
	}
}

// Complete envía la conversación y devuelve el mensaje del modelo.
func (c *Client) Complete(ctx context.Context, req domain.ChatRequest) (domain.ChatResponse, error) {
	body, err := json.Marshal(toRequest(req))
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("%w: serializando petición: %v", domain.ErrProvider, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("%w: construyendo petición: %v", domain.ErrProvider, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "tiendas-track-aiagent/1.0")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return domain.ChatResponse{}, domain.ErrTimeout
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return domain.ChatResponse{}, domain.ErrTimeout
		}
		return domain.ChatResponse{}, fmt.Errorf("%w: %v", domain.ErrProvider, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBytes))
		return domain.ChatResponse{}, fmt.Errorf("%w: status %d: %s",
			domain.ErrProvider, resp.StatusCode, strings.TrimSpace(string(detail)))
	}

	var parsed chatResponse
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes))
	if err := dec.Decode(&parsed); err != nil {
		return domain.ChatResponse{}, fmt.Errorf("%w: respuesta ilegible: %v", domain.ErrProvider, err)
	}
	if len(parsed.Choices) == 0 {
		return domain.ChatResponse{}, fmt.Errorf("%w: respuesta sin choices", domain.ErrProvider)
	}

	choice := parsed.Choices[0].Message
	out := domain.ChatResponse{
		Model: parsed.Model,
		Message: domain.Message{
			Role: domain.RoleAssistant,
		},
	}
	if choice.Content != nil {
		out.Message.Content = *choice.Content
	}
	for _, tc := range choice.ToolCalls {
		out.Message.ToolCalls = append(out.Message.ToolCalls, domain.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out, nil
}

// --- protocolo HTTP --------------------------------------------------------

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []toolSpec    `json:"tools,omitempty"`
	ToolChoice  string        `json:"tool_choice,omitempty"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    *string    `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolSpec struct {
	Type     string       `json:"type"`
	Function functionSpec `json:"function"`
}

type functionSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role      string  `json:"role"`
			Content   *string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

func toRequest(req domain.ChatRequest) chatRequest {
	out := chatRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Tools:       make([]toolSpec, 0, len(req.Tools)),
	}
	if len(req.Tools) > 0 {
		out.ToolChoice = "auto"
	}
	for _, m := range req.Messages {
		msg := chatMessage{
			Role:       m.Role,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		content := m.Content
		if content != "" || (m.Role != domain.RoleAssistant && len(m.ToolCalls) == 0) {
			msg.Content = &content
		}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, toolCall{
				ID:   tc.ID,
				Type: "function",
				Function: functionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		out.Messages = append(out.Messages, msg)
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, toolSpec{
			Type: "function",
			Function: functionSpec{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return out
}

// Compilación estática: el adaptador satisface el puerto.
var _ domain.ChatModel = (*Client)(nil)
