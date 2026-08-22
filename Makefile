DATABASE_URL ?= postgres://batti:batti_local_dev@localhost:5433/batti?sslmode=disable
MIGRATE = go run ./cmd/migrate

.PHONY: db-test help db-up db-down db-reset db-shell migrate migrate-down sqlc scrape scrape-all api test fmt tidy

help:
	@grep -E '^[a-z-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

db-up: ## start local Postgres and wait until it accepts connections
	docker compose up -d
	@printf 'waiting for postgres'
	@until docker compose exec -T postgres pg_isready -U batti -d batti >/dev/null 2>&1; do printf '.'; sleep 1; done
	@echo ' ready on localhost:5433'

db-down: ## stop local Postgres (data volume survives)
	docker compose down

db-reset: ## destroy the local database including its data volume
	docker compose down -v

db-shell: ## open psql against the local database
	docker compose exec postgres psql -U batti -d batti

migrate: ## apply all pending migrations
	$(MIGRATE) up

migrate-down: ## roll back the most recent migration
	$(MIGRATE) -steps 1 down

sqlc: ## regenerate database access code from SQL
	go tool sqlc generate

scrape: ## scrape one vendor, e.g. make scrape VENDOR=jindeal
	go run ./cmd/scrape --vendor=$(VENDOR)

scrape-all: ## scrape every vendor whose hour slot is due
	go run ./cmd/scrape --due-now

api: ## run the API server locally
	go run ./cmd/api

test: ## run the Go test suite
	go test ./...

fmt: ## format all Go code
	gofmt -w .

tidy: ## tidy module requirements
	go mod tidy
db-test: ## run the schema constraint checks
	docker compose exec -T postgres psql -U batti -d batti -v ON_ERROR_STOP=0 -f - < database/tests/constraints_test.sql
