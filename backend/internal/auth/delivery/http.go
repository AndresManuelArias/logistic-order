// Package delivery: handlers HTTP del módulo auth. Sin lógica de negocio:
// solo DTOs, validación de forma y mapeo de errores de dominio a HTTP.
package delivery

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"tiendas-track/backend/internal/auth/domain"
	"tiendas-track/backend/internal/auth/usecase"
	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/middleware"
)

// Handler expone los endpoints de autenticación.
type Handler struct {
	uc *usecase.AuthUseCase
}

// NewHandler crea el handler.
func NewHandler(uc *usecase.AuthUseCase) *Handler { return &Handler{uc: uc} }

// RegisterRoutes monta /api/v1/auth/*.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.HandleFunc("POST /api/v1/auth/register", h.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	mux.HandleFunc("POST /api/v1/auth/refresh", h.Refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", h.Logout)
	mux.Handle("POST /api/v1/auth/guest-token", auth(http.HandlerFunc(h.GuestToken)))
	mux.Handle("GET /api/v1/auth/me", auth(http.HandlerFunc(h.Me)))
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
	// Store es obligatorio para role=STORE: la tienda nace con su dueño.
	Store *storePayload `json:"store"`
}

// storePayload es la georreferenciación de la tienda (ADR-09: PostGIS).
type storePayload struct {
	Name     string    `json:"name"`
	Address  string    `json:"address"`
	Location *pointDTO `json:"location"`
}

// pointDTO es una coordenada WGS84 en el contrato HTTP.
type pointDTO struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}
	in := usecase.RegisterInput{
		Email: req.Email, Password: req.Password, FullName: req.FullName,
		Role: strings.ToUpper(req.Role),
	}
	if req.Store != nil {
		in.Store = &domain.StoreProfile{Name: req.Store.Name, Address: req.Store.Address}
		if req.Store.Location != nil {
			in.Store.Lat, in.Store.Lon = req.Store.Location.Lat, req.Store.Location.Lon
		}
	}
	u, err := h.uc.Register(r.Context(), in)
	if err != nil {
		mapAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUserResponse(u))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}
	pair, err := h.uc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		mapAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		httperr.BadRequest(w, "INVALID_JSON", "refresh_token requerido")
		return
	}
	pair, err := h.uc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		mapAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		httperr.BadRequest(w, "INVALID_JSON", "refresh_token requerido")
		return
	}
	if err := h.uc.Logout(r.Context(), req.RefreshToken); err != nil {
		mapAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type guestRequest struct {
	OrderID string `json:"order_id"`
}

func (h *Handler) GuestToken(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		httperr.Unauthenticated(w, "no autenticado")
		return
	}
	var req guestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}
	out, err := h.uc.GuestToken(r.Context(), p, strings.TrimSpace(req.OrderID))
	if err != nil {
		mapAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok || p.IsGuest() {
		httperr.Unauthenticated(w, "no autenticado")
		return
	}
	u, err := h.uc.Me(r.Context(), p.UserID)
	if err != nil {
		mapAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(u))
}

// userResponse define el contrato público del usuario (nunca el hash).
type userResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
}

func toUserResponse(u *domain.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, FullName: u.FullName, Role: u.Role}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// mapAuthError traduce errores de dominio a respuestas HTTP acordes al ADR-10.
func mapAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		httperr.Write(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "email o contraseña incorrectos")
	case errors.Is(err, domain.ErrInvalidRefreshToken):
		httperr.Write(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "refresh token inválido o expirado")
	case errors.Is(err, domain.ErrEmailAlreadyRegistered):
		httperr.Conflict(w, "EMAIL_ALREADY_REGISTERED", "el email ya está registrado")
	case errors.Is(err, domain.ErrUserSuspended):
		httperr.Write(w, http.StatusForbidden, "USER_SUSPENDED", "usuario suspendido")
	case errors.Is(err, domain.ErrOrderAccess):
		httperr.Forbidden(w, "sin acceso a la orden solicitada")
	case errors.Is(err, domain.ErrUserNotFound):
		httperr.NotFound(w, "usuario no encontrado")
	case errors.Is(err, domain.ErrInvalidEmail):
		httperr.Validation(w, "INVALID_EMAIL", "email inválido")
	case errors.Is(err, domain.ErrWeakPassword):
		httperr.Validation(w, "WEAK_PASSWORD", "la contraseña debe tener al menos 8 caracteres")
	case errors.Is(err, domain.ErrInvalidRole):
		httperr.Validation(w, "INVALID_ROLE", "rol no permitido para auto-registro")
	case errors.Is(err, domain.ErrValidation):
		httperr.Validation(w, "VALIDATION_ERROR", err.Error())
	default:
		httperr.Internal(w)
	}
}

// Compile-time: authz se usa para roles en Register.
var _ = authz.RoleStore
