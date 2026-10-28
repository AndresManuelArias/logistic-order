// Package api embebe la especificación OpenAPI canónica del proyecto (ADR-10).
// El archivo openapi.yaml es la fuente de verdad de los contratos HTTP y se
// compila dentro del binario para servirlo siempre que el servidor arranque.
package api

import _ "embed"

// SpecYAML es la especificación OpenAPI 3.1 original.
//
//go:embed openapi.yaml
var SpecYAML []byte
