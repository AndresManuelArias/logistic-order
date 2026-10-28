package domain

import "errors"

var (
	// ErrNotFound: orden inexistente.
	ErrNotFound = errors.New("orden no encontrada")
	// ErrInvalidTransition: transición de estado ilegal (RF-05).
	ErrInvalidTransition = errors.New("transición de estado no permitida")
	// ErrValidation: datos de entrada inválidos.
	ErrValidation = errors.New("datos de la orden inválidos")
	// ErrForbiddenScope: el rol no tiene acceso a la orden (RF-02).
	ErrForbiddenScope = errors.New("sin acceso a la orden")
	// ErrExternalRefConflict: external_ref duplicado (idempotencia).
	ErrExternalRefConflict = errors.New("external_ref ya registrado")
)
