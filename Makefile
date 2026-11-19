.PHONY: test generate web-deps web-build build run stop restart telemetry telemetry-stop clean

ADDR ?= 127.0.0.1:3000

ARGS ?= --addr $(ADDR)
CACHE_DIR ?= $(CURDIR)/.cache
export BUF_CACHE_DIR := $(CACHE_DIR)/buf

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
	go test ./... && CI=true pnpm --dir web run test
run: web-build
	@set -a; if [ -f .env.local ]; then . ./.env.local; fi; set +a; \
	if [ -z "$$ATHENA_DATABASE_URL" ]; then echo 'ATHENA_DATABASE_URL is not set. Copy .env.example to .env.local and set the MariaDB connection.' >&2; exit 1; fi; \
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
