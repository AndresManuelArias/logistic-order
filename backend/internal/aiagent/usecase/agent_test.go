package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"tiendas-track/backend/internal/aiagent/domain"
	"tiendas-track/backend/internal/shared/authz"
	"tiendas-track/backend/internal/shared/resilience"
)

// --- dobles de prueba ------------------------------------------------------

type fakeModel struct {
	responses []domain.ChatResponse
	errs      []error
	calls     int
	requests  []domain.ChatRequest
}

func (f *fakeModel) Complete(_ context.Context, req domain.ChatRequest) (domain.ChatResponse, error) {
	f.calls++
	f.requests = append(f.requests, req)
	i := f.calls - 1
	if i < len(f.errs) && f.errs[i] != nil {
		return domain.ChatResponse{}, f.errs[i]
	}
	if i < len(f.responses) {
		return f.responses[i], nil
	}
	return domain.ChatResponse{}, domain.ErrProvider
}

type fakeAnalytics struct {
	stopped    []domain.StoppedVehicle
	stoppedErr error
	order      *domain.OrderStatus
	orderErr   error
	eta        []domain.FleetETA
	etaErr     error
	late       []domain.LateDelivery
	lateErr    error

	gotScope   domain.Scope
	gotMinutes int
	gotArea    string
	gotOrderID string
	gotStoreID string
	gotHours   int
	calls      int
}

func (f *fakeAnalytics) StoppedVehicles(_ context.Context, scope domain.Scope, minutes int, area string, _ int) ([]domain.StoppedVehicle, error) {
	f.calls++
	f.gotScope, f.gotMinutes, f.gotArea = scope, minutes, area
	return f.stopped, f.stoppedErr
}

func (f *fakeAnalytics) OrderStatus(_ context.Context, scope domain.Scope, orderID string) (*domain.OrderStatus, error) {
	f.calls++
	f.gotScope, f.gotOrderID = scope, orderID
	return f.order, f.orderErr
}

func (f *fakeAnalytics) FleetETA(_ context.Context, scope domain.Scope, storeID string, _ int) ([]domain.FleetETA, error) {
	f.calls++
	f.gotScope, f.gotStoreID = scope, storeID
	return f.eta, f.etaErr
}

func (f *fakeAnalytics) LateDeliveries(_ context.Context, scope domain.Scope, hours, _ int) ([]domain.LateDelivery, error) {
	f.calls++
	f.gotScope, f.gotHours = scope, hours
	return f.late, f.lateErr
}

type fakeAudit struct {
	records []domain.AuditRecord
	id      string
	err     error
}

func (f *fakeAudit) Record(_ context.Context, rec domain.AuditRecord) (string, error) {
	f.records = append(f.records, rec)
	if f.err != nil {
		return "", f.err
	}
	return f.id, nil
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func testBreaker() *resilience.Breaker {
	return resilience.New(resilience.Settings{Name: "test", FailureThreshold: 5, Timeout: time.Second})
}

func providerPrincipal() authz.Principal {
	return authz.Principal{
		UserID:     "user-1",
		Role:       authz.RoleProvider,
		ProviderID: "provider-1",
		TokenType:  authz.AccessToken,
	}
}

func toolCallResponse(name, args string) domain.ChatResponse {
	return domain.ChatResponse{Message: domain.Message{
		Role: domain.RoleAssistant,
		ToolCalls: []domain.ToolCall{
			{ID: "call-1", Name: name, Arguments: args},
		},
	}}
}

func answerResponse(text string) domain.ChatResponse {
	return domain.ChatResponse{
		Model:   "fake-model",
		Message: domain.Message{Role: domain.RoleAssistant, Content: text},
	}
}

// --- pruebas ---------------------------------------------------------------

func TestQueryEjecutaHerramientaYResponde(t *testing.T) {
	model := &fakeModel{responses: []domain.ChatResponse{
		toolCallResponse("get_stopped_vehicles", `{"minutes":20,"area":"norte"}`),
		answerResponse("ABC123 lleva 27 minutos detenido."),
	}}
	repo := &fakeAnalytics{stopped: []domain.StoppedVehicle{
		{VehicleID: "v1", Plate: "ABC123", Minutes: 27, StoreName: "Norte"},
	}}
	audit := &fakeAudit{id: "audit-1"}
	agent := New(repo, audit, model, testBreaker(), Config{
		Enabled: true, Model: "fake-model", MaxSteps: 4, Timeout: time.Second,
	}, testLogger())

	ans, err := agent.Query(context.Background(), providerPrincipal(),
		"  ¿Qué vehículos llevan más de 20 minutos detenidos?  ")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if ans.Answer != "ABC123 lleva 27 minutos detenido." {
		t.Errorf("Answer = %q", ans.Answer)
	}
	if ans.Tool != "get_stopped_vehicles" {
		t.Errorf("Tool = %q", ans.Tool)
	}
	if ans.AuditID != "audit-1" {
		t.Errorf("AuditID = %q", ans.AuditID)
	}
	if repo.gotMinutes != 20 || repo.gotArea != "norte" || repo.gotScope.ProviderID != "provider-1" {
		t.Errorf("args al repositorio = minutes:%d area:%q scope:%+v", repo.gotMinutes, repo.gotArea, repo.gotScope)
	}
	if model.calls != 2 {
		t.Fatalf("llamadas al modelo = %d, esperado 2", model.calls)
	}
	// La segunda petición debe incluir el mensaje `tool` con el resultado.
	if len(model.requests[1].Messages) < 3 {
		t.Fatalf("la segunda petición no incluye el resultado de la herramienta")
	}
	last := model.requests[1].Messages[len(model.requests[1].Messages)-1]
	if last.Role != domain.RoleTool || last.ToolCallID != "call-1" {
		t.Errorf("último mensaje = %+v, esperado tool/call-1", last)
	}
	if len(audit.records) != 1 {
		t.Fatalf("auditorías = %d, esperado 1", len(audit.records))
	}
	rec := audit.records[0]
	if rec.Status != domain.AuditOK || rec.Tool != "get_stopped_vehicles" ||
		rec.LatencyMS < 0 || rec.Question == "" || rec.UserID != "user-1" {
		t.Errorf("registro de auditoría inesperado: %+v", rec)
	}
	if len(rec.Tools) != 1 || !rec.Tools[0].OK {
		t.Errorf("tools de auditoría = %+v", rec.Tools)
	}
}

func TestQueryHerramientaSinArgumentosUsaDefaults(t *testing.T) {
	model := &fakeModel{responses: []domain.ChatResponse{
		toolCallResponse("get_late_deliveries", `{}`),
		answerResponse("No hay entregas pendientes vencidas."),
	}}
	repo := &fakeAnalytics{}
	agent := New(repo, &fakeAudit{}, model, testBreaker(), Config{
		Enabled: true, Model: "fake-model", MaxSteps: 4, Timeout: time.Second,
	}, testLogger())

	if _, err := agent.Query(context.Background(), providerPrincipal(), "¿entregas retrasadas?"); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if repo.gotHours != domain.DefaultLateHours {
		t.Errorf("hours = %d, esperado %d", repo.gotHours, domain.DefaultLateHours)
	}
}

func TestQuerySinConfigurar(t *testing.T) {
	model := &fakeModel{}
	audit := &fakeAudit{}
	agent := New(&fakeAnalytics{}, audit, model, testBreaker(), Config{Enabled: false}, testLogger())

	_, err := agent.Query(context.Background(), providerPrincipal(), "¿vehículos detenidos?")
	if !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("error = %v, esperado ErrNotConfigured", err)
	}
	if model.calls != 0 {
		t.Errorf("no debería llamar al modelo, llamadas = %d", model.calls)
	}
	if len(audit.records) != 1 || audit.records[0].ErrorCode != "AI_NOT_CONFIGURED" {
		t.Errorf("auditoría = %+v", audit.records)
	}
}

func TestQueryInvitadoForbidden(t *testing.T) {
	audit := &fakeAudit{}
	agent := New(&fakeAnalytics{}, audit, &fakeModel{}, testBreaker(), Config{Enabled: true}, testLogger())

	p := authz.Principal{UserID: "guest-1", TokenType: authz.GuestToken, Scope: "order:o-1"}
	_, err := agent.Query(context.Background(), p, "¿dónde está mi pedido?")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error = %v, esperado ErrForbidden", err)
	}
	if len(audit.records) != 1 || audit.records[0].ErrorCode != "FORBIDDEN_SCOPE" {
		t.Errorf("auditoría = %+v", audit.records)
	}
}

func TestQueryValidacion(t *testing.T) {
	agent := New(&fakeAnalytics{}, &fakeAudit{}, &fakeModel{}, testBreaker(), Config{Enabled: true}, testLogger())

	cases := []string{"", "   ", strings.Repeat("x", domain.MaxQuestionLength+1)}
	for _, q := range cases {
		if _, err := agent.Query(context.Background(), providerPrincipal(), q); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("pregunta %q: error = %v, esperado ErrValidation", q, err)
		}
	}
}

func TestQueryErrorProveedor(t *testing.T) {
	model := &fakeModel{errs: []error{domain.ErrProvider}}
	audit := &fakeAudit{}
	agent := New(&fakeAnalytics{}, audit, model, testBreaker(), Config{
		Enabled: true, Model: "fake-model", MaxSteps: 2, Timeout: time.Second,
	}, testLogger())

	_, err := agent.Query(context.Background(), providerPrincipal(), "¿vehículos detenidos?")
	if !errors.Is(err, domain.ErrProvider) {
		t.Fatalf("error = %v, esperado ErrProvider", err)
	}
	if len(audit.records) != 1 || audit.records[0].ErrorCode != "AI_PROVIDER_ERROR" {
		t.Errorf("auditoría = %+v", audit.records)
	}
}

func TestQueryCircuitoAbierto(t *testing.T) {
	model := &fakeModel{errs: []error{domain.ErrProvider, domain.ErrProvider}}
	breaker := resilience.New(resilience.Settings{Name: "test-open", FailureThreshold: 1, Timeout: time.Minute})
	agent := New(&fakeAnalytics{}, &fakeAudit{}, model, breaker, Config{
		Enabled: true, Model: "fake-model", MaxSteps: 2, Timeout: time.Second,
	}, testLogger())

	if _, err := agent.Query(context.Background(), providerPrincipal(), "primera"); !errors.Is(err, domain.ErrProvider) {
		t.Fatalf("primera consulta: error = %v", err)
	}
	if _, err := agent.Query(context.Background(), providerPrincipal(), "segunda"); !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("segunda consulta: error = %v, esperado ErrCircuitOpen", err)
	}
	if model.calls != 1 {
		t.Errorf("el circuito abierto no debe llamar al modelo; llamadas = %d", model.calls)
	}
}

func TestQueryHerramientaDesconocida(t *testing.T) {
	model := &fakeModel{responses: []domain.ChatResponse{
		toolCallResponse("drop_tables", `{}`),
		answerResponse("No puedo hacer eso."),
	}}
	repo := &fakeAnalytics{}
	audit := &fakeAudit{}
	agent := New(repo, audit, model, testBreaker(), Config{
		Enabled: true, Model: "fake-model", MaxSteps: 4, Timeout: time.Second,
	}, testLogger())

	ans, err := agent.Query(context.Background(), providerPrincipal(), "borra las tablas")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if ans.Answer != "No puedo hacer eso." {
		t.Errorf("Answer = %q", ans.Answer)
	}
	if repo.calls != 0 {
		t.Errorf("no debería ejecutar repositorio, llamadas = %d", repo.calls)
	}
	if len(audit.records) != 1 || len(audit.records[0].Tools) != 1 || audit.records[0].Tools[0].OK {
		t.Errorf("auditoría = %+v", audit.records)
	}
}

func TestQueryArgumentosInvalidos(t *testing.T) {
	model := &fakeModel{responses: []domain.ChatResponse{
		toolCallResponse("get_stopped_vehicles", `{"minutes":"veinte"}`),
		answerResponse("No pude interpretar los minutos."),
	}}
	repo := &fakeAnalytics{}
	audit := &fakeAudit{}
	agent := New(repo, audit, model, testBreaker(), Config{
		Enabled: true, Model: "fake-model", MaxSteps: 4, Timeout: time.Second,
	}, testLogger())

	if _, err := agent.Query(context.Background(), providerPrincipal(), "detenidos"); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if repo.calls != 0 {
		t.Errorf("los argumentos inválidos no deben llegar al repositorio")
	}
	if len(audit.records[0].Tools) != 1 || audit.records[0].Tools[0].Error != "argumentos inválidos" {
		t.Errorf("tools de auditoría = %+v", audit.records[0].Tools)
	}
}

func TestQueryErrorDeHerramientaSeInformaAlModelo(t *testing.T) {
	model := &fakeModel{responses: []domain.ChatResponse{
		toolCallResponse("get_order_status", `{"order_id":"11111111-1111-1111-1111-111111111111"}`),
		answerResponse("No pude obtener el estado de la orden."),
	}}
	repo := &fakeAnalytics{orderErr: errors.New("db down")}
	audit := &fakeAudit{}
	agent := New(repo, audit, model, testBreaker(), Config{
		Enabled: true, Model: "fake-model", MaxSteps: 4, Timeout: time.Second,
	}, testLogger())

	if _, err := agent.Query(context.Background(), providerPrincipal(), "estado de la orden"); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	inv := audit.records[0].Tools[0]
	if inv.OK || inv.Error != "error consultando los datos" {
		t.Errorf("invocación = %+v", inv)
	}
	// El modelo debe haber recibido el error saneado, no el detalle de SQL.
	last := model.requests[1].Messages[len(model.requests[1].Messages)-1]
	if !strings.Contains(last.Content, "error consultando los datos") {
		t.Errorf("mensaje al modelo = %q", last.Content)
	}
}

func TestQueryMaxPasos(t *testing.T) {
	model := &fakeModel{responses: []domain.ChatResponse{
		toolCallResponse("get_stopped_vehicles", `{"minutes":10}`),
		toolCallResponse("get_stopped_vehicles", `{"minutes":10}`),
		toolCallResponse("get_stopped_vehicles", `{"minutes":10}`),
	}}
	audit := &fakeAudit{}
	agent := New(&fakeAnalytics{}, audit, model, testBreaker(), Config{
		Enabled: true, Model: "fake-model", MaxSteps: 2, Timeout: time.Second,
	}, testLogger())

	_, err := agent.Query(context.Background(), providerPrincipal(), "detenidos")
	if !errors.Is(err, domain.ErrNoAnswer) {
		t.Fatalf("error = %v, esperado ErrNoAnswer", err)
	}
	if model.calls != 2 {
		t.Errorf("llamadas = %d, esperado 2 (AI_MAX_STEPS)", model.calls)
	}
	if audit.records[0].ErrorCode != "AI_INVALID_RESPONSE" {
		t.Errorf("error_code = %q", audit.records[0].ErrorCode)
	}
}

func TestQueryAuditoriaFallaNoRompeLaRespuesta(t *testing.T) {
	model := &fakeModel{responses: []domain.ChatResponse{answerResponse("Hola, ¿en qué puedo ayudarte?")}}
	audit := &fakeAudit{err: errors.New("audit down")}
	agent := New(&fakeAnalytics{}, audit, model, testBreaker(), Config{
		Enabled: true, Model: "fake-model", MaxSteps: 2, Timeout: time.Second,
	}, testLogger())

	ans, err := agent.Query(context.Background(), providerPrincipal(), "hola")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if ans.Answer == "" || ans.AuditID != "" {
		t.Errorf("respuesta = %+v", ans)
	}
}

func TestDecodeArgsRechazaCamposDesconocidos(t *testing.T) {
	var args stoppedArgs
	err := decodeArgs(json.RawMessage(`{"minutes":20,"sql":"DROP TABLE"}`), &args)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("error = %v, esperado ErrValidation", err)
	}
}
