package domain

import "errors"

var (
	// ErrAlreadyExists: el usuario ya tiene su tienda (una por dueño).
	ErrAlreadyExists = errors.New("el usuario ya tiene una tienda registrada")
	// ErrNotFound: el usuario no tiene tienda (alta pendiente).
	ErrNotFound = errors.New("tienda no encontrada")
	// ErrForbiddenScope: rol sin permiso para operar tiendas.
	ErrForbiddenScope = errors.New("sin permiso para operar tiendas")
	// ErrValidation: datos de la tienda inválidos (nombre/coordenadas).
	ErrValidation = errors.New("datos de la tienda inválidos")
)
