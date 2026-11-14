# Runtime and resource contracts

This document describes Motor Core's current API. The exported signatures are
recorded in `api/core.txt`; breaking changes are listed in
[migrating to Motor Core](migration/motor-core.md).

## Application resources

`runtime.New()` returns `*runtime.Application`. `sdk.Runtime` is a default
instance for package-level host wiring; a host can instead pass an application
explicitly. There is no parallel runtime interface or legacy adapter.

Database, application, enforcer, handler, middleware, and configuration
registries are protected by the application mutex. Setters identify the tenant
explicitly, for example `SetDB(tenant, db)`, and getters use names such as
`DB(tenant)` and `Databases()`. A wildcard tenant entry takes precedence for
resource lookups that support it.

Returned maps and slices are container snapshots. Objects stored inside them
retain their identity; copying a registry does not clone a database connection,
logger, or arbitrary application object. Configuration setters copy input maps.
Resource setters publish references and do not close the previous resources.
The host must coordinate replacement with active users and cleanup.

Methods take the application mutex once. Callback invocation and logging happen
outside it, so callbacks and custom loggers may call application methods.

## Registration and phases

`SetBefore` and `SetAppRouters` register callbacks in order. `RunBefore` and
`RunAppRouters` seal their registries and dispatch each callback once. Repeated
runs do not repeat callbacks; late registration is rejected and logged.
`BeforeSealed` and `AppRoutersSealed` expose that state. Hosts must call these
runners at the intended startup points.

`SetPhase` and `RunPhase` provide four lifecycle points:

| Phase | Execution |
| --- | --- |
| `AfterResource` | Repeats after resources are applied |
| `BeforeRouter` | Once, before router initialization |
| `AfterListen` | Once, after the host confirms that the listener is bound |
| `BeforeExit` | Once, in reverse registration order |

The host announces phases; a phase constant does not detect listener readiness
or order unrelated host code. Phase numbers are implementation details.

`AfterResource` rounds do not overlap. A trigger during a round requests another
round and returns without waiting for it. A callback added during a round is
available to a later round. Reload callbacks must tolerate repeated execution
and must not continually register more callbacks.

## Panic handling

A callback panic is logged with its registration site and stack; later callbacks
continue. `WithName` supplies a label. `WithFatal` turns a startup callback panic
into a failure that propagates unchanged through framework guards; later startup
callbacks do not run. Normal panic unwinding runs deferred cleanup. The framework
does not call `os.Exit`: the host owns recovery and its process exit code.
`WithFatal` is ignored for shutdown callbacks so remaining cleanup can run. The
guard cannot recover a panic in a goroutine started by the callback.

Middleware retrieval through `GetHandlerFunc` returns `(gin.HandlerFunc, bool)`.
A host must reject a missing required authentication or authorization handler.

## Shutdown and ownership

Call `BeginShutdown` before stopping services. It prevents new non-exit phases
from being registered or triggered, but does not interrupt an active callback.
Stop configuration watchers before disposing of resources they can replace.

`SetShutdown` and `SetPhase(BeforeExit, ...)` share one registry. `RunShutdown(ctx)`
runs it once in reverse registration order with the supplied budget. A deadline
ends the wait and returns `ctx.Err()`; Go cannot forcibly terminate a callback,
so an uncooperative callback can continue after the method returns.

The host owns resource cleanup: setting a cache, queue, database, or logger does
not register its shutdown automatically. Use `storage.Cache.Close()` and
`storage.Queue.Shutdown(ctx)` on the owned roots. Subscribe before starting a
queue; `Run(ctx)` blocks until cancellation or failure and initiates shutdown.
Memory queues drain accepted messages within the shutdown budget. Retry waits
observe shutdown. Backend persistence and delivery guarantees are documented by
the backend, not implied by the common interface.

`QueueForTenant` returns a view over the backing queue and clones the message's
values map before adding `storage.PrefixKey`. It shares the backing queue and its
lifecycle; creating a view does not create or own another worker.

Logger `With` views borrow their root's resources. Keep the concrete root for
`Close` and `Sync`. Process logger publication through `logger.SetDefault` is
synchronized and does not close either logger. See
[structured logging](../logger/README_ADVANCED.md) for field and output ownership.
