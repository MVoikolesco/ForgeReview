APP_NAME := gitea-agents
MAIN_PACKAGE := ./cmd/server

.PHONY: run build test docker fmt lint check

run:
	go run $(MAIN_PACKAGE)

build:
	go build -o bin/$(APP_NAME) $(MAIN_PACKAGE)

test:
	go test ./...

docker:
	docker compose -f docker/compose.yml up --build

production:
	bash production/build.sh

fmt:
	go fmt ./...

lint:
	go vet ./...
	npm --prefix web-admin run lint

check: test lint
