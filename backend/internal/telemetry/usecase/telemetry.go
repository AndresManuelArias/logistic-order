// Package usecase: ingesta y consulta de telemetría GPS con RBAC (RF-03, RF-04).
// La autorización se resuelve SIEMPRE server-side contra el scope del token.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/resilience"
	"tiendas-track/backend/internal/telemetry/domain"
	"tiendas-track/backend/internal/tracing"
)

const (
	// MaxBatchPoints acota un lote de Bulk Sync (evita cuerpos enormes).
	MaxBatchPoints = 1000
	// DefaultTrackLimit y MaxTrackLimit acotan la traza histórica.
	DefaultTrackLimit = 1000
	MaxTrackLimit     = 5000
	// MaxTrackRange acota el rango consultable de una traza.
	MaxTrackRange = 7 * 24 * time.Hour
	// Límites del Idempotency-Key del bulk sync.
	MinKeyLen = 8
	MaxKeyLen = 128
)

// Publisher publica hechos de dominio en el bus interno (eventbus.Bus).
type Publisher interface {
	Publish(topic string, payload []byte)
}

// TelemetryUseCase orquesta ingesta, consulta y autorización de canales.
type TelemetryUseCase struct {
	points domain.PointRepository
	scopes domain.ScopeRepository
	bus    Publisher
	reads  *resilience.Breaker
	log    *slog.Logger
	now    func() time.Time
}

// New crea el caso de uso. reads protege los caminos de lectura de BD (RNF-05).
func New(points domain.PointRepository, scopes domain.ScopeRepository, bus Publisher, reads *resilience.Breaker, log *slog.Logger) *TelemetryUseCase {
	return &TelemetryUseCase{
		points: points,
		scopes: scopes,
		bus:    bus,
		reads:  reads,
		log:    log,
		now:    time.Now,
	}
}

// PointInput es una muestra tal como llega del cliente.
type PointInput struct {
	VehicleID string
	OrderID   string
	Time      time.Time
	Lat       float64
	Lon       float64
	SpeedKPH  *float64
	AccuracyM *float64
	Heading   *float64
	DeviceSeq int64
}

// BulkInput es un lote de Bulk Sync (RF-09: idempotente por clave).
type BulkInput struct {
	IdempotencyKey string
	DeviceID       string
	Points         []PointInput
}

// Ingest persiste una muestra individual. Devuelve duplicate=true si la PK
// (time, vehicle, seq) ya existía (reintento del dispositivo).
func (uc *TelemetryUseCase) Ingest(ctx context.Context, p authz.Principal, in PointInput) (*domain.Point, bool, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)

	if !canIngest(p) {
		uc.log.Warn("Ingest: rol sin permiso de ingesta",
			"trace_id", traceID, "role", p.Role)
		return nil, false, domain.ErrForbiddenScope
	}

	pt, err := uc.toPoint(ctx, p, in, domain.SourceDevice)
	if err != nil {
		uc.log.Warn("Ingest: muestra inválida", "trace_id", traceID, "error", err.Error())
		return nil, false, err
	}
	if err := pt.Validate(uc.now()); err != nil {
		uc.log.Warn("Ingest: muestra inválida", "trace_id", traceID, "error", err.Error())
		return nil, false, err
	}

	vs, err := uc.assertVehicleAccess(ctx, p, pt.VehicleID)
	if err != nil {
		uc.log.Warn("Ingest: vehículo fuera de scope",
			"trace_id", traceID, "vehicle_id", pt.VehicleID, "role", p.Role)
		return nil, false, err
	}

	var order *domain.OrderScope
	if pt.OrderID != "" {
		if order, err = uc.assertOrderVehicle(ctx, pt.OrderID, pt.VehicleID); err != nil {
			uc.log.Warn("Ingest: orden no asignada al vehículo",
				"trace_id", traceID, "order_id", pt.OrderID, "vehicle_id", pt.VehicleID)
			return nil, false, err
		}
	}

	inserted, err := uc.points.Insert(ctx, []domain.Point{*pt})
	if err != nil {
		uc.log.Error("Ingest: error persistiendo muestra",
			"trace_id", traceID, "error", err.Error())
		return nil, false, err
	}

	duplicate := len(inserted) == 0
	if !duplicate {
		uc.publish([]domain.Point{*pt}, map[string]*domain.VehicleScope{pt.VehicleID: vs},
			orderMap(order))
	}

	uc.log.Info("Ingest: muestra procesada",
		"trace_id", traceID, "vehicle_id", pt.VehicleID,
		"order_id", pt.OrderID, "duplicate", duplicate)
	return pt, duplicate, nil
}

// IngestBulk valida el lote, lo persiste con recibo idempotente y publica solo
// las muestras realmente insertadas (un reintento no reemite eventos).
func (uc *TelemetryUseCase) IngestBulk(ctx context.Context, p authz.Principal, in BulkInput) (domain.BulkResult, error) {
	traceID, _ := tracing.CorrelationIDFromContext(ctx)

	if !canIngest(p) {
		uc.log.Warn("IngestBulk: rol sin permiso de ingesta",
			"trace_id", traceID, "role", p.Role)
		return domain.BulkResult{}, domain.ErrForbiddenScope
	}
	if err := validateKey(in.IdempotencyKey); err != nil {
		return domain.BulkResult{}, err
	}
	if len(in.Points) == 0 || len(in.Points) > MaxBatchPoints {
		return domain.BulkResult{}, fmt.Errorf("%w: el lote debe tener entre 1 y %d puntos",
			domain.ErrValidation, MaxBatchPoints)
	}

	now := uc.now()
	points := make([]domain.Point, 0, len(in.Points))
	vsByVehicle := make(map[string]*domain.VehicleScope)
	orderScopes := make(map[string]*domain.OrderScope)
	checkedOrderVehicle := make(map[string]bool)

	for i, pin := range in.Points {
		pt, err := uc.toPoint(ctx, p, pin, domain.SourceBulkSync)
		if err != nil {
			return domain.BulkResult{}, fmt.Errorf("punto %d: %w", i, err)
		}
		if err := pt.Validate(now); err != nil {
			return domain.BulkResult{}, fmt.Errorf("punto %d: %w", i, err)
		}

		if _, ok := vsByVehicle[pt.VehicleID]; !ok {
			vs, err := uc.assertVehicleAccess(ctx, p, pt.VehicleID)
			if err != nil {
				return domain.BulkResult{}, fmt.Errorf("punto %d: %w", i, err)
			}
			vsByVehicle[pt.VehicleID] = vs
		}

		if pt.OrderID != "" {
			pair := pt.OrderID + "|" + pt.VehicleID
			if !checkedOrderVehicle[pair] {
				os, err := uc.assertOrderVehicle(ctx, pt.OrderID, pt.VehicleID)
				if err != nil {
					return domain.BulkResult{}, fmt.Errorf("punto %d: %w", i, err)
				}
				orderScopes[pt.OrderID] = os
				checkedOrderVehicle[pair] = true
			}
		}
		points = append(points, *pt)
	}

	receipt := domain.Receipt{
		Key:         in.IdempotencyKey,
		DeviceID:    in.DeviceID,
		PointsCount: len(points),
	}
	if p.HasRole(authz.RoleDriver) {
		receipt.DriverUserID = p.UserID
	}

	res, err := uc.points.ProcessBulk(ctx, receipt, points)
	if err != nil {
		uc.log.Error("IngestBulk: error procesando lote",
			"trace_id", traceID, "error", err.Error())
		return domain.BulkResult{}, err
	}

	if !res.AlreadyProcessed {
		uc.publishInserted(points, res.Inserted, vsByVehicle, orderScopes)
	}

	uc.log.Info("IngestBulk: lote procesado",
		"trace_id", traceID,
		"points", len(points),
		"inserted", len(res.Inserted),
		"already_processed", res.AlreadyProcessed)
	return res, nil
}

// Latest devuelve la última posición del vehículo con RBAC y circuit breaker.
func (uc *TelemetryUseCase) Latest(ctx context.Context, p authz.Principal, vehicleID string) (*domain.Point, error) {
	if p.IsGuest() {
		return nil, domain.ErrForbiddenScope
	}
	if _, err := uc.assertVehicleAccess(ctx, p, vehicleID); err != nil {
		return nil, err
	}

	pt, err := resilience.DoCtx(ctx, uc.reads, func(ctx context.Context) (*domain.Point, error) {
		return uc.points.Latest(ctx, vehicleID)
	})
	if err != nil {
		uc.log.Error("Latest: error leyendo última posición",
			"vehicle_id", vehicleID, "error", err.Error())
		return nil, err
	}
	return pt, nil
}

// Track devuelve la traza histórica del vehículo con RBAC y circuit breaker.
func (uc *TelemetryUseCase) Track(ctx context.Context, p authz.Principal, vehicleID string, from, to time.Time, limit int) ([]domain.Point, error) {
	if p.IsGuest() {
		return nil, domain.ErrForbiddenScope
	}
	if _, err := uc.assertVehicleAccess(ctx, p, vehicleID); err != nil {
		return nil, err
	}
	if err := validateRange(from, to, limit); err != nil {
		return nil, err
	}

	pts, err := resilience.DoCtx(ctx, uc.reads, func(ctx context.Context) ([]domain.Point, error) {
		return uc.points.Track(ctx, vehicleID, from, to, limit)
	})
	if err != nil {
		uc.log.Error("Track: error leyendo traza",
			"vehicle_id", vehicleID, "error", err.Error())
		return nil, err
	}
	return pts, nil
}

// CanSubscribe autoriza un canal de tiempo real contra el principal (ADR-03).
// Se evalúa al suscribirse y se revalida mientras la sesión vive.
func (uc *TelemetryUseCase) CanSubscribe(ctx context.Context, p authz.Principal, ch authz.Channel) (bool, error) {
	switch ch.Kind {
	case authz.ChannelFleetProvider:
		return !p.IsGuest() && p.HasRole(authz.RoleProvider) && p.ProviderID != "" &&
			ch.ID == p.ProviderID, nil

	case authz.ChannelFleetStore:
		return !p.IsGuest() && p.HasRole(authz.RoleStore) && p.StoreID != "" &&
			ch.ID == p.StoreID, nil

	case authz.ChannelOrder:
		if p.IsGuest() {
			id, ok := p.GuestOrderID()
			return ok && id == ch.ID, nil
		}
		os, err := uc.scopes.OrderScope(ctx, ch.ID)
		if errors.Is(err, domain.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		switch {
		case p.HasRole(authz.RoleStore):
			return p.StoreID != "" && os.StoreID == p.StoreID, nil
		case p.HasRole(authz.RoleDriver):
			return p.DriverID != "" && os.AssignedDriverID == p.DriverID, nil
		case p.HasRole(authz.RoleProvider):
			if os.AssignedVehicleID == "" {
				return false, nil
			}
			vs, err := uc.scopes.VehicleScope(ctx, os.AssignedVehicleID)
			if errors.Is(err, domain.ErrNotFound) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return vs.ProviderID == providerID(p), nil
		}
		return false, nil

	case authz.ChannelVehicle:
		if p.IsGuest() { // el invitado solo sigue su orden, no vehículos sueltos
			return false, nil
		}
		_, err := uc.assertVehicleAccess(ctx, p, ch.ID)
		if errors.Is(err, domain.ErrForbiddenScope) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// --- helpers ---------------------------------------------------------------

func canIngest(p authz.Principal) bool {
	return !p.IsGuest() && (p.HasRole(authz.RoleDriver) || p.HasRole(authz.RoleProvider))
}

func providerID(p authz.Principal) string {
	if p.ProviderID != "" {
		return p.ProviderID
	}
	return p.UserID
}

// toPoint normaliza la entrada; si falta vehicle_id en un DRIVER se deriva de
// su vehículo asignado (la app no necesita conocerlo).
func (uc *TelemetryUseCase) toPoint(ctx context.Context, p authz.Principal, in PointInput, src domain.Source) (*domain.Point, error) {
	pt := &domain.Point{
		Time:       in.Time.UTC(),
		VehicleID:  in.VehicleID,
		DeviceSeq:  in.DeviceSeq,
		OrderID:    in.OrderID,
		Lat:        in.Lat,
		Lon:        in.Lon,
		SpeedKPH:   in.SpeedKPH,
		AccuracyM:  in.AccuracyM,
		Heading:    in.Heading,
		Source:     src,
		ReceivedAt: uc.now().UTC(),
	}

	if pt.VehicleID == "" {
		if !p.HasRole(authz.RoleDriver) || p.DriverID == "" {
			return nil, fmt.Errorf("%w: falta vehicle_id", domain.ErrValidation)
		}
		vs, err := uc.scopes.VehicleByDriver(ctx, p.DriverID)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("%w: el conductor no tiene vehículo asignado", domain.ErrForbiddenScope)
		}
		if err != nil {
			return nil, err
		}
		pt.VehicleID = vs.VehicleID
	}
	return pt, nil
}

// assertVehicleAccess comprueba que el principal puede operar sobre el vehículo.
func (uc *TelemetryUseCase) assertVehicleAccess(ctx context.Context, p authz.Principal, vehicleID string) (*domain.VehicleScope, error) {
	vs, err := uc.scopes.VehicleScope(ctx, vehicleID)
	if errors.Is(err, domain.ErrNotFound) {
		// No se distingue "no existe" de "ajeno" (anti-enumeración).
		return nil, domain.ErrForbiddenScope
	}
	if err != nil {
		return nil, err
	}

	switch {
	case p.IsGuest():
		return nil, domain.ErrForbiddenScope
	case p.HasRole(authz.RoleDriver):
		if p.DriverID == "" || vs.DriverID != p.DriverID {
			return nil, domain.ErrForbiddenScope
		}
	case p.HasRole(authz.RoleProvider):
		if vs.ProviderID != providerID(p) {
			return nil, domain.ErrForbiddenScope
		}
	case p.HasRole(authz.RoleStore):
		if p.StoreID == "" {
			return nil, domain.ErrForbiddenScope
		}
		ok, err := uc.scopes.VehicleInStoreScope(ctx, vehicleID, p.StoreID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, domain.ErrForbiddenScope
		}
	default:
		return nil, domain.ErrForbiddenScope
	}
	return vs, nil
}

// assertOrderVehicle verifica que la orden exista y esté asignada al vehículo.
func (uc *TelemetryUseCase) assertOrderVehicle(ctx context.Context, orderID, vehicleID string) (*domain.OrderScope, error) {
	os, err := uc.scopes.OrderScope(ctx, orderID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("%w: la orden %s no existe", domain.ErrValidation, orderID)
	}
	if err != nil {
		return nil, err
	}
	if os.AssignedVehicleID == "" || os.AssignedVehicleID != vehicleID {
		return nil, fmt.Errorf("%w: la orden %s no está asignada al vehículo %s",
			domain.ErrValidation, orderID, vehicleID)
	}
	return os, nil
}

func orderMap(os *domain.OrderScope) map[string]*domain.OrderScope {
	if os == nil {
		return nil
	}
	return map[string]*domain.OrderScope{os.OrderID: os}
}

// publish emite los eventos de posición a los canales de tiempo real.
func (uc *TelemetryUseCase) publish(points []domain.Point, vsByVehicle map[string]*domain.VehicleScope, orders map[string]*domain.OrderScope) {
	for _, pt := range points {
		ev := domain.PointEvent{
			VehicleID:  pt.VehicleID,
			OrderID:    pt.OrderID,
			Lat:        pt.Lat,
			Lon:        pt.Lon,
			SpeedKPH:   pt.SpeedKPH,
			Heading:    pt.Heading,
			Time:       pt.Time.UTC().Format(time.RFC3339Nano),
			ReceivedAt: pt.ReceivedAt.UTC().Format(time.RFC3339Nano),
			Source:     pt.Source,
		}
		payload, err := json.Marshal(ev)
		if err != nil {
			continue
		}

		uc.bus.Publish(channel(authz.ChannelVehicle, pt.VehicleID), payload)

		if vs := vsByVehicle[pt.VehicleID]; vs != nil && vs.ProviderID != "" {
			uc.bus.Publish(channel(authz.ChannelFleetProvider, vs.ProviderID), payload)
		}

		if pt.OrderID == "" {
			continue
		}
		uc.bus.Publish(channel(authz.ChannelOrder, pt.OrderID), payload)
		if os := orders[pt.OrderID]; os != nil && os.StoreID != "" {
			uc.bus.Publish(channel(authz.ChannelFleetStore, os.StoreID), payload)
		}
	}
}

// publishInserted emite solo las muestras que la BD confirmó como nuevas.
func (uc *TelemetryUseCase) publishInserted(points []domain.Point, inserted []domain.InsertedKey, vsByVehicle map[string]*domain.VehicleScope, orders map[string]*domain.OrderScope) {
	seen := make(map[domain.InsertedKey]struct{}, len(inserted))
	for _, k := range inserted {
		seen[k] = struct{}{}
	}
	confirmed := make([]domain.Point, 0, len(inserted))
	for _, pt := range points {
		k := domain.InsertedKey{Time: pt.Time, VehicleID: pt.VehicleID, DeviceSeq: pt.DeviceSeq}
		if _, ok := seen[k]; ok {
			confirmed = append(confirmed, pt)
		}
	}
	uc.publish(confirmed, vsByVehicle, orders)
}

func channel(kind, id string) string {
	return authz.Channel{Kind: kind, ID: id}.String()
}

func validateKey(key string) error {
	if len(key) < MinKeyLen || len(key) > MaxKeyLen {
		return fmt.Errorf("%w: longitud entre %d y %d caracteres",
			domain.ErrIdempotencyKey, MinKeyLen, MaxKeyLen)
	}
	return nil
}

func validateRange(from, to time.Time, limit int) error {
	if from.IsZero() || to.IsZero() {
		return fmt.Errorf("%w: from y to son obligatorios", domain.ErrValidation)
	}
	if !to.After(from) {
		return fmt.Errorf("%w: to debe ser posterior a from", domain.ErrValidation)
	}
	if to.Sub(from) > MaxTrackRange {
		return fmt.Errorf("%w: el rango máximo es %s", domain.ErrValidation, MaxTrackRange)
	}
	if limit < 0 || limit > MaxTrackLimit {
		return fmt.Errorf("%w: limit debe estar entre 0 y %d", domain.ErrValidation, MaxTrackLimit)
	}
	return nil
}
