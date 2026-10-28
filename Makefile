# Tiendas-Track — comandos de desarrollo local (F1).
COMPOSE ?= docker compose
BACKEND_DIR := backend

.PHONY: dev up down restart logs ps health seed test build vet fmt tidy psql clean help

help: ## Lista de comandos
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

dev: ## Levanta todo el stack (build + detached)
	$(COMPOSE) up --build -d

up: ## Levanta sin reconstruir
	$(COMPOSE) up -d

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

clean: ## Detiene el stack y borra volúmenes (¡borra datos locales!)
	$(COMPOSE) down -v
