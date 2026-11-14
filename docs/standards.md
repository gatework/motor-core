# Engineering standard adoption

Motor Core adopts [go-standards v1.0.2](https://github.com/gatework/go-standards/blob/v1.0.2/STANDARD.md).
The reference integration is the go-vxlan standards-release worktree, which uses
`include standard.mk` and appends repository checks with `CHECK_EXTRA`.

## Tooling and enforcement

`.golangci.yml` and `standard.mk` are exact copies distributed by the published
`github.com/gatework/go-standards/cmd/stdsync@v1.0.2`. Tools are version-pinned in
the shared makefile and do not become runtime dependencies in go.mod. The
standard and its copies must be upgraded together through stdsync; do not edit
the distributed configuration locally.

`make check` runs standard consistency, formatting, tidy verification, vet,
shared lint, architecture, shuffled race tests and pinned vulnerability checks.
The extra checks build the module, verify the canonical consumer fixture,
compare the API snapshot, and check added code language. `make ci` is an alias.
CI calls only this unified entrypoint and uses the patched toolchain in go.mod.

`architecture.yaml` lists every production package and enforces six dependency
rules. New packages require a declaration; test-only imports are not production
architecture. The loader, storage, logging, search, and runtime resource layers
cannot acquire dependencies on higher-level request or host wiring.

## Current conformance gaps

Adoption is an enforcing gate, not a declaration that existing code conforms.
The initial uncapped shared lint scan (`--max-issues-per-linter=0
--max-same-issues=0`) reports 1,142 findings, including 575 revive,
119 intrange, 107 staticcheck, 63 funcorder and 50 thelper findings. Formatting
also fails. No baseline suppression, disabled rule, or successful-exit wrapper is
installed. `make check` remains red until these findings are resolved.

MOD-1 specifies Go1.26 and a1.26/1.27 matrix. This module retains the explicitly
approved Go1.27.4 minimum: downgrading would contradict the current toolchain
and security decision. This is an unresolved standard-level discrepancy, not a
local waiver. PKG-1 package naming, LOG-1 slog-only logging, CMP-3 process globals,
and test conventions also need migration against the shared standard. Retained
multi-adapter logging is current product behavior; adoption does not silently
replace it. Reconcile these choices through shared-standard review or separately
reviewed module changes.

Address formatting and mechanical modernization separately from API/ownership
changes. Keep existing regression and allocation tests, update the API snapshot
when intended signatures change, and retain the Redis integration verification.
Do not infer remote CI success from local gate subsets.

Full shuffled tests also require explicit MySQL fixture handling: the existing
`tools/database.TestDBConfig_Init` attempts fixed local DSNs outside short mode.
It remains a TST-5 migration gap; this adoption does not suppress that test.
