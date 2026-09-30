default: help

.PHONY: help
help: ## Show this help
	@echo
	@echo "Available commands:"
	@echo
	@awk -F ':|##' '/^[^\t].+?:.*?##/ {printf "\033[36m%-30s\033[0m %s\n", $$1, $$NF}' $(MAKEFILE_LIST)

BINDIR=$(shell go env GOPATH)
MODULE=github.com/ushineko/sanshoku

LINT_NAME?=golangci-lint
LINT_VERSION?=v2.12.2
LINT_PROGRAM=$(LINT_NAME)-$(LINT_VERSION)

# Release asset coordinates for the pinned linter version.
# (The upstream install.sh is not used: its checksum extraction matches the
# .sbom.json asset line and fails verification on recent releases.)
LINT_VERSION_NUM=$(LINT_VERSION:v%=%)
LINT_BASE_URL=https://github.com/golangci/golangci-lint/releases/download/$(LINT_VERSION)

.PHONY: install-lint
install-lint: $(BINDIR)/bin/$(LINT_PROGRAM) ## Install the pinned linter

$(BINDIR)/bin/$(LINT_PROGRAM):
	@echo "Setting up $(LINT_PROGRAM) ..."
	@set -e; \
	os=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	arch=$$(uname -m); \
	case "$$arch" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; esac; \
	dist="$(LINT_NAME)-$(LINT_VERSION_NUM)-$$os-$$arch"; \
	tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	curl -fsSL "$(LINT_BASE_URL)/$$dist.tar.gz" -o "$$tmp/$$dist.tar.gz"; \
	curl -fsSL "$(LINT_BASE_URL)/$(LINT_NAME)-$(LINT_VERSION_NUM)-checksums.txt" -o "$$tmp/checksums.txt"; \
	want=$$(awk -v f="$$dist.tar.gz" '$$2 == f {print $$1}' "$$tmp/checksums.txt"); \
	got=$$( (sha256sum "$$tmp/$$dist.tar.gz" 2>/dev/null || shasum -a 256 "$$tmp/$$dist.tar.gz") | awk '{print $$1}'); \
	if [ -z "$$want" ] || [ "$$want" != "$$got" ]; then echo "checksum mismatch for $$dist.tar.gz: want '$$want' got '$$got'"; exit 1; fi; \
	tar -C "$$tmp" -xzf "$$tmp/$$dist.tar.gz"; \
	mkdir -p "$(BINDIR)/bin"; \
	mv -v "$$tmp/$$dist/$(LINT_NAME)" "$(BINDIR)/bin/$(LINT_PROGRAM)"

.PHONY: setup
setup: install-lint ## Set up the machine for local development
	@echo "Make sure your PATH includes $$(go env GOPATH)/bin."

# golangci-lint type-checks against the standard library sources of whichever Go
# it finds, using a go/types built into the linter binary. go.mod deliberately
# carries no `toolchain` line, so the pin lives here instead, matching the Go
# that this linter release was built with. Bump it together with LINT_VERSION.
LINT_GO_TOOLCHAIN?=go1.26.0

.PHONY: lint
lint: export GOTOOLCHAIN = $(LINT_GO_TOOLCHAIN)
lint: install-lint ## Lint the module
	@go version
	$(BINDIR)/bin/$(LINT_PROGRAM) run --timeout 5m0s --config config/.golangci-$(LINT_VERSION).yml ./...

# Unit tests never open a device. Live tests skip when the hardware they need
# is absent, so this target passes on a machine with nothing plugged in.
.PHONY: test
test: ## Run the tests with the race detector
	@go test -race ./...

.PHONY: coverage
coverage: ## Run the tests and open a coverage report
	@go test -coverprofile coverage.out ./...
	@go tool cover -html=coverage.out
	@rm -f coverage.out

.PHONY: vuln
vuln: ## Scan the module for known vulnerabilities
	@govulncheck ./...

.PHONY: build
build: ## Build the testbench (no cgo)
	CGO_ENABLED=0 go build -trimpath -o sanshoku-bench ./cmd/sanshoku-bench

# The testbench is the integration test: it talks to whatever is on the desk.
# `scan` and `read` are read-only. `verify` cross-checks against liquidctl,
# solaar and headsetcontrol when they are on PATH. Writes (the LCD) are behind
# `screen --yes` and are never part of this target.
.PHONY: bench
bench: build ## Run the hardware testbench, read-only
	./sanshoku-bench scan
	./sanshoku-bench read
	./sanshoku-bench verify

# An ignore file governs only what is untracked and can be negated by accident.
# This cannot: it fails if any committed file is an ELF binary.
.PHONY: check-no-binaries
check-no-binaries: ## Fail if a binary is committed
	@bad=$$(git ls-files -z | xargs -0 -r file --mime-type | awk -F': ' '$$2 ~ /application\/x-(executable|pie-executable|sharedlib)/ {print $$1}'); \
	if [ -n "$$bad" ]; then echo "binaries in the repository:"; echo "$$bad"; exit 1; fi

.PHONY: clean
clean: ## Remove build output
	@rm -rf sanshoku-bench coverage.out bin/
