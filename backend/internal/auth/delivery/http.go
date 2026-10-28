// Package delivery: handlers HTTP del módulo auth. Sin lógica de negocio:
// solo DTOs, validación de forma y mapeo de errores de dominio a HTTP.
package delivery

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"tiendas-track/backend/internal/auth/domain"
	"tiendas-track/backend/internal/auth/usecase"
	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/httperr"
	"tiendas-track/backend/internal/shared/middleware"
	"tiendas-track/backend/internal/tracing"
)

// Handler expone los endpoints de autenticación.
type Handler struct {
	uc  *usecase.AuthUseCase
	log *slog.Logger
}

// NewHandler crea el handler.
func NewHandler(uc *usecase.AuthUseCase, log *slog.Logger) *Handler {
	return &Handler{uc: uc, log: log}
}

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
	// Store es opcional: si viene, la tienda nace con su dueño en el mismo
	// registro; si no, se da de alta después con POST /api/v1/stores.
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
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("Register: JSON inválido", "trace_id", traceID, "error", err.Error())
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}

	h.log.Info("Register: procesando solicitud",
		"trace_id", traceID,
		"email", req.Email,
		"role", req.Role,
	)

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
		h.log.Error("Register: error en caso de uso",
			"trace_id", traceID,
			"error", err.Error(),
		)
		mapAuthError(w, err)
		return
	}

	h.log.Info("Register: usuario creado exitosamente",
		"trace_id", traceID,
		"user_id", u.ID,
		"role", u.Role,
	)

	writeJSON(w, http.StatusCreated, toUserResponse(u))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("Login: JSON inválido", "trace_id", traceID, "error", err.Error())
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}

	h.log.Info("Login: procesando solicitud",
		"trace_id", traceID,
		"email", req.Email,
	)

	pair, err := h.uc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		h.log.Error("Login: error en caso de uso",
			"trace_id", traceID,
			"error", err.Error(),
		)
		mapAuthError(w, err)
		return
	}

	h.log.Info("Login: login exitoso",
		"trace_id", traceID,
		"user_id", pair.User.ID,
		"role", pair.User.Role,
	)

	writeJSON(w, http.StatusOK, pair)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())

	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		h.log.Error("Refresh: JSON inválido o refresh_token vacío", "trace_id", traceID)
		httperr.BadRequest(w, "INVALID_JSON", "refresh_token requerido")
		return
	}

	h.log.Info("Refresh: procesando solicitud",
		"trace_id", traceID,
	)

	pair, err := h.uc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		h.log.Error("Refresh: error en caso de uso",
			"trace_id", traceID,
			"error", err.Error(),
		)
		mapAuthError(w, err)
		return
	}

	h.log.Info("Refresh: refresh exitoso",
		"trace_id", traceID,
	)

	writeJSON(w, http.StatusOK, pair)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())

	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		h.log.Error("Logout: JSON inválido o refresh_token vacío", "trace_id", traceID)
		httperr.BadRequest(w, "INVALID_JSON", "refresh_token requerido")
		return
	}

	h.log.Info("Logout: procesando solicitud",
		"trace_id", traceID,
	)

	if err := h.uc.Logout(r.Context(), req.RefreshToken); err != nil {
		h.log.Error("Logout: error en caso de uso",
			"trace_id", traceID,
			"error", err.Error(),
		)
		mapAuthError(w, err)
		return
	}

	h.log.Info("Logout: logout exitoso",
		"trace_id", traceID,
	)

	w.WriteHeader(http.StatusNoContent)
}

type guestRequest struct {
	OrderID string `json:"order_id"`
}

func (h *Handler) GuestToken(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		h.log.Error("GuestToken: no autenticado", "trace_id", traceID)
		httperr.Unauthenticated(w, "no autenticado")
		return
	}

	var req guestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("GuestToken: JSON inválido", "trace_id", traceID, "error", err.Error())
		httperr.BadRequest(w, "INVALID_JSON", "cuerpo JSON inválido")
		return
	}

	h.log.Info("GuestToken: procesando solicitud",
		"trace_id", traceID,
		"order_id", req.OrderID,
		"caller_role", p.Role,
	)

	out, err := h.uc.GuestToken(r.Context(), p, strings.TrimSpace(req.OrderID))
	if err != nil {
		h.log.Error("GuestToken: error en caso de uso",
			"trace_id", traceID,
			"error", err.Error(),
		)
		mapAuthError(w, err)
		return
	}

	h.log.Info("GuestToken: token de invitado emitido exitosamente",
		"trace_id", traceID,
		"order_id", out.OrderID,
	)

	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	traceID, _ := tracing.CorrelationIDFromContext(r.Context())
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok || p.IsGuest() {
		h.log.Error("Me: no autenticado", "trace_id", traceID)
		httperr.Unauthenticated(w, "no autenticado")
		return
	}

	h.log.Info("Me: obteniendo información de usuario",
		"trace_id", traceID,
		"user_id", p.UserID,
	)

	u, err := h.uc.Me(r.Context(), p.UserID)
	if err != nil {
		h.log.Error("Me: error en caso de uso",
			"trace_id", traceID,
			"error", err.Error(),
		)
		mapAuthError(w, err)
		return
	}

	h.log.Info("Me: información de usuario obtenida exitosamente",
		"trace_id", traceID,
		"user_id", u.ID,
	)

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
