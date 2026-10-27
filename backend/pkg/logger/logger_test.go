package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewNivelInvalido(t *testing.T) {
	if _, err := New("nivel-raro", "json"); err == nil {
		t.Fatal("se esperaba error con nivel inválido")
	}
}

func TestNewFormatoInvalido(t *testing.T) {
	if _, err := New("info", "xml"); err == nil {
		t.Fatal("se esperaba error con formato inválido")
	}
}

func TestNewTextEscribeMensaje(t *testing.T) {
	var buf bytes.Buffer
	log, err := NewWithWriter("info", "text", &buf)
	if err != nil {
		t.Fatalf("NewWithWriter() error = %v", err)
	}
	log.Info("hola")
	if !strings.Contains(buf.String(), "hola") {
		t.Errorf("salida = %q, debería contener 'hola'", buf.String())
	}
}

// Los logs de debug NO aparecen con nivel info (filtro correcto).
func TestNewFiltraDebug(t *testing.T) {
	var buf bytes.Buffer
	log, _ := NewWithWriter("info", "text", &buf)
	log.Debug("secreto")
	if strings.Contains(buf.String(), "secreto") {
		t.Error("mensaje debug filtrado no debería aparecer")
	}
}
