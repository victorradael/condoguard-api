.PHONY: test test-unit test-integration lint fmt vet build run dev-up dev-down tidy logs up down

GOBIN ?= $(shell go env GOPATH)/bin
BINARY = condoguard-api

# ── Build & Run ────────────────────────────────────────────────────────────────
build:
	go build -o bin/$(BINARY) ./cmd/server

run: build
	./bin/$(BINARY)

# ── Tests ─────────────────────────────────────────────────────────────────────
test:
	go test ./... -race -count=1

test-unit:
	go test ./... -short -race -count=1

test-integration:
	go test ./... -run Integration -race -count=1

test-cover:
	go test ./... -race -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out

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
	docker compose up --build -d api mongodb

down:
	docker compose down

logs:
	docker compose logs -f api

dev-up:
	docker compose up -d mongodb

dev-down:
	docker compose down

test-db-up:
	docker compose up -d mongodb-test

test-db-down:
	docker compose stop mongodb-test && docker compose rm -f mongodb-test
