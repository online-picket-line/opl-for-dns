#!/usr/bin/env bash
set -euo pipefail

GO_BIN=""
if command -v go >/dev/null 2>&1; then
	GO_BIN="$(command -v go)"
elif [[ -x /usr/bin/go ]]; then
	GO_BIN="/usr/bin/go"
elif [[ -x /usr/local/go/bin/go ]]; then
	GO_BIN="/usr/local/go/bin/go"
else
	for candidate in /usr/lib/go-*/bin/go; do
		if [[ -x "$candidate" ]]; then
			GO_BIN="$candidate"
			break
		fi
	done
fi

if [[ -z "$GO_BIN" ]]; then
	echo "go not found in PATH or common locations" >&2
	exit 127
fi

"$GO_BIN" test -v ./...
