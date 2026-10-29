package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"tiendas-track/backend/internal/aiagent/domain"
)

// uuidRe valida identificadores antes de enviarlos a PostgreSQL. Evita que un
// argumento del modelo provoque un error de cast en la base de datos.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// tool es una herramienta del catálogo con su definición pública y su ejecutor.
type tool struct {
	definition domain.ToolDefinition
	run        func(ctx context.Context, scope domain.Scope, raw json.RawMessage) (any, error)
}

// registry es el catálogo cerrado de herramientas (ADR-07). El modelo solo
// puede elegir nombres y argumentos de esta lista.
type registry struct {
	tools map[string]tool
	repo  domain.AnalyticsRepository
}

func newRegistry(repo domain.AnalyticsRepository) *registry {
	r := &registry{tools: map[string]tool{}, repo: repo}

	r.register(tool{
		definition: domain.ToolDefinition{
			Name:        "get_stopped_vehicles",
			Description: "Vehículos que llevan detenidos al menos `minutes` minutos, opcionalmente filtrados por tienda o zona (`area`).",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"minutes": {"type": "integer", "minimum": 1, "maximum": 1440, "description": "Minutos mínimos detenido (por defecto 20)."},
					"area": {"type": "string", "maxLength": 80, "description": "Nombre o dirección de la tienda/zona a filtrar (opcional)."}
				},
				"additionalProperties": false
			}`),
		},
		run: r.stoppedVehicles,
	})

	r.register(tool{
		definition: domain.ToolDefinition{
			Name:        "get_order_status",
			Description: "Estado actual e hitos de una orden por su identificador. Devuelve el mismo error si no existe o está fuera del alcance del usuario.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"order_id": {"type": "string", "format": "uuid", "description": "Identificador de la orden."}
				},
				"required": ["order_id"],
				"additionalProperties": false
			}`),
		},
		run: r.orderStatus,
	})

	r.register(tool{
		definition: domain.ToolDefinition{
			Name:        "get_fleet_eta",
			Description: "Estimación de llegada de las órdenes en tránsito, calculada con la última posición GPS y una velocidad de crucero.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"store_id": {"type": "string", "format": "uuid", "description": "Tienda destino a filtrar (opcional)."}
				},
				"additionalProperties": false
			}`),
		},
		run: r.fleetETA,
	})

	r.register(tool{
		definition: domain.ToolDefinition{
			Name:        "get_late_deliveries",
			Description: "Órdenes pendientes (CREATED o IN_TRANSIT) que llevan más de `hours` horas sin entregarse.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"hours": {"type": "integer", "minimum": 1, "maximum": 720, "description": "Horas mínimas pendiente (por defecto 24)."}
				},
				"additionalProperties": false
			}`),
		},
		run: r.lateDeliveries,
	})

	return r
}

func (r *registry) register(t tool) { r.tools[t.definition.Name] = t }

// definitions devuelve el catálogo en el orden estable de registro.
func (r *registry) definitions() []domain.ToolDefinition {
	order := []string{"get_stopped_vehicles", "get_order_status", "get_fleet_eta", "get_late_deliveries"}
	out := make([]domain.ToolDefinition, 0, len(order))
	for _, name := range order {
		if t, ok := r.tools[name]; ok {
			out = append(out, t.definition)
		}
	}
	return out
}

// invoke valida y ejecuta una llamada. Un nombre desconocido o argumentos
// inválidos son errores de validación que se devuelven al modelo, no al usuario.
func (r *registry) invoke(ctx context.Context, scope domain.Scope, call domain.ToolCall) (json.RawMessage, error) {
	t, ok := r.tools[call.Name]
	if !ok {
		return nil, fmt.Errorf("%w: herramienta %q no disponible", domain.ErrValidation, call.Name)
	}
	raw := json.RawMessage(strings.TrimSpace(call.Arguments))
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage(`{}`)
	}
	result, err := t.run(ctx, scope, raw)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("aiagent: serializando resultado: %w", err)
	}
	return body, nil
}

// --- herramientas ----------------------------------------------------------

type stoppedArgs struct {
	Minutes *int   `json:"minutes"`
	Area    string `json:"area"`
}

func (r *registry) stoppedVehicles(ctx context.Context, scope domain.Scope, raw json.RawMessage) (any, error) {
	var args stoppedArgs
	if err := decodeArgs(raw, &args); err != nil {
		return nil, err
	}
	minutes := domain.DefaultStoppedMinutes
	if args.Minutes != nil {
		minutes = *args.Minutes
	}
	if minutes < 1 || minutes > 1440 {
		return nil, fmt.Errorf("%w: minutes debe estar entre 1 y 1440", domain.ErrValidation)
	}
	area := strings.TrimSpace(args.Area)
	if len([]rune(area)) > 80 {
		return nil, fmt.Errorf("%w: area supera 80 caracteres", domain.ErrValidation)
	}
	rows, err := r.repo.StoppedVehicles(ctx, scope, minutes, area, domain.MaxToolRows)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []domain.StoppedVehicle{}
	}
	return rows, nil
}

type orderArgs struct {
	OrderID string `json:"order_id"`
}

func (r *registry) orderStatus(ctx context.Context, scope domain.Scope, raw json.RawMessage) (any, error) {
	var args orderArgs
	if err := decodeArgs(raw, &args); err != nil {
		return nil, err
	}
	if !uuidRe.MatchString(strings.TrimSpace(args.OrderID)) {
		return nil, fmt.Errorf("%w: order_id debe ser un UUID", domain.ErrValidation)
	}
	row, err := r.repo.OrderStatus(ctx, scope, strings.TrimSpace(args.OrderID))
	if err != nil {
		return nil, err
	}
	return row, nil
}

type etaArgs struct {
	StoreID string `json:"store_id"`
}

func (r *registry) fleetETA(ctx context.Context, scope domain.Scope, raw json.RawMessage) (any, error) {
	var args etaArgs
	if err := decodeArgs(raw, &args); err != nil {
		return nil, err
	}
	storeID := strings.TrimSpace(args.StoreID)
	if storeID != "" && !uuidRe.MatchString(storeID) {
		return nil, fmt.Errorf("%w: store_id debe ser un UUID", domain.ErrValidation)
	}
	rows, err := r.repo.FleetETA(ctx, scope, storeID, domain.MaxToolRows)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []domain.FleetETA{}
	}
	return rows, nil
}

type lateArgs struct {
	Hours *int `json:"hours"`
}

func (r *registry) lateDeliveries(ctx context.Context, scope domain.Scope, raw json.RawMessage) (any, error) {
	var args lateArgs
	if err := decodeArgs(raw, &args); err != nil {
		return nil, err
	}
	hours := domain.DefaultLateHours
	if args.Hours != nil {
		hours = *args.Hours
	}
	if hours < 1 || hours > 720 {
		return nil, fmt.Errorf("%w: hours debe estar entre 1 y 720", domain.ErrValidation)
	}
	rows, err := r.repo.LateDeliveries(ctx, scope, hours, domain.MaxToolRows)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []domain.LateDelivery{}
	}
	return rows, nil
}

// decodeArgs exige JSON estricto: rechaza campos desconocidos y datos extra.
func decodeArgs(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: argumentos inválidos: %v", domain.ErrValidation, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: argumentos con datos extra", domain.ErrValidation)
	}
	return nil
}
