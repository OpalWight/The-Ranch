APP_NAME := the-ranch
GO := go
DOCKER := docker

GHCR_REPO := ghcr.io/opalwight/the-ranch

.PHONY: build run run-worker test lint clean docker-build docker-push deploy infra-up infra-down migrate jenkins-build jenkins-up jenkins-down jenkins-logs

build:
	$(GO) build -o bin/api ./cmd/api
	$(GO) build -o bin/worker ./cmd/worker

run: ## Run the API server (requires infra-up + migrate first)
	DATABASE_URL="postgres://filesync:changeme@localhost:5432/filesync?sslmode=disable" \
	MINIO_ENDPOINT="localhost:9000" \
	MINIO_ACCESS_KEY="minioadmin" \
	MINIO_SECRET_KEY="changeme" \
	MINIO_BUCKET="filesync" \
	MINIO_USE_SSL="false" \
	REDIS_URL="redis://:changeme@localhost:6379/0" \
	$(GO) run ./cmd/api

run-worker: ## Run the worker (requires infra-up + migrate first)
	DATABASE_URL="postgres://filesync:changeme@localhost:5432/filesync?sslmode=disable" \
	MINIO_ENDPOINT="localhost:9000" \
	MINIO_ACCESS_KEY="minioadmin" \
	MINIO_SECRET_KEY="changeme" \
	MINIO_BUCKET="filesync" \
	MINIO_USE_SSL="false" \
	REDIS_URL="redis://:changeme@localhost:6379/0" \
	$(GO) run ./cmd/worker

test:
	$(GO) test ./... -v

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

docker-build:
	$(DOCKER) build --target api -t $(APP_NAME):latest .
	$(DOCKER) build --target worker -t $(APP_NAME)-worker:latest .

docker-push: docker-build ## Tag and push images to GHCR
	$(DOCKER) tag $(APP_NAME):latest $(GHCR_REPO)/api:latest
	$(DOCKER) tag $(APP_NAME)-worker:latest $(GHCR_REPO)/worker:latest
	$(DOCKER) push $(GHCR_REPO)/api:latest
	$(DOCKER) push $(GHCR_REPO)/worker:latest

deploy: ## Apply K8s manifests via Kustomize
	kubectl apply -k deploy/k8s/overlays/homelab

infra-up: ## Start Postgres + MinIO + Redis
	$(DOCKER) compose up -d

infra-down: ## Stop Postgres + MinIO + Redis
	$(DOCKER) compose down

migrate: ## Run SQL migrations against local Postgres
	@for f in migrations/*.up.sql; do \
		echo "Running $$f..."; \
		$(DOCKER) exec -i homelab-postgres-1 psql -U filesync -d filesync < "$$f"; \
	done

jenkins-build: ## Build the custom Jenkins image
	$(DOCKER) compose build jenkins

jenkins-up: ## Start Jenkins (and infrastructure if not running)
	$(DOCKER) compose up -d jenkins

jenkins-down: ## Stop Jenkins
	$(DOCKER) compose stop jenkins

jenkins-logs: ## Tail Jenkins logs
	$(DOCKER) compose logs -f jenkins
