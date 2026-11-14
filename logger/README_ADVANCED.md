# Structured logging

The package supports a text writer, Zap, and Logrus. All adapters and decorators
use the same interface:

```go
type Logger interface {
    Log(ctx context.Context, level Level, message string, fields ...Field)
    With(fields ...Field) Logger
    Enabled(level Level) bool
}
```

Pass the request context to `Log`. A logger stores fields, not a request context.
Known `trace_id`, `user_id`, and `request_id` context values become entry fields.
Explicit fields override prebound fields with the same key.

## Adapters

```go
log, err := logger.NewLogrusLogger(
    logger.WithLevel(logger.InfoLevel),
    logger.WithPath("logs/app.log"),
    logger.WithEncoder("json"),
)
if err != nil {
    return err
}
defer log.Close()

requestLog := log.With(logger.RequestID("req-123"))
requestLog.Log(ctx, logger.InfoLevel, "request completed",
    logger.StatusCode(200),
    logger.Latency(elapsed),
)
```

`NewLogrusLogger` returns `(*Logrus, error)` and `NewZapLogger` returns
`(*Zap, error)`. File opening and invalid decorator settings are reported during
construction. Logrus-specific operations, such as `AddHook`, are available on
the concrete root.

`NewLogger` returns `*Text` and writes the compact text format. Use `WithOutput`
for a supplied writer; use the Logrus or Zap adapter for an owned rotating file.
`NewFromConfig` selects a registered adapter and returns `(Logger, error)`.
Network outputs, metrics plugins, and tracing plugins that are not implemented
are rejected by configuration validation.

## Resource ownership

Keep the root returned by the constructor for `Close` and `Sync`. A value returned
by `With` implements only `Logger`: it shares the root's resources and cannot
close them. `With` does not start another worker.

Writers supplied through `WithOutput` remain caller-owned. The logger can call
an optional `Sync` method but never closes these writers. File handles created
for `WithPath` are owned and closed by the adapter.

A decorator owns the logger passed to its constructor. Closing the outer root
closes its wrapped logger after completing its own work. Built-in roots support
repeated `Close` calls. Calls made after their root closes are ignored.

## Decorators

Use typed options when constructing an adapter:

```go
log, err := logger.NewLogrusLogger(
    logger.WithPath("logs/app.log"),
    logger.WithEncoder("json"),
    logger.WithSanitizer(logger.DefaultSanitizerConfig),
    logger.WithSampling(logger.DefaultSamplingConfig),
    logger.WithAsync(logger.AsyncConfig{
        BufferSize:    1024,
        FlushInterval: 100 * time.Millisecond,
        DropPolicy:    "block",
    }),
)
```

The adapter owns this entire pipeline. Typed options replace string keys stored
in a context. The same decorators can be composed explicitly using
`NewSanitizerLogger`, `NewSamplingLogger`, and `NewAsyncLogger`.

- Sanitization handles prebound, per-entry, helper, and known context fields.
  Rules match field names; arbitrary text in a message is not matched against
  field rules. `SetMasker` applies a process-wide text masker to messages and
  string fields at the output boundary.
- Sampling shares one budget across field views. Error and Fatal entries always
  bypass sampling, including calls through `Helper`.
- Async uses one bounded worker. `drop` discards entries when full, `block` waits
  while observing cancellation and shutdown, and `sample` retries admission for
  one in ten full-queue attempts. Error and Fatal entries are written
  synchronously. `Sync` waits for previously accepted entries; `Close` stops
  admission, drains accepted entries, and closes the wrapped root.

Field slices and configuration containers are copied. Values stored in
`Field.Any` are borrowed: keep referenced maps, slices, and objects immutable
until asynchronous processing completes. Dropped-entry callbacks run on the
publishing goroutine and must support concurrent calls if publishers do.

## Helpers and the process default

`NewHelper(log)` provides `Info`, `Infof`, `Infow`, and the corresponding methods
for other levels. These conveniences render messages before entering the same
`Log` method. `WithFields(map[string]any)` and `WithError` return helper views;
`FieldsFromMap` converts a map to typed fields for the core interface.

`Default()` and `SetDefault(log)` synchronize access to the process default.
Replacing it does not close the previous logger. Its owner must coordinate
in-flight users before closing it. `SetupLogger(*Config)` constructs and publishes
a root, returning its logger and any construction error; a failure leaves the
existing default unchanged.

## Verification

```sh
go test -short -race ./logger/...
go test -run '^$' -bench 'Logger|Async|Sampling|Sanitizer' -benchmem ./logger
```

Benchmark results depend on the adapter, output, decorator settings, and machine.
Compare repeated runs using the same workload; asynchronous enqueue latency and
end-to-end output throughput measure different work.
