.PHONY: sync lint test build check bootstrap prepare train compare misses test-ml
sync:
	uv sync --locked
lint:
	uv run --locked ruff check .
	uv run --locked ruff format --check .
test:
	uv run --locked pytest -m 'not ml and not harness'
build:
	uv run --locked python -m build --no-isolation
check: lint test build
bootstrap:
	uv run --locked saifety-lab bootstrap
prepare:
	uv run --locked saifety-lab prepare
train:
	uv run --locked saifety-lab train-native
compare:
	uv run --locked saifety-lab compare-native
misses:
	uv run --locked saifety-lab misses
test-ml:
	uv run --locked --extra ml pytest -m ml

.PHONY: bootstrap-context prepare-context train-context compare-context
bootstrap-context:
	uv run --locked saifety-lab bootstrap-context
prepare-context:
	uv run --locked saifety-lab prepare-context
train-context:
	uv run --locked saifety-lab train-context
compare-context:
	uv run --locked saifety-lab compare-context

.PHONY: bootstrap-encoder prepare-encoder fetch-encoder train-encoder compare-encoder
bootstrap-encoder:
	uv run --locked saifety-lab bootstrap-context --lock encoder-v4.sources.lock.json --out bin/encoder --onnx
prepare-encoder:
	uv run --locked saifety-lab prepare-context --lock encoder-v4.sources.lock.json --source data/encoder-source --out data/encoder --tool bin/encoder/context-corpus
fetch-encoder:
	uv run --locked --extra ml saifety-lab fetch-encoder
train-encoder:
	uv run --locked --extra ml saifety-lab train-encoder --base artifacts/e5-base
compare-encoder:
	uv run --locked --extra ml saifety-lab compare-encoder

.PHONY: bootstrap-encoder-onnx
bootstrap-encoder-onnx:
	uv run --locked saifety-lab bootstrap-encoder-onnx

.PHONY: bootstrap-agent-harness prepare-agent-harness test-agent-harness
bootstrap-agent-harness:
	uv run --locked saifety-lab bootstrap-agent-harness
prepare-agent-harness:
	uv run --locked saifety-lab prepare-agent-harness
test-agent-harness:
	uv run --locked pytest -m harness
