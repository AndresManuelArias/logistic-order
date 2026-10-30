// Package llm expone la factory de proveedores LLM.
// Aplica el patrón Strategy: el código de negocio depende del puerto
// domain.ChatModel; la selección de la estrategia concreta ocurre aquí,
// en la capa de infraestructura, guiada por ProviderConfig.Name.
package llm

import (
	"fmt"
	"strings"

	"tiendas-track/backend/internal/aiagent/domain"
)

// ProviderName identifica la estrategia de LLM a usar.
type ProviderName string

const (
	// ProviderOpenAI usa la API compatible con OpenAI (OpenAI, Groq, Ollama…).
	ProviderOpenAI ProviderName = "openai"
	// ProviderGemini usa la API REST nativa de Google Gemini.
	ProviderGemini ProviderName = "gemini"
)

// ProviderConfig agrupa todos los parámetros de configuración del proveedor.
type ProviderConfig struct {
	// Name elige la estrategia: "openai" (por defecto) o "gemini".
	Name    ProviderName
	BaseURL string
	APIKey  string
	Timeout Config // reusa Config solo por el campo Timeout
}

// NewProvider es la factory que implementa el Strategy pattern:
// devuelve la estrategia (domain.ChatModel) adecuada al proveedor
// configurado, sin que ninguna capa superior conozca las implementaciones.
//
// Proveedores soportados:
//   - "openai"  → Client  (OpenAI / Groq / Ollama / vLLM)
//   - "gemini"  → GeminiClient (Google Gemini generateContent API)
func NewProvider(name ProviderName, cfg Config) (domain.ChatModel, error) {
	switch ProviderName(strings.ToLower(string(name))) {
	case ProviderOpenAI, "": // vacío → openai por compatibilidad hacia atrás
		return New(cfg), nil
	case ProviderGemini:
		return NewGemini(cfg), nil
	default:
		return nil, fmt.Errorf("llm: proveedor desconocido %q (valores válidos: openai, gemini)", name)
	}
}
