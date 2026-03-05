APP_NAME := the-ranch
GO := go
DOCKER := docker

.PHONY: build run test lint clean docker-build

build:
	$(GO) build -o bin/api ./cmd/api

run:
	$(GO) run ./cmd/api

test:
	$(GO) test ./... -v

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

docker-build:
	$(DOCKER) build -t $(APP_NAME):latest .
