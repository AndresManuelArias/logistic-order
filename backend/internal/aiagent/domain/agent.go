// Package domain define el lenguaje del agente operativo de IA (RF-10).
// Es un paquete puro: no conoce HTTP, SQL ni proveedores de LLM.
package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Errores de negocio del módulo. Los adaptadores los envuelven para que el
// borde HTTP traduzca el fallo sin filtrar detalles del proveedor.
var (
	ErrNotConfigured = errors.New("agente IA no configurado")
	ErrProvider      = errors.New("proveedor LLM no disponible")
	ErrTimeout       = errors.New("tiempo de espera del proveedor agotado")
	ErrNoAnswer      = errors.New("el modelo no devolvió una respuesta final")
	ErrForbidden     = errors.New("consulta fuera del alcance del usuario")
	ErrValidation    = errors.New("consulta inválida")
	ErrNotFound      = errors.New("recurso no encontrado o fuera de alcance")
)

const (
	// MaxQuestionLength acota la entrada del usuario (coste y abuso).
	MaxQuestionLength = 500
	// MaxToolCallsPerStep evita que un modelo encadene herramientas sin control.
	MaxToolCallsPerStep = 4
	// DefaultStoppedMinutes es la ventana por defecto de detección de paradas.
	DefaultStoppedMinutes = 20
	// DefaultLateHours es la ventana por defecto de entregas pendientes vencidas.
	DefaultLateHours = 24
	// MaxToolRows acota las filas que se envían al modelo.
	MaxToolRows = 50
	// DefaultETASpeedKPH se usa cuando la última muestra no trae velocidad.
	DefaultETASpeedKPH = 30.0
	// AuditAnswerLimit evita guardar respuestas desproporcionadas.
	AuditAnswerLimit = 4000
)

// Roles de los mensajes enviados al modelo.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall es una herramienta elegida por el modelo.
type ToolCall struct {
	ID               string
	Name             string
	Arguments        string // JSON crudo; se valida antes de ejecutar
	ThoughtSignature string
}

// ToolInvocation es el resultado auditable de una llamada a herramienta.
type ToolInvocation struct {
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	OK         bool            `json:"ok"`
	Error      string          `json:"error,omitempty"`
	DurationMS int64           `json:"duration_ms"`
}

// Message es un mensaje del protocolo de chat (formato neutro).
type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
	Name       string
}

// ToolDefinition describe una herramienta para el proveedor (JSON Schema).
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ChatRequest es una petición de finalización de chat.
type ChatRequest struct {
	Model       string
	Messages    []Message
	Tools       []ToolDefinition
	Temperature float64
	MaxTokens   int
}

// ChatResponse es la respuesta del modelo.
type ChatResponse struct {
	Message Message
	Model   string
}

// ChatModel es el puerto hacia el proveedor de LLM.
type ChatModel interface {
	Complete(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// Answer es la respuesta final que consume el portal.
type Answer struct {
	Answer  string
	Tool    string
	Tools   []ToolInvocation
	Model   string
	AuditID string
}

// AuditRecord es una consulta registrada en app.ai_query_log.
type AuditRecord struct {
	UserID     string
	Question   string
	Tool       string
	Params     json.RawMessage
	Tools      []ToolInvocation
	Status     string
	ErrorCode  string
	Answer     string
	Model      string
	LatencyMS  int64
	OccurredAt time.Time
}

// Estados de auditoría.
const (
	AuditOK    = "OK"
	AuditError = "ERROR"
)
