# Tiendas-Track — comandos de desarrollo local (F1).
COMPOSE ?= docker compose
BACKEND_DIR := backend

.PHONY: dev up db-up run-local down restart logs ps health seed test build vet fmt tidy psql e2e-f2 e2e-f3 test-web lint-web install-mobile test-mobile lint-mobile gen-mobile-api seed-load load-test contract iac-validate iac-plan ci-local clean help

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
	cd $(BACKEND_DIR) && GOTOOLCHAIN=auto go test ./...

build: ## Compila el backend
	cd $(BACKEND_DIR) && GOTOOLCHAIN=auto go build ./...

vet: ## Análisis estático go vet
	cd $(BACKEND_DIR) && GOTOOLCHAIN=auto go vet ./...

fmt: ## Formatea el código Go
	cd $(BACKEND_DIR) && GOTOOLCHAIN=auto gofmt -w .

tidy: ## Ordena dependencias
	cd $(BACKEND_DIR) && GOTOOLCHAIN=auto go mod tidy

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

install-mobile: ## Instala dependencias de la app móvil (Node 20.19+ / 22 LTS)
	cd mobile && npm ci

test-mobile: ## Pruebas de la app móvil offline-first (F4)
	cd mobile && npm test

lint-mobile: ## Typecheck de la app móvil (F4)
	cd mobile && npm run typecheck

gen-mobile-api: ## Regenera los tipos OpenAPI de la app móvil (ADR-10)
	cd mobile && npm run gen:api

seed-load: ## Siembra 100 vehículos LOAD-* para k6 (F6)
	bash scripts/load/seed_load_vehicles.sh 100

load-test: ## Prueba de carga k6 vía Docker (F6; backend en :8082)
	docker run --rm --network host -i \
	  -e BASE_URL=$${BASE_URL:-http://localhost:8082} \
	  -e VUS=$${VUS:-100} -e DURATION=$${DURATION:-2m} \
	  -e WS_VUS=$${WS_VUS:-20} \
	  grafana/k6 run - < scripts/load/k6_f6.js

contract: ## Lint del contrato OpenAPI con Spectral (F6)
	bash scripts/contract/check_openapi.sh

iac-validate: ## terraform fmt + validate de infra/terraform (F6)
	cd infra/terraform && terraform fmt -check -recursive \
	  && terraform init -backend=false -input=false >/dev/null \
	  && terraform validate

iac-plan: ## terraform plan offline sin credenciales AWS (F6)
	cd infra/terraform && terraform plan -refresh=false -input=false -no-color \
	  -var offline_plan=true \
	  -var db_password=offline-dummy \
	  -var jwt_secret=offline-dummy-secret-at-least-32-chars \
	  -var ai_api_key=offline-dummy

ci-local: ## Replica los jobs principales del CI en local (F6)
	cd $(BACKEND_DIR) && test -z "$$(gofmt -l .)" && GOTOOLCHAIN=auto go vet ./... \
	  && GOTOOLCHAIN=auto go test -race ./... && GOTOOLCHAIN=auto go build ./...
	cd web && npm run typecheck && npm run lint && npm test && npm run check:api
	cd mobile && npm run typecheck && npm test
	$(MAKE) contract
	$(MAKE) iac-validate
	$(COMPOSE) config -q

clean: ## Detiene el stack y borra volúmenes (¡borra datos locales!)
	$(COMPOSE) down -v
