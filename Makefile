.PHONY: help build build-cli test test-v test-cover lint fmt vet vulncheck clean deps verify install install-requirements vm-deploy run smoke config talos talos-config talos-generate node-create node-delete node-recreate node-update check-go

# Auto-download Go toolchain if local version < go.mod requirement
export GOTOOLCHAIN=auto

SHELL := /bin/bash

# ANSI colors (actual ESC char via printf - works in both printf and sed)
ESC    := $(shell printf '\033')
BOLD   := $(ESC)[1m
CYAN   := $(ESC)[36m
GREEN  := $(ESC)[32m
YELLOW := $(ESC)[33m
RED    := $(ESC)[31m
GREY   := $(ESC)[90m
RESET  := $(ESC)[0m

# Default target
help:
	@printf "\n$(BOLD)vmware-vm-bootstrap$(RESET)\n"
	@printf "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)\n"
	@printf "\n$(BOLD)  Development$(RESET)\n"
	@printf "    $(GREEN)make build$(RESET)         Compile check - verify all packages compile\n"
	@printf "    $(GREEN)make fmt$(RESET)           Auto-format source files (lists changed files)\n"
	@printf "    $(GREEN)make vet$(RESET)           Detect suspicious code patterns (silence = OK)\n"
	@printf "    $(GREEN)make lint$(RESET)          Deep linting via golangci-lint (superset of vet)\n"
	@printf "\n$(BOLD)  Setup$(RESET)\n"
	@printf "    $(GREEN)make install-requirements$(RESET)  Install all required external tools\n"
	@printf "\n$(BOLD)  Testing$(RESET)\n"
	@printf "    $(GREEN)make test$(RESET)          Run all tests\n"
	@printf "    $(GREEN)make test-v$(RESET)        Run tests with verbose output\n"
	@printf "    $(GREEN)make test-cover$(RESET)    Run tests + generate HTML coverage report\n"
	@printf "    $(GREEN)make vulncheck$(RESET)     Run govulncheck security scan\n"
	@printf "\n$(BOLD)  VM Management$(RESET) $(YELLOW)(requires configs/vcenter.sops.yaml)$(RESET)\n"
	@printf "    $(GREEN)make config$(RESET)        Interactive config manager (create/edit VM configs)\n"
	@printf "    $(GREEN)make talos-config$(RESET)  Configure Talos schematics (extensions → schematic ID)\n"
	@printf "    $(GREEN)make talos-generate$(RESET) Generate Talos vm.* configs from cluster plan\n"
	@printf "    $(GREEN)make vm-deploy$(RESET)     Select a non-Talos VM config and bootstrap it\n"
	@printf "    $(GREEN)make smoke$(RESET)         Bootstrap + minimal post-install checks + cleanup\n"
	@printf "\n$(BOLD)  Node Lifecycle$(RESET)\n"
	@printf "    $(GREEN)make node-create$(RESET)   Create node from VM config\n"
	@printf "    $(GREEN)make node-delete$(RESET)   Delete node from VM config\n"
	@printf "    $(GREEN)make node-recreate$(RESET) Delete + create node from VM config\n"
	@printf "    $(GREEN)make node-update$(RESET)   Upgrade Talos node OS via talosctl\n"
	@printf "\n$(BOLD)  Maintenance$(RESET)\n"
	@printf "    $(GREEN)make clean$(RESET)         Remove build artifacts and caches\n"
	@printf "    $(GREEN)make deps$(RESET)          Download + tidy dependencies\n"
	@printf "    $(GREEN)make verify$(RESET)        Verify dependency checksums\n"
	@printf "\n$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)\n\n"

# Ensure local Go toolchain meets go.mod requirement (avoids toolchain auto confusion)
GO_REQUIRED := $(shell awk '/^go /{print $$2}' go.mod)
check-go:
	@{ \
		LOCAL_VER=$$(GOTOOLCHAIN=local go env GOVERSION 2>/dev/null | sed 's/^go//' || echo "unknown"); \
		REQUIRED="$(GO_REQUIRED)"; \
		if [ "$$LOCAL_VER" = "unknown" ]; then \
			printf "$(YELLOW)Go not found in PATH (local toolchain).$(RESET)\n"; \
			printf "  Run: $(GREEN)make install-requirements$(RESET)\n\n"; \
			exit 1; \
		fi; \
		LOWEST=$$(printf '%s\n%s\n' "$$REQUIRED" "$$LOCAL_VER" | sort -V | head -n1); \
		if [ "$$LOWEST" != "$$REQUIRED" ]; then \
			printf "$(YELLOW)Local Go toolchain too old (local: $$LOCAL_VER, required: $$REQUIRED).$(RESET)\n"; \
			printf "  Run: $(GREEN)make install-requirements$(RESET)\n\n"; \
			exit 1; \
		fi; \
	}

# Compile check - verifies all packages compile (library: no binary output)
build: check-go
	@printf "$(CYAN)Compiling all packages...$(RESET)\n"
	@go build ./...
	@printf "$(GREEN)✓ All packages compile OK$(RESET)\n"

# Build vmbootstrap CLI binary
build-cli: check-go
	@printf "$(CYAN)Building vmbootstrap...$(RESET)\n"
	@mkdir -p bin
	@go build -o bin/vmbootstrap ./cmd/vmbootstrap
	@printf "$(GREEN)✓ Built: bin/vmbootstrap$(RESET)\n"

# Run tests
test: check-go
	@printf "$(CYAN)Running tests...$(RESET)\n"
	@set -o pipefail; go test ./... 2>&1 | sed \
		-e 's|^ok |$(GREEN)ok$(RESET) |g' \
		-e 's|^FAIL|$(RED)FAIL$(RESET)|g' \
		-e 's|^?|$(GREY)?|g' \
		-e 's|\[no test files\]|$(GREY)[no test files]$(RESET)|g'

# Run tests with verbose output
test-v: check-go
	@printf "$(CYAN)Running tests (verbose)...$(RESET)\n"
	@set -o pipefail; go test -v ./... 2>&1 | sed \
		-e 's|^ok |$(GREEN)ok$(RESET) |g' \
		-e 's|^FAIL|$(RED)FAIL$(RESET)|g' \
		-e 's|^PASS$$|$(GREEN)PASS$(RESET)|g' \
		-e 's|^--- PASS|$(GREEN)--- PASS$(RESET)|g' \
		-e 's|^    --- PASS|    $(GREEN)--- PASS$(RESET)|g' \
		-e 's|^--- FAIL|$(RED)--- FAIL$(RESET)|g' \
		-e 's|^    --- FAIL|    $(RED)--- FAIL$(RESET)|g' \
		-e 's|^--- SKIP|$(YELLOW)--- SKIP$(RESET)|g' \
		-e 's|^    --- SKIP|    $(YELLOW)--- SKIP$(RESET)|g' \
		-e 's|^=== RUN|$(GREY)=== RUN$(RESET)|g' \
		-e 's|^    === RUN|    $(GREY)=== RUN$(RESET)|g' \
		-e 's|^?|$(GREY)?|g' \
		-e 's|\[no test files\]|$(GREY)[no test files]$(RESET)|g'

# Run tests with coverage (excludes examples/, scripts/, mocks/ - not library code)
test-cover: check-go
	@printf "$(CYAN)Running tests with coverage...$(RESET)\n"
	@mkdir -p tmp
	@set -o pipefail; \
	PKGS=$$(go list ./... | grep -v '/examples\|/scripts\|/mocks\|/cmd'); \
	GOCOVERDIR= go test -coverprofile=tmp/coverage.out $$PKGS 2>&1 | sed \
		-e 's|^ok |$(GREEN)ok$(RESET) |g' \
		-e 's|^FAIL|$(RED)FAIL$(RESET)|g' \
		-e 's|^?|$(GREY)?|g' \
		-e 's|\[no test files\]|$(GREY)[no test files]$(RESET)|g'; \
	go tool cover -html=tmp/coverage.out -o tmp/coverage.html
	@printf "$(GREEN)✓ Coverage report: tmp/coverage.html$(RESET)\n"

# Run linter (requires golangci-lint)
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null)
ifeq ($(GOLANGCI_LINT),)
GOLANGCI_LINT := $(shell GOPATH=$$(go env GOPATH); [ -x "$$GOPATH/bin/golangci-lint" ] && echo "$$GOPATH/bin/golangci-lint")
endif
ifeq ($(GOLANGCI_LINT),)
GOLANGCI_LINT := $(shell [ -x "/usr/local/bin/golangci-lint" ] && echo "/usr/local/bin/golangci-lint")
endif

lint:
	@{ \
		if [ -z "$(GOLANGCI_LINT)" ]; then \
			printf "$(YELLOW)golangci-lint not installed.$(RESET)\n"; \
			printf "  Run: $(GREEN)make install-requirements$(RESET)\n\n"; \
			exit 0; \
		fi; \
	}; \
	printf "$(CYAN)Running golangci-lint...$(RESET)\n"; \
	"$(GOLANGCI_LINT)" run ./...

# Format code
fmt:
	@printf "$(CYAN)Formatting code...$(RESET)\n"
	@go fmt ./...
	@printf "$(GREEN)✓ Done$(RESET)\n"

# Run govulncheck (requires govulncheck)
GO_BIN_DIR := $(shell [ -x "/usr/local/go/bin/go" ] && echo "/usr/local/go/bin")
GOVULNCHECK := $(shell GOPATH=$$(go env GOPATH); [ -x "$$GOPATH/bin/govulncheck" ] && echo "$$GOPATH/bin/govulncheck")
ifeq ($(GOVULNCHECK),)
GOVULNCHECK := $(shell [ -x "/usr/local/bin/govulncheck" ] && echo "/usr/local/bin/govulncheck")
endif
ifeq ($(GOVULNCHECK),)
GOVULNCHECK := $(shell command -v govulncheck 2>/dev/null)
endif
GO_REQUIRED := $(shell awk '/^go /{print $$2}' go.mod)
GO_LOCAL_VERSION := $(shell GOTOOLCHAIN=local go env GOVERSION 2>/dev/null | sed 's/^go//')
GO_LOCAL_OK := $(shell [ -n "$(GO_LOCAL_VERSION)" ] && [ "$$(printf '%s\n%s\n' "$(GO_REQUIRED)" "$(GO_LOCAL_VERSION)" | sort -V | head -n1)" = "$(GO_REQUIRED)" ] && echo "yes")

vulncheck:
	@{ \
		if [ -z "$(GOVULNCHECK)" ]; then \
			printf "$(YELLOW)govulncheck not installed.$(RESET)\n"; \
			printf "  Run: $(GREEN)make install-requirements$(RESET)\n\n"; \
			exit 0; \
		fi; \
	}; \
	printf "$(CYAN)Running govulncheck...$(RESET)\n"; \
	if [ "$(GO_LOCAL_OK)" = "yes" ]; then \
		PATH="$(GO_BIN_DIR):$$PATH" GOTOOLCHAIN=local "$(GOVULNCHECK)" ./...; \
	else \
		PATH="$(GO_BIN_DIR):$$PATH" GOTOOLCHAIN=auto "$(GOVULNCHECK)" ./...; \
	fi

# Run go vet
vet: check-go
	@printf "$(CYAN)Running go vet...$(RESET)\n"
	@go vet ./...
	@printf "$(GREEN)✓ Done$(RESET)\n"

# Clean build artifacts
clean:
	@printf "$(CYAN)Cleaning build artifacts...$(RESET)\n"
	@rm -f tmp/coverage.out tmp/coverage.html bin/vmbootstrap
	@go clean -testcache
	@printf "$(GREEN)✓ Clean complete$(RESET)\n"

# Download + tidy dependencies
deps: check-go
	@printf "$(CYAN)Updating dependencies...$(RESET)\n"
	@go mod download
	@go mod tidy
	@printf "$(GREEN)✓ Dependencies updated$(RESET)\n"

# Verify dependency checksums
verify: check-go
	@printf "$(CYAN)Verifying dependencies...$(RESET)\n"
	@go mod verify
	@printf "$(GREEN)✓ All dependencies verified$(RESET)\n"

# Select a VM config interactively and bootstrap it
vm-deploy: build-cli
	@bin/vmbootstrap run $(if $(VCENTER_CONFIG),--vcenter-config $(VCENTER_CONFIG),) $(if $(DEBUG),--debug,)

# Backward-compatible alias (hidden from help menu)
run: vm-deploy

# Smoke test (bootstrap + minimal post-install checks + optional cleanup)
smoke: build-cli
	@bin/vmbootstrap smoke $(if $(VM),--config $(VM),) $(if $(VCENTER_CONFIG),--vcenter-config $(VCENTER_CONFIG),) $(if $(DEBUG),--debug,)

# Node lifecycle commands
node-create: build-cli
	@bin/vmbootstrap node create $(if $(VM),--config $(VM),) $(if $(VCENTER_CONFIG),--vcenter-config $(VCENTER_CONFIG),) $(if $(DEBUG),--debug,)

node-delete: build-cli
	@bin/vmbootstrap node delete $(if $(VM),--config $(VM),) $(if $(VCENTER_CONFIG),--vcenter-config $(VCENTER_CONFIG),) $(if $(DEBUG),--debug,)

node-recreate: build-cli
	@bin/vmbootstrap node recreate $(if $(VM),--config $(VM),) $(if $(VCENTER_CONFIG),--vcenter-config $(VCENTER_CONFIG),) $(if $(DEBUG),--debug,)

node-update: build-cli
	@bin/vmbootstrap node update $(if $(VM),--config $(VM),) $(if $(VCENTER_CONFIG),--vcenter-config $(VCENTER_CONFIG),) $(if $(VERSION),--to-version $(VERSION),) $(if $(TALOSCONFIG),--talosconfig $(TALOSCONFIG),) $(if $(ENDPOINT),--endpoint $(ENDPOINT),) $(if $(PRESERVE),--preserve,) $(if $(INSECURE),--insecure,) $(if $(DEBUG),--debug,)

# Interactive config manager (create/edit VM configs)
config: build-cli
	@bin/vmbootstrap $(if $(VCENTER_CONFIG),--vcenter-config $(VCENTER_CONFIG),) $(if $(DEBUG),--debug,)

# Configure Talos image schematics (extensions -> schematic ID)
talos-config: build-cli
	@bin/vmbootstrap talos config $(if $(VCENTER_CONFIG),--vcenter-config $(VCENTER_CONFIG),) $(if $(DEBUG),--debug,)

# Generate Talos node vm.* configs from cluster plan file.
talos-generate: build-cli
	@bin/vmbootstrap talos generate $(if $(PLAN),--config $(PLAN),) $(if $(FORCE),--force,) $(if $(VCENTER_CONFIG),--vcenter-config $(VCENTER_CONFIG),) $(if $(DEBUG),--debug,)

# Backward-compatible shortcut.
talos: talos-config

# Install all required external tools
install-requirements:
	@bash scripts/install-requirements.sh

# Install CLI tool (makes vmbootstrap available in PATH)
install: check-go
	@printf "$(CYAN)Installing vmbootstrap CLI...$(RESET)\n"
	@go install ./cmd/vmbootstrap
	@printf "$(GREEN)✓ Installed! Run: vmbootstrap$(RESET)\n"
