// Package tracing maneja correlation IDs (UUID de traza) para rastrear requests.
// El correlation ID se propaga en el contexto y se incluye en todos los logs.
package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type ctxKey int

const correlationIDKey ctxKey = iota

// CorrelationIDFromContext extrae el correlation ID del contexto.
// Si no existe, devuelve una cadena vacía y false.
func CorrelationIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(correlationIDKey).(string)
	return id, ok
}

// WithCorrelationID inyecta un correlation ID en el contexto.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}

// GenerateCorrelationID crea un UUID aleatorio para correlación de traces.
func GenerateCorrelationID() string {
	b := make([]byte, 16)
	rand.Read(b) // Ignoramos el error: rng criptográfica es infalible en Go
	return hex.EncodeToString(b)
}

// TracingMiddleware es middleware HTTP que:
//   - Genera un correlation ID (UUID) para cada request
//   - Lo inyecta en el contexto
//   - Propaga el correlation ID a los logs del handler
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := GenerateCorrelationID()
		ctx := WithCorrelationID(r.Context(), correlationID)

		// Propagar correlation ID a headers para observabilidad
		w.Header().Set("X-Correlation-ID", correlationID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
