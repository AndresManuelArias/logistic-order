// Package domain: modelo de autenticación. Sin dependencias de frameworks.
package domain

import (
	"time"

	"tiendas-track/backend/internal/shared/authz"
)

// User es un usuario del sistema con credenciales y rol (RF-01).
type User struct {
	ID           string
	Email        string
	PasswordHash string
	FullName     string
	Role         string // authz.RoleProvider | authz.RoleStore | authz.RoleDriver
	Status       string // ACTIVE | SUSPENDED
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AllowedRegistrationRoles son los roles que un usuario puede autoasignarse.
// PROVIDER se crea por seed/administración: nadie se autoasigna rol de proveedor.
var AllowedRegistrationRoles = []string{authz.RoleStore, authz.RoleDriver}

// CanRegisterRole valida el rol de registro.
func CanRegisterRole(role string) bool {
	for _, r := range AllowedRegistrationRoles {
		if r == role {
			return true
		}
	}
	return false
}
