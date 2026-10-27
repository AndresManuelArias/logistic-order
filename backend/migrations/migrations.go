// Package migrations embebe las migraciones SQL en el binario.
// Los archivos se aplican en orden alfabético; cada uno se ejecuta una
// sola vez y queda registrado en telemetry.schema_migrations.
package migrations

import "embed"

// FS contiene las migraciones SQL embebidas.
//
//go:embed *.sql
var FS embed.FS
