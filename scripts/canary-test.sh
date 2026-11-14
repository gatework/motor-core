#!/usr/bin/env bash
# Exercise migration, input preservation, and rejection of an unrelated module.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT
mkdir -p "$workdir/consumer" "$workdir/unrelated"
cat > "$workdir/consumer/go.mod" <<'MOD'
module example.com/consumer
go 1.27.1
require github.com/go-admin-team/go-admin-core/v2 v2.11.0
MOD
cat > "$workdir/consumer/main.go" <<'GO'
package main
import (
    "github.com/go-admin-team/go-admin-core/v2/tools/search"
    "github.com/go-admin-team/go-admin-core/v2/sdk/api"
    "github.com/go-admin-team/go-admin-core/v2/sdk/runtime"
    "github.com/go-admin-team/go-admin-core/v2/logger/writer"
)
var _ = api.API{}
var _ = runtime.New
var _ = writer.New
func main() { _, _ = search.Resolve(search.MySQL, struct{}{}) }
GO
cat > "$workdir/consumer/go.work" <<'WORK'
go 1.27.1
use .
WORK
cp -R "$workdir/consumer" "$workdir/before"
"$ROOT/scripts/canary.sh" "$workdir/consumer"
diff -r "$workdir/before" "$workdir/consumer"
cat > "$workdir/unrelated/go.mod" <<'MOD'
module example.com/unrelated
go 1.27.1
MOD
printf 'package main\nfunc main() {}\n' > "$workdir/unrelated/main.go"
if "$ROOT/scripts/canary.sh" "$workdir/unrelated" > "$workdir/unrelated.log" 2>&1; then
	echo 'canary accepted a consumer that never imports Motor Core' >&2
	exit 1
fi
grep -q 'must use github.com/gatework/motor-core' "$workdir/unrelated.log"
echo 'PASS canary input preservation and dependency guard'
