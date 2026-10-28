#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_ROOT=$(dirname "$SCRIPT_DIR")

if [ ! -f "$PROJECT_ROOT/.env" ]; then
  echo "Missing $PROJECT_ROOT/.env. Copy .env.example and set a local password first." >&2
  exit 1
fi

set -a
. "$PROJECT_ROOT/.env"
set +a

cd "$PROJECT_ROOT/backend"
exec go run -buildvcs=false ./cmd/fabrica
