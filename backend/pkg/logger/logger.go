// Package logger envuelve log/slog con configuración por entorno.
// Log estructurado: JSON en producción, texto legible en desarrollo.
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// New crea un logger según nivel y formato indicados.
// Niveles válidos: debug, info, warn, error.
// Formatos válidos: text, json.
func New(level, format string) (*slog.Logger, error) {
	return NewWithWriter(level, format, os.Stdout)
}

// NewWithWriter permite inyectar el writer (pruebas).
func NewWithWriter(level, format string, w io.Writer) (*slog.Logger, error) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "", "info":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("logger: nivel de log inválido %q", level)
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "", "json":
		handler = slog.NewJSONHandler(w, opts)
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default:
		return nil, fmt.Errorf("logger: formato de log inválido %q", format)
	}

	return slog.New(handler), nil
}
