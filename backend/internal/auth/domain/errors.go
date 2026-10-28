package domain

import "errors"

var (
	// ErrInvalidCredentials: email o password incorrectos (RF-01).
	ErrInvalidCredentials = errors.New("credenciales inválidas")
	// ErrEmailAlreadyRegistered: email duplicado.
	ErrEmailAlreadyRegistered = errors.New("el email ya está registrado")
	// ErrUserNotFound: usuario inexistente.
	ErrUserNotFound = errors.New("usuario no encontrado")
	// ErrInvalidRole: rol no permitido para auto-registro.
	ErrInvalidRole = errors.New("rol no permitido para auto-registro")
	// ErrWeakPassword: política mínima de contraseña.
	ErrWeakPassword = errors.New("la contraseña debe tener al menos 8 caracteres")
	// ErrInvalidEmail: formato de email inválido.
	ErrInvalidEmail = errors.New("email inválido")
	// ErrInvalidRefreshToken: refresh token inválido, expirado o ya rotado.
	ErrInvalidRefreshToken = errors.New("refresh token inválido o expirado")
	// ErrUserSuspended: usuario bloqueado.
	ErrUserSuspended = errors.New("usuario suspendido")
	// ErrOrderAccess: invitado sin acceso a la orden solicitada.
	ErrOrderAccess = errors.New("sin acceso a la orden")
)
