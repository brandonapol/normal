GO ?= go
BIN_DIR := bin
TOOLS_DIR := $(BIN_DIR)/tools
GOLANGCI_LINT_VERSION ?= v2.6.2
GOVULNCHECK_VERSION ?= v1.1.4
COVERAGE_THRESHOLD ?= 75
COVERAGE_FILE := coverage.out
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint
GOVULNCHECK := $(TOOLS_DIR)/govulncheck

.DEFAULT_GOAL := help

.PHONY: help
help:
	@echo "normal - control plane for an opinionated phone OS"
	@echo
	@echo "  make ci            everything CI runs, in CI order"
	@echo "  make test          unit tests"
	@echo "  make test-race     unit tests under the race detector"
	@echo "  make cover         coverage report, gated at $(COVERAGE_THRESHOLD)%"
	@echo "  make lint          golangci-lint"
	@echo "  make fmt           format Go and CUE in place"
	@echo "  make schema        cue fmt check and cue vet of the examples"
	@echo "  make invariants    verify the frozen invariant corpus via the CLI"
	@echo "  make drift         confirm generated files match their source"
	@echo "  make build         host binary into $(BIN_DIR)"
	@echo "  make build-arm64   static linux/arm64 daemon binary"
	@echo "  make vuln          govulncheck"
	@echo "  make clean"

$(TOOLS_DIR):
	@mkdir -p $(TOOLS_DIR)

$(GOLANGCI_LINT): | $(TOOLS_DIR)
	GOBIN=$(CURDIR)/$(TOOLS_DIR) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOVULNCHECK): | $(TOOLS_DIR)
	GOBIN=$(CURDIR)/$(TOOLS_DIR) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

.PHONY: tools
tools: $(GOLANGCI_LINT) $(GOVULNCHECK)

.PHONY: fmt
fmt:
	$(GO) fmt ./...
	$(GO) tool cue fmt ./schema/...

.PHONY: fmt-check
fmt-check:
	@unformatted="$$(gofmt -l . | grep -v '^$(BIN_DIR)/' || true)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; echo "$$unformatted"; exit 1; \
	fi
	@echo "gofmt clean"

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

.PHONY: test
test:
	$(GO) test ./...

.PHONY: test-race
test-race:
	$(GO) test -race ./...

.PHONY: cover
cover:
	$(GO) test -covermode=atomic -coverprofile=$(COVERAGE_FILE) ./...
	@$(GO) tool cover -func=$(COVERAGE_FILE) | tail -1
	@total=$$($(GO) tool cover -func=$(COVERAGE_FILE) | awk '/^total:/ {print substr($$3, 1, length($$3)-1)}'); \
	awk -v total="$$total" -v want="$(COVERAGE_THRESHOLD)" 'BEGIN { \
		if (total + 0 < want + 0) { printf "coverage %.1f%% is below the %s%% floor\n", total, want; exit 1 } \
		printf "coverage %.1f%% meets the %s%% floor\n", total, want }'

.PHONY: schema-fmt-check
schema-fmt-check:
	$(GO) tool cue fmt --check ./schema/...
	@echo "cue fmt clean"

.PHONY: schema-vet
schema-vet:
	@for fixture in examples/*.json testdata/invariants/accept/*.json; do \
		$(GO) tool cue vet -d '#PhoneConfig' schema/normal.cue "$$fixture" || exit 1; \
		echo "cue vet ok  $$fixture"; \
	done

.PHONY: schema
schema: schema-fmt-check schema-vet

.PHONY: invariants
invariants:
	./scripts/check-invariants.sh

.PHONY: drift
drift:
	@$(GO) run ./cmd/normalctl baseline > /tmp/normal-baseline.json
	@if ! diff -q /tmp/normal-baseline.json examples/baseline.config.json >/dev/null; then \
		echo "examples/baseline.config.json is stale"; \
		echo "regenerate with: go run ./cmd/normalctl baseline > examples/baseline.config.json"; \
		diff -u examples/baseline.config.json /tmp/normal-baseline.json | head -20; \
		exit 1; \
	fi
	@echo "generated examples are current"

.PHONY: tidy-check
tidy-check:
	$(GO) mod tidy
	@if ! git diff --quiet -- go.mod go.sum; then \
		echo "go.mod or go.sum is untidy; run: go mod tidy"; \
		git diff -- go.mod go.sum; exit 1; \
	fi
	@echo "go.mod and go.sum are tidy"

.PHONY: build
build:
	$(GO) build -o $(BIN_DIR)/normalctl ./cmd/normalctl

.PHONY: build-arm64
build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags="-s -w" \
		-o $(BIN_DIR)/normalctl-linux-arm64 ./cmd/normalctl
	@ls -lh $(BIN_DIR)/normalctl-linux-arm64 | awk '{print "linux/arm64 static binary:", $$5}'

.PHONY: vuln
vuln: $(GOVULNCHECK)
	$(GOVULNCHECK) ./...

.PHONY: ci
ci: tidy-check fmt-check schema vet lint test-race cover invariants drift build-arm64
	@echo
	@echo "all checks passed"

.PHONY: clean
clean:
	rm -rf $(BIN_DIR) $(COVERAGE_FILE)
