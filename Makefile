.PHONY: seed seed-poc reset-demo test test-unit test-integration test-cover lint fmt vet build run up down logs dev-up dev-down \
	tidy sqlc sqlc-check migrate-up migrate-down migrate-reset migrate-version test-db-up test-db-down

BINARY = condoguard-api

# Bancos de teste do docker-compose (perfil "test").
TEST_DATABASE_URL ?= postgres://postgres@localhost:55432/postgres?sslmode=disable
TEST_REDIS_URL    ?= redis://localhost:56379/0

# ── Build & Run ────────────────────────────────────────────────────────────────
build:
	go build -o bin/$(BINARY) ./cmd/server

run: build
	./bin/$(BINARY)

# ── Tests ─────────────────────────────────────────────────────────────────────
# Sem TEST_DATABASE_URL os testes de PostgreSQL são pulados.
test:
	go test ./... -race -count=1

test-unit:
	go test ./... -short -race -count=1

# Repositórios, migrações, paridade e Redis contra bancos reais.
test-integration:
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" TEST_REDIS_URL="$(TEST_REDIS_URL)" \
		go test ./... -race -count=1

test-cover:
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" TEST_REDIS_URL="$(TEST_REDIS_URL)" \
		go test ./... -race -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out

test-db-up:
	docker compose --profile test up -d --wait postgres-test redis-test

test-db-down:
	docker compose --profile test rm -sf postgres-test redis-test

# ── Database ──────────────────────────────────────────────────────────────────
# Consultas tipadas: edite internal/db/queries/*.sql e rode `make sqlc`.
sqlc:
	sqlc generate

sqlc-check:
	sqlc diff

# Migrações goose (embutidas no binário; o servidor aplica "up" ao subir).
migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-reset:
	go run ./cmd/migrate reset

migrate-version:
	go run ./cmd/migrate version

# ── Code Quality ──────────────────────────────────────────────────────────────
fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	golangci-lint run ./...

# ── Dependencies ──────────────────────────────────────────────────────────────
tidy:
	go mod tidy

# ── Docker ────────────────────────────────────────────────────────────────────
up:
	docker compose up --build -d api postgres redis

down:
	docker compose down

logs:
	docker compose logs -f api

dev-up:
	docker compose up -d --wait postgres redis

dev-down:
	docker compose down

# ── Dados fictícios (is_demo) ─────────────────────────────────────────────────
# Precisam de DATABASE_URL; reset-demo também de DATABASE_MIGRATIONS_URL.
# SEED_PASSWORD define a senha das contas de teste (sem ela, uma aleatória é impressa).
seed:
	go run ./cmd/seed --profile=seed

seed-poc:
	go run ./cmd/seed --profile=poc

reset-demo:
	go run ./cmd/seed --profile=poc --reset-demo
