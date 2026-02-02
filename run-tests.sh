#!/usr/bin/env bash
set -euo pipefail

if ! command -v go >/dev/null 2>&1; then
	if [[ -x /usr/bin/go ]]; then
		export PATH="/usr/bin:${PATH}"
	else
		echo "go not found in PATH" >&2
		exit 127
	fi
fi

go test -v ./...
