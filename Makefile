COMPOSE ?= docker-compose
POSTGRES_USER ?= aprova
POSTGRES_DB ?= aprova

.PHONY: up down migrate run-api run-worker test lint keys verificar-token

up:
	$(COMPOSE) up -d --wait

down:
	$(COMPOSE) down

migrate:
	@for arquivo in migrations/*.up.sql; do \
		echo "aplicando $$arquivo"; \
		$(COMPOSE) exec -T postgres psql -v ON_ERROR_STOP=1 -U $(POSTGRES_USER) -d $(POSTGRES_DB) -f - < "$$arquivo" || exit 1; \
	done

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

keys:
	@go run ./scripts/gerar-chaves

verificar-token:
	@go run ./scripts/verificar-token $(INSTALLATION_ID)

test:
	go test ./...

lint:
	golangci-lint run
