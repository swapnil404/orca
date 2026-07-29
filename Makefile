SHELL := /bin/bash

ENV_FILE ?= .env
COMPOSE_FILE ?= deploy/docker-compose.yml
POSTGRES_VERSION ?= 17
COMPOSE = docker compose --env-file $(ENV_FILE) -f $(COMPOSE_FILE)

.DEFAULT_GOAL := help

.PHONY: help setup run dev dev-detach dev-web dev-agent dev-migrate stop server agent web migrate proto gen-proto test lint clean postgres-image

help:
	@printf '%s\n' \
		'Usage: make <target>' \
		'' \
		'Targets:' \
		'  setup           Install web dependencies' \
		'  run             Apply migrations and run the server' \
		'  dev             Start Postgres, API, and proxy in foreground' \
		'  dev-detach      Start Postgres, API, and proxy in background' \
		'  dev-web         Run web development on 127.0.0.1:5173' \
		'  dev-agent       Run the agent against the development stack' \
		'  dev-migrate     Apply development database migrations' \
		'  stop            Stop the development stack' \
		'  server          Run the API server' \
		'  agent           Run the agent' \
		'  web             Run the web development server' \
		'  migrate         Apply database migrations' \
		'  test            Run all Go tests' \
		'  lint            Format and vet Go code' \
		'  clean           Remove build output and development volumes' \
		'  gen-proto       Generate Go protobuf code' \
		'  postgres-image  Build the agent PostgreSQL image'

setup:
	npm --prefix web ci

run: migrate server

dev: dev-migrate
	$(COMPOSE) up --build

dev-detach: dev-migrate
	$(COMPOSE) up --build -d

dev-web:
	@set -a; \
	if [[ -f "$(ENV_FILE)" ]]; then source "$(ENV_FILE)"; fi; \
	set +a; \
	npm --prefix web run dev -- --host 127.0.0.1 --port 5173 --strictPort

dev-agent: agent

dev-migrate:
	$(COMPOSE) run --rm migrate

stop:
	$(COMPOSE) down

server:
	@set -a; \
	if [[ -f "$(ENV_FILE)" ]]; then source "$(ENV_FILE)"; fi; \
	set +a; \
	go run ./server/cmd/server

agent:
	@set -a; \
	if [[ -f "$(ENV_FILE)" ]]; then source "$(ENV_FILE)"; fi; \
	set +a; \
	go run ./agent/cmd/agent

web: dev-web

migrate:
	@set -a; \
	if [[ -f "$(ENV_FILE)" ]]; then source "$(ENV_FILE)"; fi; \
	set +a; \
	./scripts/migrate.sh

proto: gen-proto

gen-proto:
	protoc --proto_path=proto \
		--go_out=. \
		--go_opt=module=github.com/swapnil404/orca \
		proto/orca.proto

test:
	go test ./...

lint:
	gofmt -w $$(git ls-files '*.go')
	go vet ./...

clean:
	rm -rf bin web/dist
	$(COMPOSE) down -v

postgres-image:
	docker build --build-arg POSTGRES_VERSION=$(POSTGRES_VERSION) \
		-t orca-postgres:$(POSTGRES_VERSION) agent/images/postgres
