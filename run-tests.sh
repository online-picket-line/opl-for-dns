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

# Use gotestsum for dot reporter (consistent with other sub-projects).
# Falls back to plain `go test` when gotestsum is not installed.
GOTESTSUM=""
if command -v gotestsum >/dev/null 2>&1; then
	GOTESTSUM="$(command -v gotestsum)"
elif [[ -x "${GOPATH:-$HOME/go}/bin/gotestsum" ]]; then
	GOTESTSUM="${GOPATH:-$HOME/go}/bin/gotestsum"
fi

if [[ -n "$GOTESTSUM" ]]; then
	"$GOTESTSUM" --format dots -- ./...
else
	"$GO_BIN" test -v ./...
fi
