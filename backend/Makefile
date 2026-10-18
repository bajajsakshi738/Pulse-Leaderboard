.PHONY: run run-generator build migrate tidy docker-up docker-down

# Run the API server locally (requires Postgres + Redis already running,
# e.g. via `make docker-up`, and a .env file based on .env.example).
run:
	go run ./cmd/server

# Run the fake score generator locally against a running API.
run-generator:
	go run ./cmd/generator

build:
	go build -o bin/server ./cmd/server
	go build -o bin/generator ./cmd/generator

# Apply the SQL migration to whatever DATABASE_URL is set in your
# environment or .env file. Requires the `psql` CLI to be installed.
migrate:
	psql "$$DATABASE_URL" -f migrations/0001_init.sql

tidy:
	go mod tidy

# Bring up Postgres + Redis only, for local development against
# `go run` directly (no Docker build needed for the Go binaries).
docker-up:
	docker compose up -d postgres redis

docker-down:
	docker compose down
