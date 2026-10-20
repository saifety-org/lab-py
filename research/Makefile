# CI and local checks must use pinned modules, never a developer's go.work.
export GOWORK := off
export GOFLAGS := -mod=readonly
GOLANGCI_LINT_VERSION := v2.14.0
GOLANGCI_LINT ?= $(CURDIR)/bin/golangci-lint

.PHONY: fmt-check lint lint-install ci-test ci-build

fmt-check:
	bash scripts/check-format.sh

lint-install:
	GOBIN=$(CURDIR)/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

lint: fmt-check vet
	@$(GOLANGCI_LINT) version | grep -F 'version $(GOLANGCI_LINT_VERSION:v%=%) ' >/dev/null || { echo 'Run make lint-install (requires $(GOLANGCI_LINT_VERSION))'; exit 1; }
	$(GOLANGCI_LINT) run --config .golangci.yml ./...
	$(GOLANGCI_LINT) run --config .golangci.yml --build-tags onnx ./...

.PHONY: test vet build train prepare compare regression report confusables missed-injections

test:
	go test ./...

vet:
	go vet ./...

build:
	go build ./cmd/comparison ./cmd/train ./cmd/fetch-data ./cmd/gen-confusables ./cmd/missed-injections ./cmd/model-bridge ./cmd/context-corpus ./cmd/context-expand ./cmd/context-collect ./cmd/context-onnx-bridge

train:
	go run ./cmd/train -data datasets/training -out artifacts/weights.json -seed 1

prepare:
	go run ./cmd/comparison prepare

# Requires the local sAIfety DeBERTa/ONNX cache; no fallback is allowed.
compare: prepare
	go run ./cmd/train -train artifacts/comparison/train.jsonl -out artifacts/comparison/weights.json -seed 1
	go build -tags onnx -o bin/bench ./cmd/bench
	./bin/bench
	go run ./cmd/comparison report

regression:
	go test ./internal/regression -count=1 -v

report:
	go run ./cmd/comparison report

# Rebuild the runtime table into artifacts; never overwrite application sources.
confusables:
	go run ./cmd/gen-confusables

# Diagnostic corpus of known misses; it is not an independent quality benchmark.
missed-injections:
	go run ./cmd/missed-injections

ci-test:
	go test -race -count=1 -timeout=5m ./...

ci-build: build
	go build -tags onnx -o bin/context-onnx-bridge ./cmd/context-onnx-bridge
	go build ./...
	go build -tags onnx -o bin/bench ./cmd/bench

.PHONY: validate-context prepare-context review-context reproduce-context
CONTEXT_SOURCE := datasets/contextual/v4
CONTEXT_GATES := -source $(CONTEXT_SOURCE) -reviews $(CONTEXT_SOURCE)/review.json -require-reviewed -freeze $(CONTEXT_SOURCE)/holdout.json -history datasets/contextual/v3 -catalog $(CONTEXT_SOURCE)/scenarios.json -require-task-balanced
validate-context:
	go run ./cmd/context-corpus $(CONTEXT_GATES)

prepare-context:
	go run ./cmd/context-corpus $(CONTEXT_GATES) -out artifacts/contextual

# Existing data/review/lock are never overwritten by reproduction.
EXPANSION_OUT ?= artifacts/contextual-v3-reproduced
reproduce-context:
	mkdir -p artifacts
	go run ./cmd/context-expand -out $(EXPANSION_OUT)

# A fresh directory prevents overwriting human annotations from earlier runs.
REVIEW_OUT ?= artifacts/context-review-v1
review-context:
	mkdir -p artifacts
	go run ./cmd/context-corpus -review-out $(REVIEW_OUT)

.PHONY: reproduce-context-v4 task-baseline-context
BALANCED_OUT ?= artifacts/contextual-v4-reproduced
reproduce-context-v4:
	mkdir -p artifacts
	go run ./cmd/context-expand -scenarios datasets/contextual/v4/scenarios.json -previous datasets/contextual/v3 -out $(BALANCED_OUT)

TASK_BASELINE_OUT ?= artifacts/task-only-v4
task-baseline-context:
	mkdir -p artifacts
	go run ./cmd/context-corpus $(CONTEXT_GATES) -task-baseline-out $(TASK_BASELINE_OUT)

# Offline candidate reservations only; never calls a model or assigns training splits.
.PHONY: plan-context-v5
CONTEXT_V5_PLAN ?= artifacts/context-collection-v5.plan.json
plan-context-v5:
	mkdir -p artifacts
	go run ./cmd/context-collect plan -input datasets/contextual/v5/protocol.json -sha256 c5bd48ef63338196746a9c0b95f228ab9129283d056db9ab67327bdaba8c1676 -out $(CONTEXT_V5_PLAN)
