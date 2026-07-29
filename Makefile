SHELL := /bin/bash

ENV_FILE ?= .env
POSTGRES_VERSION ?= 17

.DEFAULT_GOAL := help

.PHONY: help setup run dev server agent web migrate gen-proto postgres-image

help:
	@printf '%s\n' \
		'Usage: make <target>' \
		'' \
		'Targets:' \
		'  setup           Install web dependencies' \
		'  run             Apply migrations and run the server' \
		'  dev             Run the server and web app together' \
		'  server          Run the API server' \
		'  agent           Run the agent' \
		'  web             Run the web development server' \
		'  migrate         Apply database migrations' \
		'  gen-proto       Generate Go protobuf code' \
		'  postgres-image  Build the agent PostgreSQL image'

setup:
	npm --prefix web ci

run: migrate server

dev: migrate
	$(MAKE) --no-print-directory -j2 server web

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

web:
	npm --prefix web run dev

migrate:
	@set -a; \
	if [[ -f "$(ENV_FILE)" ]]; then source "$(ENV_FILE)"; fi; \
	set +a; \
	./scripts/migrate.sh

gen-proto:
	protoc --proto_path=proto \
		--go_out=. \
		--go_opt=module=github.com/swapnil404/orca \
		proto/orca.proto

postgres-image:
	docker build --build-arg POSTGRES_VERSION=$(POSTGRES_VERSION) \
		-t orca-postgres:$(POSTGRES_VERSION) agent/images/postgres
