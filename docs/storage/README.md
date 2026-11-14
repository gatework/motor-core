# Storage contracts

`storage.Cache` and `storage.Queue` are the public contracts. Implementations
share cancellation, error precedence, and ownership tests in `storage/cachetest`.
No legacy adapter or compatibility interface is provided.

## Cache

Construct an in-process cache with `cache.New(cache.Options{...})`. Operations
accept context first: `Get`, `Set`, `Del`, `Incr`, and `Expire`. A missing or
expired entry returns `storage.ErrCacheMiss`; a closed instance returns
`storage.ErrCacheClosed`. An already cancelled context takes precedence.

`Set` with a non-positive TTL does not expire. `Incr` is atomic and preserves
an existing entry's expiry. Close owned caches when no request borrows them.
The memory implementation shards its keyspace and bounds capacity per shard;
its options document capacity rounding and lazy/background expiry.

Redis implementations use `redis.NewCache` with a supplied client or
`redis.OpenCache` with an owned client. NewCache borrows the supplied client; OpenCache owns the client it creates.
Closing a borrowed client belongs to the host. `storage.WithPrefix` decorates
keys without creating a separate backend.

## Queue

Construct memory queues with `queue.New(size)` and Redis queues with
`redis.NewQueue` or `redis.OpenQueue`. Subscribe all topics before `Run`:

```go
err := q.Subscribe(ctx, "orders", func(ctx context.Context, msg storage.Message) error {
    return handle(ctx, msg)
})
if err != nil {
    return err
}
return q.Run(ctx)
```

`Run` blocks until cancellation or failure and initiates shutdown. A context
already cancelled before startup is rejected; cancellation during a running
queue is a clean exit. The owner calls `Shutdown(ctx)` to stop admissions and
wait within its budget. Repeated shutdown calls can wait for unfinished work.
A tenant queue view shares the backing queue's lifetime.

Publish requires a subscribed topic. Duplicate and late subscriptions return
explicit errors. Handlers must tolerate repeated deliveries. Memory queues
retry failed handlers up to four attempts and drain accepted messages on
shutdown; they are not durable. Redis streams persist messages, acknowledge
successful processing, and retain exhausted messages pending for inspection.
Do not treat acknowledgment as stream deletion or an automatic retention policy.

Redis retry scans advance past exhausted entries so later pending messages are
not starved. `ClaimMinIdle` must exceed the normal maximum handler duration to
avoid concurrent redelivery. `Batch` bounds each scan. Redis processing remains
serial in the consumer loop; this interface does not promise isolation between
slow handlers on different Redis topics.

Message values cross Redis as JSON. Values must be JSON encodable; use strings
when numeric precision or an exact application type matters. The caller owns
nested mutable values and must not modify them during publication or delivery.

Run conformance tests with a disposable Redis database:

```sh
REDIS_URL=redis://127.0.0.1:6379/0 go test -short -race ./storage/redis
```
