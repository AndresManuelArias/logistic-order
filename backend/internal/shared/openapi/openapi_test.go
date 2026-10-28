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

// Todos los endpoints F2/F3 documentados deben existir en el spec (ADR-10:
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
		{"/api/v1/stores", "post"},
		{"/api/v1/stores/me", "get"},
		{"/api/v1/telemetry/points", "post"},
		{"/api/v1/telemetry/bulk", "post"},
		{"/api/v1/vehicles/{id}/latest", "get"},
		{"/api/v1/vehicles/{id}/track", "get"},
		{"/ws", "get"},
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

// Todos los $ref internos resuelven: evita contratos roto por renombrados.
func TestSpecRefsResueltos(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal(SpecYAML(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var refs []string
	var walk func(v any)
	walk = func(v any) {
		switch node := v.(type) {
		case map[string]any:
			for k, child := range node {
				if k == "$ref" {
					if s, ok := child.(string); ok {
						refs = append(refs, s)
					}
					continue
				}
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(doc)

	if len(refs) == 0 {
		t.Fatal("el spec no declara $ref")
	}
	for _, ref := range refs {
		if !strings.HasPrefix(ref, "#/") {
			t.Errorf("$ref externo no permitido: %s", ref)
			continue
		}
		var cur any = doc
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			m, ok := cur.(map[string]any)
			if !ok {
				t.Errorf("%s: no resuelve", ref)
				cur = nil
				break
			}
			cur = m[part]
		}
		if cur == nil {
			t.Errorf("%s: no resuelve", ref)
		}
	}
}

// El contrato de tiempo real (ADR-03/ADR-10) se documenta en /ws y el bulk
// declara el header Idempotency-Key obligatorio.
func TestSpecContratoTiempoReal(t *testing.T) {
	var doc struct {
		Paths map[string]struct {
			Get  map[string]any `yaml:"get"`
			Post map[string]any `yaml:"post"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(SpecYAML(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	ws, ok := doc.Paths["/ws"]
	if !ok || ws.Get == nil {
		t.Fatal("falta GET /ws")
	}
	if _, ok := ws.Get["x-websocket"]; !ok {
		t.Error("/ws debe declarar x-websocket (protocolo, canales y límites)")
	}

	bulk := doc.Paths["/api/v1/telemetry/bulk"].Post
	params, _ := bulk["parameters"].([]any)
	found := false
	for _, p := range params {
		if m, ok := p.(map[string]any); ok && m["name"] == "Idempotency-Key" && m["in"] == "header" {
			found = true
		}
	}
	if !found {
		t.Error("el bulk debe documentar el header Idempotency-Key")
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
