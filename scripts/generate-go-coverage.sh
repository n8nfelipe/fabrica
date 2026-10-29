#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

(
	cd "$repo_root/backend"
	go test -coverprofile="$repo_root/coverage.out" ./...
	module_path="$(go list -m)"
	awk -v module="$module_path" 'NR == 1 { print; next } {
		prefix = module "/"
		if (index($0, prefix) == 1) $0 = "backend/" substr($0, length(prefix) + 1)
		print
	}' "$repo_root/coverage.out" > "$repo_root/coverage.normalized.out"
	mv "$repo_root/coverage.normalized.out" "$repo_root/coverage.out"
)
