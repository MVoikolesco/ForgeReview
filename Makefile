APP_NAME := gitea-agents
MAIN_PACKAGE := ./cmd/server

.PHONY: run build test docker fmt lint

run:
	go run $(MAIN_PACKAGE)

build:
	go build -o bin/$(APP_NAME) $(MAIN_PACKAGE)

test:
	go test ./...

docker:
	docker compose up --build

fmt:
	go fmt ./...

lint:
	@echo "lint placeholder"
