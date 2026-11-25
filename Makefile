.PHONY: test bruno-test generate web-deps web-build build run stop restart telemetry telemetry-stop clean

ADDR ?= 127.0.0.1:3000

ARGS ?= --addr $(ADDR)
CACHE_DIR ?= $(CURDIR)/.cache
export BUF_CACHE_DIR := $(CACHE_DIR)/buf
# Parallel Go test packages. 4 won the 2026-11-24 shootout (513s full green
# vs 836s at -p 2); override per run with make test GO_TEST_P=N.
GO_TEST_P ?= 4

generate: web-deps
	buf generate

web-deps:
	CI=true pnpm --dir web install --frozen-lockfile --store-dir $(CACHE_DIR)/pnpm

web-build: web-deps
	CI=true pnpm --dir web run typecheck
	CI=true pnpm --dir web run build

build: web-build
	mkdir -p bin
	go build -o bin/athena ./cmd/athena

# Go and web tests. MariaDB tests use ATHENA_TEST_DATABASE_URL, which defaults
# to ATHENA_DATABASE_URL from .env.local, and skip when neither is set. Each
# test creates and drops its own test_athena_* database.
test: web-deps
	@set -a; if [ -f .env.local ]; then . ./.env.local; fi; set +a; \
	export ATHENA_TEST_DATABASE_URL="$${ATHENA_TEST_DATABASE_URL:-$$ATHENA_DATABASE_URL}"; \
	go test -p $(GO_TEST_P) -timeout 20m ./... && CI=true pnpm --dir web run test

# Black-box ConnectRPC checks. Athena must already be running and the Bruno
# CLI must be installed (`pnpm add --global @usebruno/cli`). The password is
# passed only to the runner and never stored in the collection.
BRUNO ?= bru
BRUNO_COLLECTION ?= $(CURDIR)/bruno/athena-api
BRUNO_REPORT ?= /tmp/athena-bruno-junit.xml

bruno-test:
	@if ! command -v $(firstword $(BRUNO)) >/dev/null 2>&1; then echo 'Bruno CLI not found. Install it with: pnpm add --global @usebruno/cli' >&2; exit 1; fi
	@if [ -z "$${ATHENA_BRUNO_PASSWORD:-}" ]; then echo 'ATHENA_BRUNO_PASSWORD is required; it is never written to disk.' >&2; exit 1; fi
	@curl --fail --silent --show-error "$${ATHENA_BRUNO_BASE_URL:-http://127.0.0.1:3000}/healthz" >/dev/null
	@cd "$(BRUNO_COLLECTION)" && $(BRUNO) run --env local \
		--env-var "baseUrl=$${ATHENA_BRUNO_BASE_URL:-http://127.0.0.1:3000}" \
		--env-var "username=$${ATHENA_BRUNO_USERNAME:-administrator}" \
		--env-var "password=$${ATHENA_BRUNO_PASSWORD}" \
		--reporter-junit $(BRUNO_REPORT) \
		--reporter-skip-body
run: web-build
	@set -a; if [ -f .env.local ]; then . ./.env.local; fi; set +a; \
	if [ -z "$$ATHENA_DATABASE_URL" ]; then echo 'ATHENA_DATABASE_URL is not set. Copy .env.example to .env.local and set the MariaDB connection.' >&2; exit 1; fi; \
	export ATHENA_FILES_DIR="$${ATHENA_FILES_DIR:-.athena/files}"; \
	go run ./cmd/athena serve $(ARGS)

stop:
	@./scripts/stop-athena.sh $(ADDR)
# Stop any running Athena on ADDR, then rebuild and start it again.
restart:
	@$(MAKE) --no-print-directory stop
	@$(MAKE) --no-print-directory run
# Local Grafana with an OpenTelemetry collector, Tempo, Prometheus and Loki.
# Grafana: http://127.0.0.1:3001. OTLP/HTTP: http://127.0.0.1:4318. All ports bind to loopback.
LGTM_IMAGE ?= grafana/otel-lgtm:0.41.0
telemetry:
	@docker inspect athena-lgtm >/dev/null 2>&1 && docker start athena-lgtm >/dev/null || \
	docker run -d --name athena-lgtm -p 127.0.0.1:3001:3000 -p 127.0.0.1:4317:4317 -p 127.0.0.1:4318:4318 $(LGTM_IMAGE) >/dev/null
	@echo "Grafana: http://127.0.0.1:3001 (set OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 in .env.local, then make restart)"
telemetry-stop:
	@docker stop athena-lgtm >/dev/null 2>&1 || true
clean:
	rm -rf bin internal/webui/dist/assets internal/webui/dist/index.html
