.PHONY: build test test-coverage lint lint-install sqlc sqlc-install sqlc-check openapi-check

BINARY_NAME := voxis-api
MAIN_PATH := ./cmd/server
BUILD_DIR := bin
GO ?= go
GOLANGCI_LINT_VERSION ?= v2.4.0
GOLANGCI_LINT_VERSION_STRIPPED := $(patsubst v%,%,$(GOLANGCI_LINT_VERSION))
GOLANGCI_LINT_BIN := $(CURDIR)/$(BUILD_DIR)/golangci-lint
SQLC_VERSION ?= v1.31.1
# sqlc v1.31.1 requires Go 1.26; application builds and tests keep GO's 1.25 toolchain.
SQLC_GOTOOLCHAIN ?= go1.26.0+auto
REDOCLY_VERSION ?= 2.52.1
SQLC_BIN := $(CURDIR)/$(BUILD_DIR)/sqlc

build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

test:
	$(GO) test -v -race ./...

test-coverage:
	@packages="$$($(GO) list ./... | wc -l | tr -d ' ')"; printf 'Coverage package denominator: %s\n' "$$packages"
	$(GO) test -race -covermode=atomic -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | grep '^total:'

lint: lint-install
	$(GOLANGCI_LINT_BIN) run ./...

lint-install:
	@mkdir -p $(BUILD_DIR)
	@if [ ! -x "$(GOLANGCI_LINT_BIN)" ] || ! "$(GOLANGCI_LINT_BIN)" version 2>/dev/null | grep -q "$(GOLANGCI_LINT_VERSION_STRIPPED)"; then \
		GOBIN="$(CURDIR)/$(BUILD_DIR)" "$(GO)" install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi

sqlc: sqlc-install
	$(SQLC_BIN) generate

sqlc-install:
	@mkdir -p $(BUILD_DIR)
	@if [ ! -x "$(SQLC_BIN)" ] || ! "$(SQLC_BIN)" version 2>/dev/null | grep -q "$(SQLC_VERSION)"; then \
		GOTOOLCHAIN="$(SQLC_GOTOOLCHAIN)" GOBIN="$(CURDIR)/$(BUILD_DIR)" "$(GO)" install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION); \
	fi

sqlc-check: sqlc
	git diff --exit-code -- internal/adapter/postgres/sqlcdb

openapi-check:
	npx --yes @redocly/cli@$(REDOCLY_VERSION) lint api/openapi.yaml
