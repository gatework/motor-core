# Migrating to Motor Core

The module path is `github.com/gatework/motor-core`, without `/v2`.
This is a new module identity, derived from upstream go-admin-core v2.
The Go requirement remains 1.27.4. Future tags for this path must use the
v0 or v1 version series; upstream v2 tags describe the old module.

The repository starts with a new root commit containing the refactored library;
upstream Git history is not included. Upstream attribution and license remain.
Consumers such as go-admin must migrate separately. No release tag is created
by the initial repository publication.

## Import migration

Build the migration tool from this checkout:

```sh
go build -o /tmp/motor-coreupgrade ./tools/coreupgrade
/tmp/motor-coreupgrade -motor /path/to/consumer
/tmp/motor-coreupgrade -motor -w /path/to/consumer
```

The first invocation previews changes. The second rewrites Go imports of
`github.com/go-admin-team/go-admin-core` and its `/v2` module to Motor Core.
It also handles the older promoted packages, retains import aliases, ignores
symlink files, and leaves comments and ordinary string literals alone. Running
it twice makes no additional changes. `-v2` remains available for historical
upstream migrations and cannot be combined with `-motor`.

After a Motor Core version is published, consumers must explicitly replace their
old requirement with that version and run `go mod tidy`. Do not put the old v2
version on the new, unversioned module path. Mixing both module identities in
one build creates distinct Go types and separate package globals.

## Compatibility verification

```sh
scripts/canary.sh /path/to/consumer
```

The canary copies each consumer into a temporary directory, migrates the copy,
and builds it in a temporary workspace containing this checkout. The original
source, module files, and workspace files remain unchanged. A successful build
must actually depend on Motor Core and contain no old core packages. This
verifies compilation, not production startup, database behavior, or deployment.
Consumers still using the old identifiers above are expected to fail this check
until their separate migration is complete. CI runs `make canary-test`, a
canonical API fixture and an input-preservation check. It no longer treats the
unmigrated upstream go-admin default branch as a compatible consumer.

Relative local replacements in a consumer must also resolve in the temporary
copy; dependency-resolution failures are reported as failures.

## API and ownership changes

The names below are final APIs: no type aliases, deprecated forwarding methods, or
transitional spellings are provided. Consumers must update their source in a
separate migration. The import tool only changes import paths, not identifiers.

| Previous spelling | Motor Core spelling |
| --- | --- |
| `api.Api`, chained `MakeContext` / `MakeOrm` setup | `api.New(context, api.Dependencies{...})` returning `(*api.API, error)` |
| `api.API.Orm`, `service.Service.Orm` | `DB`; pass dependencies explicitly and use `API.Service()` |
| `runtime.NewConfig`, `runtime.Runtime` interface | `runtime.New()` returning `*runtime.Application` |
| Runtime `SetDb`, `SetDbByTenant` | `SetDB(tenant, db)` |
| Runtime `GetDb`, `GetDbByTenant`, `GetAllDb` | `DB(tenant)`, `Databases()` |
| Runtime resource getters | `App`, `Apps`, `Casbin`, `Enforcers`, `Engine`, `Logger`, `Cache`, `Queue`, `Config`, `ConfigValue` |
| `config.NewConfig`, package-level configuration access | `config.New()` returning `(*config.Config, error)`; call methods on that instance |
| `user.GetUserId`, `GetUserIdStr`, `GetUserName` | `UserID`, `UserIDString`, `UserName` |
| `user.GetRoleId`, `GetRoleName` | `RoleID`, `RoleName` |
| `user.GetDeptId`, `GetDeptName` | `DepartmentID`, `DepartmentName` |
| `writer.FileWriter`, `writer.NewFileWriter` | `writer.File`, `writer.New` |
| Legacy cache/queue adapters | `storage.Cache`, `storage.Queue`; construct `cache.New(options)` and `queue.New(size)` |
| `search.ResolveSearchQuery` | `search.Resolve(driver, query)` returning `(Condition, error)` |

Logging now has one structured contract: `Log(context.Context, Level, string,
...Field)`, `With(...Field) Logger`, and `Enabled(Level) bool`. Use `NewHelper`
for formatted convenience methods. Adapter constructors return concrete roots
and errors; keep the root to close its resources. `With` returns a borrowed field
view. `Default()` / `SetDefault()` synchronize process logger publication without
transferring ownership. See [structured logging](../../logger/README_ADVANCED.md).

The legacy storage implementations and SDK logger shim are removed. Queue
handlers receive a context and `storage.Message`; use `Publish`, `Subscribe`,
`Run`, and `Shutdown`. Cache operations use context and explicit errors.
Search resolution now validates tags and identifiers and reports invalid input;
callers must propagate its error before applying the returned condition.

The following ownership and lifecycle corrections accompany the rename:
- Runtime app, exclusion, handler, and configuration accessors return container
  snapshots. Configuration setters copy the supplied map. Values inside maps
  still refer to the same resources; this is not a deep copy of arbitrary objects.
- Memory configuration sources and loader snapshots own their byte slices.
  Stopping a source watcher unblocks `Next`. Closing a config closes its default
  loader, but a loader supplied through `WithLoader` remains caller-owned.
  Close still does not wait for an application `OnChange` callback already running.
- Queue `Run` returns after shutdown; startup and shutdown no longer use a
  WaitGroup as a stop signal. Retry backoff observes the stop signal.
- The service manager cancels peers on failure and waits within its shutdown
  budget. Asynchronous runnables may implement `Shutdown(context.Context) error`.
  Listener startup remains asynchronous, and cancellation closes its HTTP server.
- `logger/writer.File` now implements `io.WriteCloser`. Writes consume
  their input before returning and report I/O failures; callers must close owned
  writers. File writing is synchronous. Use the logger's async adapter when
  asynchronous logging is required.
- User claim extraction delegates to the JWT middleware's checked extractor.
  Foreign values under the JWT context key no longer panic. Search DTO traversal
  skips scalar fields and accepts pointer DTOs without losing tagged filters.

The API snapshot in `api/core.txt` is regenerated from the module itself.
The protobuf source and descriptor both carry the new module identity. Regenerate
with protoc 36.2 and protoc-gen-go v1.36.11:

```sh
protoc -I errors --go_out=paths=source_relative:errors errors/errors.proto
```

Historical release notes, migration fixtures, and upstream issue links retain
old module names deliberately. They are not active import paths.
