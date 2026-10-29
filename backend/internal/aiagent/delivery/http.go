// Package delivery expone POST /api/v1/ai/query (RF-10). Sin lógica de negocio:
// valida la forma, delega en el caso de uso y traduce errores a HTTP.
package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"tiendas-track/backend/internal/aiagent/domain"
	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/middleware"
	"tiendas-track/backend/internal/shared/resilience"
	"tiendas-track/backend/internal/tracing"
)

const maxBodyBytes = 8 << 10 // 8 KiB

// Agent es el puerto que consume el handler (facilita pruebas aisladas).
type Agent interface {
	Query(ctx context.Context, p authz.Principal, question string) (*domain.Answer, error)
}

// Handler atiende las consultas del asistente operativo.
type Handler struct {
	agent Agent
	log   *slog.Logger
}

// NewHandler crea el handler.
func NewHandler(agent Agent, log *slog.Logger) *Handler {
	return &Handler{agent: agent, log: log}
}

// RegisterRoutes monta el endpoint para los roles con datos operativos.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler,
	roles func(...string) func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/ai/query",
		auth(roles(authz.RoleProvider, authz.RoleStore, authz.RoleDriver)(http.HandlerFunc(h.Query))))
}

type queryRequest struct {
	Question string `json:"question"`
}

type toolDTO struct {
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	OK         bool            `json:"ok"`
	Error      string          `json:"error,omitempty"`
	DurationMS int64           `json:"duration_ms"`
}

type queryResponse struct {
	Answer  string    `json:"answer"`
	Tool    string    `json:"tool,omitempty"`
	Tools   []toolDTO `json:"tools,omitempty"`
	Model   string    `json:"model,omitempty"`
	AuditID string    `json:"audit_id,omitempty"`
}

// Query valida la pregunta y devuelve la respuesta del agente.
func (h *Handler) Query(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())

	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		httperr.Unauthenticated(w, "no autenticado")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req queryRequest
	if err := dec.Decode(&req); err != nil {
		httperr.Validation(w, "INVALID_BODY", "cuerpo JSON inválido o demasiado grande")
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httperr.Validation(w, "INVALID_BODY", "cuerpo con datos extra")
		return
	}

	question := strings.TrimSpace(req.Question)
	if question == "" {
		httperr.Validation(w, "INVALID_QUESTION", "la pregunta no puede estar vacía")
		return
	}
	if utf8.RuneCountInString(question) > domain.MaxQuestionLength {
		httperr.Validation(w, "INVALID_QUESTION", "la pregunta supera 500 caracteres")
		return
	}

	ans, err := h.agent.Query(r.Context(), p, question)
	if err != nil {
		h.mapError(w, traceID, err)
		return
	}

	resp := queryResponse{
		Answer:  ans.Answer,
		Tool:    ans.Tool,
		Model:   ans.Model,
		AuditID: ans.AuditID,
	}
	for _, inv := range ans.Tools {
		resp.Tools = append(resp.Tools, toolDTO{
			Name:       inv.Name,
			Arguments:  inv.Arguments,
			OK:         inv.OK,
			Error:      inv.Error,
			DurationMS: inv.DurationMS,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) mapError(w http.ResponseWriter, traceID string, err error) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		httperr.Validation(w, "INVALID_QUESTION", "la pregunta no es válida")
	case errors.Is(err, domain.ErrForbidden):
		httperr.Forbidden(w, "tu rol no puede consultar el asistente")
	case errors.Is(err, domain.ErrNotConfigured):
		httperr.ServiceUnavailable(w, "AI_NOT_CONFIGURED",
			"el asistente no está configurado en este entorno")
	case errors.Is(err, resilience.ErrCircuitOpen):
		w.Header().Set("Retry-After", "10")
		httperr.ServiceUnavailable(w, "CIRCUIT_OPEN",
			"el proveedor del asistente no está disponible; reintenta más tarde")
	case errors.Is(err, domain.ErrTimeout):
		httperr.Write(w, http.StatusGatewayTimeout, "AI_TIMEOUT",
			"el proveedor del asistente tardó demasiado")
	case errors.Is(err, domain.ErrProvider):
		httperr.Write(w, http.StatusBadGateway, "AI_PROVIDER_ERROR",
			"el proveedor del asistente devolvió un error")
	case errors.Is(err, domain.ErrNoAnswer):
		httperr.Write(w, http.StatusBadGateway, "AI_INVALID_RESPONSE",
			"el asistente no produjo una respuesta válida")
	default:
		h.log.Error("aiagent: error inesperado en la consulta",
			"trace_id", traceID, "error", err.Error())
		httperr.Internal(w)
	}
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
