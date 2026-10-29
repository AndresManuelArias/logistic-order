// Package config carga la configuración del monolito desde variables de entorno.
// Regla: nada de valores por defecto distintos entre máquinas; los secretos
// nunca se definen aquí, llegan por entorno (ver .env.example en la raíz).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config es la configuración operativa del servidor.
type Config struct {
	AppName          string
	Env              string // development | test | production
	HTTPAddr         string // dirección de escucha HTTP, ej. ":8080"
	DatabaseURL      string // URL de PostgreSQL/TimescaleDB (secreto por entorno)
	DBMaxConns       int32
	DBConnectTimeout time.Duration
	MigrateOnStartup bool
	LogLevel         string
	LogFormat        string
	ShutdownTimeout  time.Duration

	// F2: autenticación JWT (RF-01, ADR-04). El secreto llega solo por entorno.
	JWTSecret     string
	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration
	GuestTokenTTL time.Duration
	BcryptCost    int

	// F3: tiempo real y resiliencia (RF-04, RNF-04, RNF-05).
	WSAllowedOrigins   []string
	WSGrantTTL         time.Duration
	CBFailureThreshold uint32
	CBTimeout          time.Duration

	// F5: agente operativo de IA (RF-10, RNF-05). La API key es un secreto:
	// si falta, el agente queda deshabilitado y responde 503 controlado.
	AIEnabled  bool
	AIBaseURL  string
	AIAPIKey   string
	AIModel    string
	AITimeout  time.Duration
	AIMaxSteps int

	// F6: rate limiting (§9.4). Límites por minuto y por clave (IP o usuario).
	RateLimitEnabled         bool
	RateLimitLoginPerMin     int
	RateLimitTelemetryPerMin int
	RateLimitAIPerMin        int
	RateLimitTrustProxy      bool
}

// Load lee la configuración del entorno. Falla si falta DATABASE_URL,
// que es obligatoria en todos los entornos.
func Load() (*Config, error) {
	cfg := &Config{
		AppName:          getEnv("APP_NAME", "tiendas-track-backend"),
		Env:              getEnv("APP_ENV", "development"),
		HTTPAddr:         getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		DBConnectTimeout: getDuration("DB_CONNECT_TIMEOUT", 30*time.Second),
		MigrateOnStartup: getBool("MIGRATE_ON_STARTUP", true),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		LogFormat:        getEnv("LOG_FORMAT", "text"),
		ShutdownTimeout:  getDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	maxConns, err := getInt32("DB_MAX_CONNS", 10)
	if err != nil {
		return nil, err
	}
	cfg.DBMaxConns = maxConns

	cost, err := getInt32("BCRYPT_COST", 10)
	if err != nil {
		return nil, err
	}
	cfg.BcryptCost = int(cost)

	// Autenticación (F2). SECRETOS: sin valor por defecto en código.
	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	cfg.JWTAccessTTL = getDuration("JWT_ACCESS_TTL", 15*time.Minute)
	cfg.JWTRefreshTTL = getDuration("JWT_REFRESH_TTL", 720*time.Hour) // 30 días
	cfg.GuestTokenTTL = getDuration("GUEST_TOKEN_TTL", 48*time.Hour)

	// Tiempo real y resiliencia (F3).
	cfg.WSAllowedOrigins = splitList(getEnv("WS_ALLOWED_ORIGINS",
		"http://localhost:3000,http://localhost:8080"))
	cfg.WSGrantTTL = getDuration("WS_GRANT_TTL", 60*time.Second)
	cfg.CBTimeout = getDuration("CB_TIMEOUT", 10*time.Second)

	threshold, err := getInt32("CB_FAILURE_THRESHOLD", 5)
	if err != nil {
		return nil, err
	}
	cfg.CBFailureThreshold = uint32(threshold)

	// Agente IA (F5). Sin AI_API_KEY el agente se deshabilita; no es un error
	// de arranque para que el resto del sistema siga operativo.
	cfg.AIEnabled = getBool("AI_ENABLED", true)
	cfg.AIBaseURL = getEnv("AI_BASE_URL", "https://api.openai.com/v1")
	cfg.AIAPIKey = os.Getenv("AI_API_KEY")
	cfg.AIModel = getEnv("AI_MODEL", "gpt-4o-mini")
	cfg.AITimeout = getDuration("AI_TIMEOUT", 20*time.Second)

	steps, err := getInt32("AI_MAX_STEPS", 4)
	if err != nil {
		return nil, err
	}
	if steps < 1 || steps > 8 {
		return nil, fmt.Errorf("config: AI_MAX_STEPS debe estar entre 1 y 8")
	}
	cfg.AIMaxSteps = int(steps)

	// Rate limiting (F6). Valores conservadores para desarrollo; ajustables
	// por entorno. Un valor <= 0 desactiva el limitador de esa superficie.
	cfg.RateLimitEnabled = getBool("RATE_LIMIT_ENABLED", true)
	cfg.RateLimitTrustProxy = getBool("RATE_LIMIT_TRUST_PROXY", false)
	if cfg.RateLimitLoginPerMin, err = getInt("RATE_LIMIT_LOGIN_PER_MIN", 10); err != nil {
		return nil, err
	}
	if cfg.RateLimitTelemetryPerMin, err = getInt("RATE_LIMIT_TELEMETRY_PER_MIN", 120); err != nil {
		return nil, err
	}
	if cfg.RateLimitAIPerMin, err = getInt("RATE_LIMIT_AI_PER_MIN", 10); err != nil {
		return nil, err
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL es obligatoria")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("config: JWT_SECRET es obligatoria y debe tener al menos 32 caracteres")
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// splitList convierte "a, b , c" en []string sin entradas vacías.
func splitList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func getInt32(key string, def int32) (int32, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("config: %s inválida (%q)", key, v)
	}
	return int32(n), nil
}

func getInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s inválida (%q)", key, v)
	}
	if n < 0 {
		return 0, fmt.Errorf("config: %s no puede ser negativa (%d)", key, n)
	}
	return n, nil
}
