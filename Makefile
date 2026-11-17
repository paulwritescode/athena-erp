.PHONY: generate web-deps web-build build run clean

ARGS ?= --addr :3000
CACHE_DIR ?= $(CURDIR)/.cache
export BUF_CACHE_DIR := $(CACHE_DIR)/buf

generate: web-deps
	buf generate

web-deps:
	NPM_CONFIG_CACHE=$(CACHE_DIR)/npm npm --prefix web ci

web-build: web-deps
	npm --prefix web run typecheck
	npm --prefix web run build

build: web-build
	mkdir -p bin
	go build -o bin/athena ./cmd/athena

run: web-build
	go run ./cmd/athena serve $(ARGS)

clean:
	rm -rf bin internal/webui/dist/assets internal/webui/dist/index.html
