// Package httperr estandariza el formato de error de la API (ADR-10).
// Formato: { "error": { "code": "CODE", "message": "..." } }
package httperr

import (
	"encoding/json"
	"net/http"
)

// Error es la representación del error de negocio para el cliente.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type envelope struct {
	Error Error `json:"error"`
}

// Write responde con el código HTTP indicado y el error estandarizado.
func Write(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Error: Error{Code: code, Message: message}})
}

// BadRequest 400: entrada malformada.
func BadRequest(w http.ResponseWriter, code, message string) {
	Write(w, http.StatusBadRequest, code, message)
}

// Unauthenticated 401: sin token o token inválido.
func Unauthenticated(w http.ResponseWriter, message string) {
	Write(w, http.StatusUnauthorized, "UNAUTHENTICATED", message)
}

// Forbidden 403: autenticado pero sin permiso sobre el recurso.
func Forbidden(w http.ResponseWriter, message string) {
	Write(w, http.StatusForbidden, "FORBIDDEN_SCOPE", message)
}

// NotFound 404: recurso inexistente.
func NotFound(w http.ResponseWriter, message string) {
	Write(w, http.StatusNotFound, "NOT_FOUND", message)
}

// Conflict 409: conflicto de estado o idempotencia.
func Conflict(w http.ResponseWriter, code, message string) {
	Write(w, http.StatusConflict, code, message)
}

// Validation 422: datos de entrada inválidos (reglas de negocio).
func Validation(w http.ResponseWriter, code, message string) {
	Write(w, http.StatusUnprocessableEntity, code, message)
}

// Internal 500: error inesperado del servidor.
func Internal(w http.ResponseWriter) {
	Write(w, http.StatusInternalServerError, "INTERNAL", "error interno")
}
