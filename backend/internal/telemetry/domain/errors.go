package domain

import "errors"

var (
	// ErrValidation: muestra o lote con datos inválidos (422).
	ErrValidation = errors.New("datos de telemetría inválidos")
	// ErrForbiddenScope: el principal no tiene alcance sobre el vehículo/canal (403).
	ErrForbiddenScope = errors.New("sin permiso sobre el recurso de telemetría")
	// ErrNotFound: no hay muestras para el vehículo en el rango pedido (404).
	ErrNotFound = errors.New("telemetría no encontrada")
	// ErrIdempotencyKey: el Bulk Sync no envió Idempotency-Key válida (422).
	ErrIdempotencyKey = errors.New("Idempotency-Key requerida para el bulk sync")
)
