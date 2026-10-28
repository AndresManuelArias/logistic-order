# Tiendas-Track — comandos de desarrollo local (F1).
COMPOSE ?= docker compose
BACKEND_DIR := backend

.PHONY: dev up db-up run-local down restart logs ps health seed test build vet fmt tidy psql e2e-f2 e2e-f3 test-web lint-web test-mobile lint-mobile gen-mobile-api clean help

help: ## Lista de comandos
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

dev: ## Levanta todo el stack (build + detached)
	$(COMPOSE) up --build -d

up: ## Levanta sin reconstruir
	$(COMPOSE) up -d

db-up: ## Inicializa solo la base de datos
	$(COMPOSE) up db -d

run-local: db-up ## Inicia el backend localmente contra PostgreSQL local
	cd $(BACKEND_DIR) && set -a && . ../.env && set +a && DATABASE_URL="postgres://$${POSTGRES_USER:-tiendas}:$${POSTGRES_PASSWORD:-tiendas_dev}@localhost:$${DB_PORT:-5432}/$${POSTGRES_DB:-tiendas}?sslmode=disable" go run ./cmd/server

down: ## Detiene y elimina contenedores
	$(COMPOSE) down

restart: ## Reinicia el backend
	$(COMPOSE) restart backend

logs: ## Logs del backend en vivo
	$(COMPOSE) logs -f backend

ps: ## Estado de los servicios
	$(COMPOSE) ps

health: ## Verifica /healthz y /readyz
	@echo "healthz:" && curl -fsS http://localhost:8080/healthz && echo \
	&& echo "readyz:" && curl -fsS http://localhost:8080/readyz && echo

seed: ## Carga datos demo (idempotente; credenciales en README)
	$(COMPOSE) --profile seed run --rm seed

swagger: ## Abre la documentación OpenAPI (imprime la URL)
	@echo "http://localhost:8080/swagger/index.html" \
	&& curl -fsS -o /dev/null -w "openapi.json: HTTP %{http_code}\n" \
	   http://localhost:8080/api/v1/openapi.json

test: ## Pruebas unitarias del backend
	cd $(BACKEND_DIR) && GOTOOLCHAIN=local go test ./...

build: ## Compila el backend
	cd $(BACKEND_DIR) && GOTOOLCHAIN=local go build ./...

vet: ## Análisis estático go vet
	cd $(BACKEND_DIR) && GOTOOLCHAIN=local go vet ./...

fmt: ## Formatea el código Go
	cd $(BACKEND_DIR) && GOTOOLCHAIN=local gofmt -w .

tidy: ## Ordena dependencias
	cd $(BACKEND_DIR) && GOTOOLCHAIN=local go mod tidy

psql: ## Shell psql dentro del contenedor db
	$(COMPOSE) exec db psql -U $${POSTGRES_USER:-tiendas} -d $${POSTGRES_DB:-tiendas}

e2e-f2: ## E2E F2 contra el stack local (72 casos)
	bash scripts/e2e_f2.sh

e2e-f3: ## E2E F3: telemetría, RBAC y WebSockets (60 casos)
	bash scripts/e2e_f3.sh

test-web: ## Pruebas del portal y librerías compartidas (F4)
	cd web && npm test

lint-web: ## Typecheck + lint + build del portal (F4)
	cd web && npm run typecheck && npm run lint && npm run build

test-mobile: ## Pruebas de la app móvil offline-first (F4)
	cd mobile && npm test

lint-mobile: ## Typecheck de la app móvil (F4)
	cd mobile && npm run typecheck

gen-mobile-api: ## Regenera los tipos OpenAPI de la app móvil (ADR-10)
	cd mobile && npm run gen:api

clean: ## Detiene el stack y borra volúmenes (¡borra datos locales!)
	$(COMPOSE) down -v
