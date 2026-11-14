# Package map

Motor Core is the independent module `github.com/gatework/motor-core`. The Git
history starts at its own root commit; historical upstream reports are context,
not current API documentation. See `docs/contract.md`, `docs/storage/README.md`,
and `docs/migration/motor-core.md` for current contracts.

| Package | Responsibility |
| --- | --- |
| `captcha` | CAPTCHA generation and bounded verification storage |
| `casbin` | Enforcer integration |
| `config` | Dynamic sources, loaders, readers and value snapshots |
| `jwtauth` | JWT middleware and typed user claim access |
| `logger` | Structured adapters, redaction, sampling and bounded async delivery |
| `response` | HTTP response envelopes |
| `sdk/runtime` | Explicit application resource and callback registries |
| `sdk/config` | Validated settings snapshots and reload notifications |
| `sdk/api`, `sdk/service` | Request-bound dependencies and binding |
| `sdk/pkg/ws` | Owned WebSocket registration loop and connection workers |
| `server` | Service lifecycle and HTTP listeners |
| `storage` | Cache/queue contracts and memory/Redis implementations |
| `tools/search` | Validated search DTO resolution |
| `tools/coreupgrade` | Import-path migration tool |

The two config packages have distinct responsibilities. Settings publication
never owns application resource lifetimes. The host creates and closes database,
cache, queue and logger roots after their borrowers finish.

`make ci` checks module files, build, vet, staticcheck, race tests and the exported
API snapshot. `make canary-test` checks import migration and an isolated canonical
API fixture; it does not prove an unmigrated go-admin consumer works.

`make bench` is a smoke check. Performance comparisons must use the same machine
and toolchain, include successful delivery counts and p99 where relevant, and
propagate benchmark failures. Sampling or dropping entries is not sink throughput.
The module has allocation budgets; shared CI runner timing is not an acceptance
threshold.

`make api-check` regenerates `api/core.txt` and compares it with the index. Review
and stage intentional snapshot changes before running this gate. Tests requiring
MySQL are skipped by short mode; Redis conformance needs `REDIS_URL`.
