Follow [go-standards/STANDARD.md v1.0.2](https://github.com/gatework/go-standards/blob/v1.0.2/STANDARD.md); run `make check` before delivery.

# Motor Core

Read [CONTRIBUTING.md](CONTRIBUTING.md) for repository workflow and
[standards adoption](docs/standards.md) for known migration gaps.

- `architecture.yaml` enforces production import boundaries; declare new packages explicitly.
- Preserve resource ownership and shutdown contracts in `docs/contract.md` and `docs/storage/README.md`.
- Public API changes update `api/core.txt` and `docs/migration/motor-core.md`; no transition aliases.
- Keep go-admin consumer migration separate. Publish no commits or versions without authorization.
- Shared `.golangci.yml` and `standard.mk` are exact distributed copies; update them through pinned stdsync.
