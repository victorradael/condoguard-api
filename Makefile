# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=condoguard
MAIN_PATH=cmd/api/main.go

# Docker parameters
DOCKER_COMPOSE=docker-compose

.PHONY: all build test clean run docker-build docker-run docker-stop deps

all: test build

build:
	$(GOBUILD) -o $(BINARY_NAME) $(MAIN_PATH)

test:
	$(GOTEST) -v ./...

clean:
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME).exe

run:
	$(GOBUILD) -o $(BINARY_NAME) $(MAIN_PATH)
	./$(BINARY_NAME)

docker-build:
	$(DOCKER_COMPOSE) build

docker-run:
	$(DOCKER_COMPOSE) up -d

docker-stop:
	$(DOCKER_COMPOSE) down

deps:
	$(GOMOD) download

lint:
	golangci-lint run

# Development commands
dev:
	air -c .air.toml

migrate:
	go run cmd/migrate/main.go

.PHONY: mock
mock:
	mockgen -source=internal/repository/interfaces.go -destination=internal/repository/mocks/repository_mocks.go

.PHONY: swagger
swagger:
	swag init -g cmd/api/main.go -o docs 