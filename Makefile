COMPOSE ?= docker-compose
POSTGRES_USER ?= aprova
POSTGRES_DB ?= aprova
DATABASE_URL ?= postgres://aprova:aprova@localhost:5432/aprova?sslmode=disable
MIGRATE ?= migrate
SQLC ?= sqlc

.PHONY: up down migrate migrate-up migrate-down migrate-reset sqlc test-banco run-api run-worker test lint keys verificar-token sandbox-azure consultar-permissao imagem-sandbox-go

up:
	$(COMPOSE) up -d --wait

down:
	$(COMPOSE) down

migrate: migrate-up

migrate-up:
	$(MIGRATE) -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	$(MIGRATE) -path migrations -database "$(DATABASE_URL)" down 1

migrate-reset:
	$(MIGRATE) -path migrations -database "$(DATABASE_URL)" down -all

sqlc:
	$(SQLC) generate

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

keys:
	@go run ./scripts/gerar-chaves

verificar-token:
	@go run ./scripts/verificar-token $(INSTALLATION_ID)

sandbox-azure: export REPOSITORIO := $(REPOSITORIO)
sandbox-azure: export SHA := $(SHA)
sandbox-azure: export DIRETORIO := $(DIRETORIO)
sandbox-azure:
	@go run ./scripts/disparar-sandbox-azure "$$REPOSITORIO" "$$SHA" "$$DIRETORIO"

imagem-sandbox-go:
	docker build -f imagens/go/Dockerfile -t aprova-sandbox-go:local .

consultar-permissao:
	@go run ./scripts/consultar-permissao $(INSTALLATION_ID) $(REPOSITORIO) $(LOGIN)

test:
	go test ./...

test-banco:
	go test ./tests/integration/postgres/... -race -count=1 -v

lint:
	golangci-lint run
