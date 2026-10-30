// Adaptador Gemini para el puerto domain.ChatModel.
// Usa la API REST de Gemini (generateContent) con function calling nativo.
// No usa SDK: solo net/http + encoding/json para mantener las dependencias mínimas.
//
// Documentación del protocolo:
//   https://ai.google.dev/api/generate-content
//   https://ai.google.dev/api/caching#Tool
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
	geminiDefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	geminiDefaultModel   = "gemini-1.5-flash"
)

// GeminiClient implementa domain.ChatModel sobre la API de Gemini.
type GeminiClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewGemini construye el adaptador de Gemini.
func NewGemini(cfg Config) *GeminiClient {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = geminiDefaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &GeminiClient{
		baseURL: base,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		http:    &http.Client{Timeout: timeout},
	}
}

// Complete envía la conversación a Gemini y traduce la respuesta al formato
// neutro del dominio. Soporta function calling multi-step del mismo modo que
// el adaptador de OpenAI.
func (c *GeminiClient) Complete(ctx context.Context, req domain.ChatRequest) (domain.ChatResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = geminiDefaultModel
	}

	// Endpoint: POST .../models/{model}:generateContent?key={apiKey}
	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, model, c.apiKey)

	body, err := json.Marshal(toGeminiRequest(req))
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("%w: serializando petición Gemini: %v", domain.ErrProvider, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("%w: construyendo petición Gemini: %v", domain.ErrProvider, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "tiendas-track-aiagent/1.0")

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
		return domain.ChatResponse{}, fmt.Errorf("%w: Gemini status %d: %s",
			domain.ErrProvider, resp.StatusCode, strings.TrimSpace(string(detail)))
	}

	var parsed geminiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&parsed); err != nil {
		return domain.ChatResponse{}, fmt.Errorf("%w: respuesta Gemini ilegible: %v", domain.ErrProvider, err)
	}

	return fromGeminiResponse(parsed, model)
}

// ── Wire types de Gemini ────────────────────────────────────────────────────

// geminiRequest es el cuerpo de POST generateContent.
type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	Tools             []geminiTool    `json:"tools,omitempty"`
	GenerationConfig  geminiGenConfig `json:"generationConfig"`
}

type geminiContent struct {
	// Role: "user" | "model" (Gemini no usa "assistant" ni "tool")
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// geminiPart cubre los tres tipos que usamos: texto, functionCall y functionResponse.
// Los campos se omiten cuando están vacíos para que Gemini no rechace el request.
type geminiPart struct {
	Text             string                `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall   `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResp   `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type geminiFunctionResp struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiGenConfig struct {
	Temperature    float64 `json:"temperature"`
	MaxOutputTokens int    `json:"maxOutputTokens,omitempty"`
}

// geminiResponse es la respuesta de generateContent.
type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	ModelVersion string `json:"modelVersion"`
}

// ── Traducción dominio ↔ wire ───────────────────────────────────────────────

// toGeminiRequest convierte el ChatRequest neutro al wire format de Gemini.
// Gemini trata el system prompt de forma separada (systemInstruction) y usa
// "model" en lugar de "assistant" como role del asistente.
func toGeminiRequest(req domain.ChatRequest) geminiRequest {
	out := geminiRequest{
		GenerationConfig: geminiGenConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		},
	}

	// Extraer el system prompt si el primer mensaje es role=system.
	messages := req.Messages
	if len(messages) > 0 && messages[0].Role == domain.RoleSystem {
		out.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: messages[0].Content}},
		}
		messages = messages[1:]
	}

	// Convertir cada mensaje al formato de Gemini.
	for _, m := range messages {
		content := messageToGeminiContent(m)
		out.Contents = append(out.Contents, content)
	}

	// Convertir las definiciones de herramientas.
	if len(req.Tools) > 0 {
		decls := make([]geminiFunctionDecl, 0, len(req.Tools))
		for _, t := range req.Tools {
			decls = append(decls, geminiFunctionDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			})
		}
		out.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}

	return out
}

func messageToGeminiContent(m domain.Message) geminiContent {
	role := m.Role
	// Gemini usa "model" para el asistente; "user" para user, tool results y system.
	switch role {
	case domain.RoleAssistant:
		role = "model"
	case domain.RoleTool:
		// Los resultados de herramientas van en un content con role "user"
		// y una Part de tipo functionResponse.
		role = "user"
	default:
		role = "user"
	}

	content := geminiContent{Role: role}

	// Texto del mensaje.
	if m.Content != "" {
		content.Parts = append(content.Parts, geminiPart{Text: m.Content})
	}

	// ToolCalls del asistente → functionCall parts (en mensajes "model").
	for _, tc := range m.ToolCalls {
		args := json.RawMessage(tc.Arguments)
		if !json.Valid(args) || len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		content.Parts = append(content.Parts, geminiPart{
			FunctionCall: &geminiFunctionCall{
				Name: tc.Name,
				Args: args,
			},
		})
	}

	// Resultado de herramienta (role=tool) → functionResponse part.
	if m.Role == domain.RoleTool {
		// El contenido de la respuesta es texto; lo envolvemos en un JSON
		// { "content": "..." } para que sea un objeto válido.
		responseJSON, err := json.Marshal(map[string]string{"content": m.Content})
		if err != nil {
			responseJSON = json.RawMessage(`{"content":""}`)
		}
		content.Parts = []geminiPart{{
			FunctionResponse: &geminiFunctionResp{
				Name:     m.Name,
				Response: responseJSON,
			},
		}}
	}

	return content
}

// fromGeminiResponse convierte la respuesta de Gemini al ChatResponse neutro.
func fromGeminiResponse(parsed geminiResponse, model string) (domain.ChatResponse, error) {
	if len(parsed.Candidates) == 0 {
		return domain.ChatResponse{}, fmt.Errorf("%w: Gemini: respuesta sin candidates", domain.ErrProvider)
	}

	candidate := parsed.Candidates[0].Content
	out := domain.ChatResponse{
		Model: parsed.ModelVersion,
		Message: domain.Message{
			Role: domain.RoleAssistant,
		},
	}
	if out.Model == "" {
		out.Model = model
	}

	for _, part := range candidate.Parts {
		switch {
		case part.FunctionCall != nil:
			// Normalizar args: Gemini puede devolver un objeto vacío.
			args := string(part.FunctionCall.Args)
			if args == "" || args == "null" {
				args = "{}"
			}
			// Gemini no devuelve un ID por tool call; generamos uno determinista.
			id := "gemini-" + part.FunctionCall.Name
			out.Message.ToolCalls = append(out.Message.ToolCalls, domain.ToolCall{
				ID:        id,
				Name:      part.FunctionCall.Name,
				Arguments: args,
			})
		case part.Text != "":
			out.Message.Content += part.Text
		}
	}

	out.Message.Content = strings.TrimSpace(out.Message.Content)
	return out, nil
}

// Compilación estática: el adaptador satisface el puerto.
var _ domain.ChatModel = (*GeminiClient)(nil)
