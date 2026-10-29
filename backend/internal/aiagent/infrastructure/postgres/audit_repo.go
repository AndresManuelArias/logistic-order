package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"tiendas-track/backend/internal/aiagent/domain"
)

// AuditRepository persiste las consultas del agente en app.ai_query_log.
type AuditRepository struct {
	pool *pgxpool.Pool
}

// NewAuditRepository crea el repositorio de auditoría.
func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{pool: pool}
}

const insertAuditSQL = `
INSERT INTO app.ai_query_log
    (user_id, question, tool, params, latency_ms, status, error_code, answer, model, tools)
VALUES ($1::uuid, $2, $3, $4::jsonb, $5, $6, $7, $8, $9, $10::jsonb)
RETURNING id::text`

// Record inserta la consulta y devuelve su id. La pregunta y la respuesta se
// guardan tal cual (truncadas por el caso de uso); nunca se guardan secretos.
func (r *AuditRepository) Record(ctx context.Context, rec domain.AuditRecord) (string, error) {
	params := rec.Params
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	tools := rec.Tools
	if tools == nil {
		tools = []domain.ToolInvocation{}
	}
	toolsJSON, err := json.Marshal(tools)
	if err != nil {
		return "", fmt.Errorf("aiagent.audit.tools: %w", err)
	}

	var id string
	err = r.pool.QueryRow(ctx, insertAuditSQL,
		rec.UserID, rec.Question, rec.Tool, string(params), rec.LatencyMS,
		rec.Status, rec.ErrorCode, rec.Answer, rec.Model, string(toolsJSON),
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("aiagent.audit.insert: %w", err)
	}
	return id, nil
}

// Compilación estática: el adaptador satisface el puerto.
var _ domain.AuditRepository = (*AuditRepository)(nil)
