# motor-core

Module: `github.com/gatework/motor-core` (no `/v2` suffix).
See [migration from go-admin-core](docs/migration/motor-core.md).

Forked from [go-admin-team/go-admin-core](https://github.com/go-admin-team/go-admin-core).

[![Go Version](https://img.shields.io/badge/Go-1.27.4+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

[English](README.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md) · [日本語](README.ja-JP.md)

## ✨ 主な機能

### 📝 Logger モジュール（エンタープライズ向けロギング）

- **マスキングロガー** - 機密情報（電話番号・パスワード・メールアドレスなど）を自動マスクし、コンプライアンス要件に対応
- **Logrus アダプター** - Logrus エコシステムに完全対応、50 種類以上の Hook が利用可能
- **本番向け設定** - ベストプラクティス（非同期 + サンプリング + マスキング）を同梱
- **並行安全** - Race Detector で検証済み、データ競合ゼロ

### 🔐 JWT 認証

- **Gin ミドルウェア** — Gin アプリケーションにログイン・更新・ID 解決を提供
- **有限なトークン寿命** — `MaxRefresh` がトークンを更新できる総時間を、最初の発行時点から起算して制限
- **設定可能** — `Timeout` と `MaxRefresh` はいずれも設定ファイルから制御

### 🚀 その他のコンポーネント

- [x] キャッシュ（memory / Redis）—— [docs/storage](docs/storage/README.md) を参照
- [x] キュー（memory / Redis Streams）—— [docs/storage](docs/storage/README.md) を参照
- [x] 設定管理（複数のデータソースに対応）
- [x] ログライター（ファイルローテーション対応）

> 変更のたびに、データ競合検出、2 プラットフォームでの staticcheck、
> およびリクエストごとに実行されるコードパスのメモリアロケーション予算を検証します。

---

## 🚀 クイックスタート

### インストール

```bash
go get -u github.com/gatework/motor-core
```

**動作要件:** Go 1.27.4 以降（`go.mod` の `go` ディレクティブを参照）

### 基本的なロギング

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

## 🧪 テスト

```bash
# CI ゲートが実行する内容すべて
make ci

# データ競合検出（全パッケージ）
make test-race

# ベンチマーク —— スモーク実行。比較可能な数値が必要なら -benchtime を上げる
make bench

# 継続的負荷・リソース回収・goroutine のライフサイクル
GOADMIN_SOAK=2m make soak

# ベースリビジョンと比較し、benchstat で判定
make bench-compare BASE=origin/main
```

**変更のたびに強制されるもの:**

- データ競合検出（全パッケージ）
- darwin と linux 両方での staticcheck
- `api/core.txt` の公開 API スナップショットがコードと一致すること
- リクエストごとに実行されるコードパスのアロケーション予算
- このツリーに対する下流利用者のビルド

---

## 📦 主な依存関係

```go
github.com/casbin/casbin/v3         v3.8.1
github.com/gin-gonic/gin            v1.11.0
github.com/golang-jwt/jwt/v5        v5.3.0
gorm.io/gorm                        v1.31.1
github.com/sirupsen/logrus          v1.9.4
golang.org/x/crypto                 v0.47.0
```

上記は執筆時点の `go.mod` の内容です。実際のバージョンは同ファイルを参照してください。

---

## 🤝 コントリビューション

Issue と Pull Request を歓迎します。

1. 本リポジトリを Fork
2. `main` から機能ブランチを作成 (`git checkout -b feature/AmazingFeature`)
3. 変更をコミット (`git commit -m 'Add some AmazingFeature'`)
4. ブランチへプッシュ (`git push origin feature/AmazingFeature`)
5. **`main` に向けて Pull Request を作成**

> **ブランチについて:** `main` が唯一のアクティブなブランチであり、すべての
> リリースの供給元です。`master` と `dev` は履歴用のブランチで、変更は
> 受け付けていません。`master` は 2022 年以降更新されておらず、リリース
> ラインに乗ったこともありません。

### 脆弱性の報告

セキュリティに関する問題は公開 issue を立てず、
[非公開の脆弱性報告](https://github.com/gatework/motor-core/security/advisories/new)
をご利用ください。

---

## 📝 License

Apache License 2.0 - 詳細は [LICENSE](LICENSE) を参照してください。

---

## 🔗 関連プロジェクト

- [go-admin](https://github.com/go-admin-team/go-admin) - Gin + Vue + Element UI によるフロントエンド・バックエンド分離型の権限管理システム
- [go-admin-ui](https://github.com/go-admin-team/go-admin-ui) - go-admin のフロントエンド

---

## ⭐ Star History

このプロジェクトがお役に立ちましたら、Star ⭐ をいただけると励みになります。
