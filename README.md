# motor-core

Module: `github.com/gatework/motor-core` (no `/v2` suffix).
See [migration from go-admin-core](docs/migration/motor-core.md).

Forked from [go-admin-team/go-admin-core](https://github.com/go-admin-team/go-admin-core).

[![Go Version](https://img.shields.io/badge/Go-1.27.4+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

[English](README.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md) · [日本語](README.ja-JP.md)

## ✨ Core Features

### 📝 Logger Module (Enterprise-Grade Logging Solution)

- **Sanitizer Logger** - Auto-sanitize sensitive data (phone, password, email, etc.), compliance-ready
- **Logrus Adapter** - Full Logrus ecosystem support with 50+ hooks available
- **Production-Ready Config** - Built-in best practices (async + sampling + sanitization)
- **Concurrency Safe** - Verified with Race Detector, zero data races

### 🔐 JWT Authentication

- **Gin middleware** - Login, refresh and identity handling for Gin applications
- **Bounded token lifetime** - `MaxRefresh` caps how long a token may be renewed
  for, measured from its original issuance
- **Configurable** - `Timeout` and `MaxRefresh` are both driven by configuration

### 🚀 Other Components

- [x] Cache component (memory and Redis) — see [docs/storage](docs/storage/README.md)
- [x] Queue component (memory and Redis streams) — see [docs/storage](docs/storage/README.md)
- [x] Configuration management (multiple data sources)
- [x] Log writer (file rotation support)

> Every change runs the race detector, the shared lint rules, and
> allocation budgets on the paths that execute per request.

---

## 🚀 Quick Start

### Installation

```bash
go get -u github.com/gatework/motor-core
```

**System Requirements:** Go 1.27.4 or higher (see the `go` directive in `go.mod`)

### Basic Logging

```go
package main

import (
    "context"
    "log"

    "github.com/gatework/motor-core/logger"
)

func main() {
    root, err := logger.NewLogrusLogger(
        logger.WithPath("logs/app.log"),
        logger.WithLevel(logger.InfoLevel),
        logger.WithEncoder("json"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer root.Close()
    root.With(logger.UserID(12345)).Log(context.Background(), logger.InfoLevel, "started")
}
```

See [structured logging](logger/README_ADVANCED.md) for sampling, redaction,
async delivery, and resource ownership. Keep the concrete root for Close/Sync;
field views borrow it. Configuration watchers publish settings only: hosts
explicitly create, inject, and retire application resources.

## Configuration

```go
manager, err := config.New(config.WithSource(file.NewSource(file.WithPath("config.json"))))
if err != nil {
    return err
}
defer manager.Close()
```

The example uses `config` and `config/source/file` from this module. See
[configuration](config/README.md) and [migration](docs/migration/motor-core.md).

## Performance measurement

Run `make bench` for a smoke check. Use `BenchmarkDelivery` to compare async
admission and actual sink delivery: it reports written/s and dropped/op and
includes draining accepted entries. Sampling and dropping are not throughput
gains. Compare on the same machine and toolchain, including p99 under mixed load.

## Engineering standards

Follow [go-standards v1.0.2](https://github.com/gatework/go-standards/blob/v1.0.2/STANDARD.md).
Run `make check`; see [adoption status](docs/standards.md) for the remaining migration gaps.
See [GitHub releases](docs/RELEASING.md) for exact-artifact publication.

## 🧪 Testing

```bash
# Everything the CI gate runs
make ci

# Race detector, across every package
make test-race

# Benchmarks — a smoke run; raise -benchtime for numbers worth comparing
make bench

# Sustained load, resource reclamation and goroutine lifecycles
GOADMIN_SOAK=2m make soak

# Measure this tree against a base revision, through benchstat
make bench-compare BASE=origin/main
```

**Enforced on every change:**

- Race detector, every package
- Shared lint, including staticcheck
- The exported API snapshot in `api/core.txt` must match the code
- Allocation budgets on the paths that run per request
- Downstream consumers are built against this tree

---

## 📦 Main Dependencies

```go
github.com/casbin/casbin/v3         v3.8.1
github.com/gin-gonic/gin            v1.11.0
github.com/golang-jwt/jwt/v5        v5.3.0
gorm.io/gorm                        v1.31.1
github.com/sirupsen/logrus          v1.9.4
golang.org/x/crypto                 v0.47.0
```

Versions above reflect `go.mod` at the time of writing; that file is the source
of truth.

---

## 🤝 Contributing

We welcome Issues and Pull Requests!

1. Fork this repository
2. Create your feature branch from `main` (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request **against `main`**

> **Branches:** `main` is the only active branch and the source of every release.
> `master` and `dev` are historical and no longer accept changes — `master` in
> particular has not moved since 2022 and was never part of the release lineage.

### Reporting a vulnerability

Please do not open a public issue for security problems. Use
[private vulnerability reporting](https://github.com/gatework/motor-core/security/advisories/new)
instead.

---

## 📝 License

Apache License 2.0 - See [LICENSE](LICENSE) file for details

---

## 🔗 Related Projects

- [go-admin](https://github.com/go-admin-team/go-admin) - Gin + Vue + Element UI based RBAC system with separated frontend/backend
- [go-admin-ui](https://github.com/go-admin-team/go-admin-ui) - go-admin frontend project

---

## ⭐ Star History

If this project helps you, please give us a Star ⭐
