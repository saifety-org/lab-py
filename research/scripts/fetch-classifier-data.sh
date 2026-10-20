#!/bin/sh
# Fetch public classifier data using Go; commands in samples are never executed.
set -eu
cd "$(dirname "$0")/.."
exec go run ./cmd/fetch-data "$@"
