#!/usr/bin/env bash
# Build disposable consumer copies against this tree. The supplied repositories,
# including their go.mod/go.sum/go.work files, are never modified.
set -euo pipefail

CORE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CORE_MOD="$(awk '/^module /{print $2; exit}' "$CORE_DIR/go.mod")"
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

repos=("$@")
if [ ${#repos[@]} -eq 0 ] && [ -n "${CANARY_REPOS:-}" ]; then
	IFS=':' read -r -a repos <<<"$CANARY_REPOS"
fi
if [ ${#repos[@]} -eq 0 ]; then
	echo "usage: scripts/canary.sh /path/to/consumer [...]" >&2
	exit 2
fi

(cd "$CORE_DIR" && GOWORK=off go build -o "$workdir/coreupgrade" ./tools/coreupgrade)
failed=0
index=0
for repo in "${repos[@]}"; do
	index=$((index + 1))
	name=$(basename "$repo")
	if [ ! -f "$repo/go.mod" ]; then
		echo "FAIL $name: no go.mod at $repo" >&2
		failed=1
		continue
	fi
	consumer="$workdir/consumer-$index"
	mkdir -p "$consumer"
	if ! (cd "$repo" && tar --exclude=.git --exclude=vendor --exclude=node_modules \
		--exclude=go.work --exclude=go.work.sum -cf - .) | (cd "$consumer" && tar -xf -); then
		echo "FAIL $name: could not copy consumer" >&2
		failed=1
		continue
	fi
	if ! "$workdir/coreupgrade" -w -motor "$consumer" >"$workdir/migration.log" 2>&1; then
		echo "FAIL $name: migration failed" >&2
		cat "$workdir/migration.log" >&2
		failed=1
		continue
	fi
	if ! (cd "$consumer" && GOWORK=off go work init . "$CORE_DIR"); then
		echo "FAIL $name: could not initialize workspace" >&2
		failed=1
		continue
	fi
	if ! deps=$(cd "$consumer" && GOWORK="$consumer/go.work" go list -deps ./...); then
		echo "FAIL $name: dependency resolution failed" >&2
		failed=1
		continue
	fi
	if ! printf '%s\n' "$deps" | awk -v m="$CORE_MOD" '
		$0 == m || index($0, m "/") == 1 { found = 1 }
		$0 == "github.com/go-admin-team/go-admin-core" ||
		index($0, "github.com/go-admin-team/go-admin-core/") == 1 { legacy = 1 }
		END { exit !(found && !legacy) }'; then
		echo "FAIL $name: must use $CORE_MOD without legacy core packages" >&2
		failed=1
		continue
	fi
	if (cd "$consumer" && GOWORK="$consumer/go.work" go build ./...); then
		echo "PASS $name"
	else
		echo "FAIL $name: build failed" >&2
		failed=1
	fi
done
exit "$failed"
