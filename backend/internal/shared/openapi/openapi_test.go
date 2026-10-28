package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// El YAML embebido debe ser una especificación OpenAPI 3.1 válida en estructura.
func TestSpecYAMLValida(t *testing.T) {
	if len(SpecYAML()) == 0 {
		t.Fatal("SpecYAML embebido vacío")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(SpecYAML(), &doc); err != nil {
		t.Fatalf("YAML inválido: %v", err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Errorf("openapi = %v, esperado 3.1.0 (ADR-10)", doc["openapi"])
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatal("la especificación no declara paths")
	}
}

// Todos los endpoints F2 documentados deben existir en el spec (ADR-10:
// el contrato es la fuente de verdad; sin doc no hay endpoint).
func TestSpecCubreEndpointsF2(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(SpecYAML(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	required := []struct {
		path   string
		method string
	}{
		{"/healthz", "get"},
		{"/readyz", "get"},
		{"/api/v1/auth/register", "post"},
		{"/api/v1/auth/login", "post"},
		{"/api/v1/auth/refresh", "post"},
		{"/api/v1/auth/logout", "post"},
		{"/api/v1/auth/me", "get"},
		{"/api/v1/auth/guest-token", "post"},
		{"/api/v1/orders", "post"},
		{"/api/v1/orders", "get"},
		{"/api/v1/orders/{id}", "get"},
		{"/api/v1/orders/{id}/status", "patch"},
		{"/api/v1/orders/{id}/assignment", "post"},
	}
	for _, r := range required {
		ops, ok := doc.Paths[r.path]
		if !ok {
			t.Errorf("falta documentar %s", r.path)
			continue
		}
		if _, ok := ops[r.method]; !ok {
			t.Errorf("falta documentar %s %s", strings.ToUpper(r.method), r.path)
		}
	}
}

// La conversión YAML→JSON debe producir un documento JSON parseable equivalente.
func TestSpecJSONEquivalente(t *testing.T) {
	body, err := SpecJSON()
	if err != nil {
		t.Fatalf("SpecJSON: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("JSON inválido: %v", err)
	}
	if parsed["openapi"] != "3.1.0" {
		t.Errorf("openapi en JSON = %v", parsed["openapi"])
	}
}

// Los tres endpoints de documentación responden 200 con content-type correcto.
func TestEndpointsDocumentacion(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)

	cases := []struct {
		url      string
		wantType string
	}{
		{"/api/v1/openapi.yaml", "application/yaml"},
		{"/api/v1/openapi.json", "application/json"},
		{"/swagger/index.html", "text/html"},
		{"/swagger/openapi.json", "application/json"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, esperado 200", tc.url, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.wantType) {
			t.Errorf("%s: Content-Type = %q, esperado %q", tc.url, ct, tc.wantType)
		}
	}

	// /swagger redirige a la UI.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger", nil))
	if rec.Code != http.StatusFound {
		t.Errorf("/swagger: status = %d, esperado 302", rec.Code)
	}
}
