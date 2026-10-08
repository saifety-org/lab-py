.PHONY: sync lint test build check bootstrap prepare train compare misses test-ml
sync:
	uv sync --locked
lint:
	uv run --locked ruff check .
	uv run --locked ruff format --check .
test:
	uv run --locked pytest -m 'not ml'
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
