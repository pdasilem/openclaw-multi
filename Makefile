MODULE := github.com/pdasilem/openclaw-multi
BINARIES := openclaw-multi openclaw-overlay-api openclaw-overlay-watcher
BIN_DIR := bin
PREFIX ?= /usr/local
OPT_DIR ?= /opt/openclaw-multi
DESTDIR ?=
INSTALL_BIN_DIR := $(DESTDIR)$(PREFIX)/bin
INSTALL_TEMPLATE_DIR := $(DESTDIR)$(OPT_DIR)/templates

GOOS ?= linux
GO_AMD64 := GOOS=linux GOARCH=amd64
GO_ARM64 := GOOS=linux GOARCH=arm64

GOLANGCI_LINT_VERSION := v1.64.8

.PHONY: all build build-amd64 build-arm64 test test-unit lint schema-check \
        install-check ci clean dev install help test-docker-build test-docker-run test-phase-0

all: build ## Default: build all binaries

build: ## Build all binaries to bin/ (native arch)
	@mkdir -p $(BIN_DIR)
	@for bin in $(BINARIES); do \
		echo "  build $$bin"; \
		go build -o $(BIN_DIR)/$$bin ./cmd/$$bin; \
	done

build-amd64: ## Cross-compile for linux/amd64
	@mkdir -p $(BIN_DIR)
	@for bin in $(BINARIES); do \
		echo "  build $$bin (linux/amd64)"; \
		$(GO_AMD64) go build -o $(BIN_DIR)/$$bin-linux-amd64 ./cmd/$$bin; \
	done

build-arm64: ## Cross-compile for linux/arm64
	@mkdir -p $(BIN_DIR)
	@for bin in $(BINARIES); do \
		echo "  build $$bin (linux/arm64)"; \
		$(GO_ARM64) go build -o $(BIN_DIR)/$$bin-linux-arm64 ./cmd/$$bin; \
	done

test: ## Run all tests with race detector and coverage
	go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

test-unit: ## Run unit tests only (exclude integration/e2e)
	go test -race -coverprofile=coverage.out \
		$(shell go list ./... | grep -v '/test/')

lint: ## Run golangci-lint
	golangci-lint run ./...

schema-check: ## Validate SQL schema and JSON schemas
	@echo "  checking schemas/state.sql..."
	@sqlite3 :memory: < schemas/state.sql && echo "  OK: state.sql"
	@for f in $$(find schemas -maxdepth 1 -name '*.schema.json' -type f); do \
		echo "  checking $$f..."; \
		jq empty $$f && echo "  OK: $$f"; \
	done

shellcheck: ## Run shellcheck on all bash scripts
	@if command -v shellcheck >/dev/null 2>&1; then \
		find scripts/ -name '*.sh' -exec shellcheck {} +; \
	else \
		echo "  shellcheck not installed, skipping"; \
	fi

install-check: build ## Verify install target installs runtime files into expected paths
	@tmpdir="$$(mktemp -d /tmp/openclaw-multi-install.XXXXXX)"; \
	$(MAKE) --no-print-directory install DESTDIR="$$tmpdir"; \
	for bin in $(BINARIES); do \
		test -x "$$tmpdir$(PREFIX)/bin/$$bin"; \
	done; \
	for tmpl in \
		cloudflared-config.tmpl \
		openclaw-overlay-api.service.tmpl \
		openclaw-gateway.service.tmpl \
		openclaw-overlay-watcher.service.tmpl \
		openclaw-backup@.service.tmpl \
		openclaw-backup@.timer.tmpl; do \
		test -f "$$tmpdir$(OPT_DIR)/templates/$$tmpl"; \
	done; \
	rm -rf "$$tmpdir"

ci: lint test build schema-check install-check shellcheck ## Run full CI pipeline

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) coverage.out coverage.html

dev: ## Run TUI locally
	go run ./cmd/openclaw-multi

install: ## Install binaries to /usr/local/bin (requires sudo)
	@if [ -z "$(DESTDIR)" ] && [ "$$(id -u)" != "0" ]; then \
		echo "Warning: install requires root. Run: sudo make install"; \
		exit 1; \
	fi
	@install -d -m 0755 $(INSTALL_BIN_DIR)
	@for bin in $(BINARIES); do \
		install -m 0755 $(BIN_DIR)/$$bin $(INSTALL_BIN_DIR)/$$bin; \
	done
	@install -d -m 0755 $(INSTALL_TEMPLATE_DIR)
	@find templates -maxdepth 1 -type f ! -name '.gitkeep' -exec install -m 0644 {} $(INSTALL_TEMPLATE_DIR)/ \;

test-docker-build: ## Build Docker test image
	docker build -f test/docker/Dockerfile.test -t openclaw-multi-test .

test-docker-run: ## Run Docker test container interactively
	docker run -it --rm --privileged \
		-v /sys/fs/cgroup:/sys/fs/cgroup:rw \
		openclaw-multi-test

test-phase-0: build-amd64 test-docker-build ## Run Phase 0 smoke test
	bash test/e2e/phase-0/smoke.sh

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
