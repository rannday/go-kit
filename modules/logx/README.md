# logx

Structured logging built on Go's `log/slog`.
Zero dependencies. Module path `github.com/rannday/go-kit/modules/logx`; package name `logx`.

- Text or JSON output
- Stack traces by level
- Runtime level changes
- Redaction support
- HTTP server middleware
- HTTP client transport
- `Configure` installs package-global logger
- `New` builds logger without touching globals
- `WrapHandler` composes stack-trace and redaction decorators without globals
- File rotation is size-based only
- Color output is text-console only

## Requirements

- Go 1.21 or newer, as declared in `go.mod`

## Install

```bash
go get github.com/rannday/go-kit/modules/logx
```

## Quick Start

```go
package main

import (
	"log/slog"

	logx "github.com/rannday/go-kit/modules/logx"
)

func main() {
	if err := logx.Configure(logx.Config{
		Level:           slog.LevelInfo,
		Console:         true,
		AddSource:       false,
		StacktraceLevel: slog.LevelError,
	}); err != nil {
		panic(err)
	}
	defer logx.Reset()

	logx.Info("starting", "version", "1.0.0")
	logx.Warn("cache miss", "cache_key", "user:42")
	logx.Error("operation failed", "id", 123)
}
```

## Configuration

```go
if err := logx.Configure(logx.Config{
	Level:           slog.LevelDebug,
	Console:         true,
	FilePath:        "app.log",
	JSONFile:        false,
	AddSource:       true,
	StacktraceLevel: slog.LevelError,
}); err != nil {
	panic(err)
}
```

If you want stack traces at info level, set `StacktraceEnabled: true` and `StacktraceLevel: slog.LevelInfo`.
With both stack options at zero, stack traces are disabled. Console output goes to stderr;
set `ConsoleJSON: true` for JSON console records. `JSONFile` controls file output independently.

## Build Without Installing

Use `New` when you want a configured logger without touching global state.

```go
logger, closer, err := logx.New(logx.Config{
	Level:      slog.LevelInfo,
	Console:    true,
	FilePath:   "app.log",
	JSONFile:   false,
	AddSource:  true,
})
if err != nil {
	panic(err)
}
defer func() {
	if closer != nil {
		_ = closer.Close()
	}
}()

logger.Info("ready")
```

The caller owns the closer returned by `New`; it is nil for console-only output.
Closing it disables that logger and all its `With`/`WithGroup` children, including console output.
`FileWriter` takes precedence over `FilePath` and transfers closing responsibility to logx.
Use a separate writer for each logger configuration.

## Bootstrap Then Configure

Use `Configure` for early console logging, then call `Configure` again after app config or env is loaded.

```go
// bootstrap: console only
if err := logx.Configure(logx.Config{
	Level:   slog.LevelInfo,
	Console: true,
}); err != nil {
	panic(err)
}

// ...load env/config...

// configure again: attach file output after config is known
if err := logx.Configure(logx.Config{
	Level:           slog.LevelDebug,
	Console:         true,
	FilePath:        "app.log",
	JSONFile:        false,
	AddSource:       true,
	StacktraceLevel: slog.LevelError,
}); err != nil {
	panic(err)
}
```

Calling `Configure` again is the supported way to attach file logging after startup.
If file setup fails, `Configure` returns an error and leaves the previous global logger alone.
`Console: false` with neither `FilePath` nor `FileWriter` is an error. There is no silent stderr fallback.
Successful reconfiguration installs `slog.Default()` and closes the previous file sink.
Cached loggers attached to that closed sink stop emitting; retrieve the new logger after reconfiguration.
`Reset` closes the global sink, resets redaction defaults, and makes `slog.Default()` discard records.
The next `logx.Logger()` call lazily creates an info-level stderr logger.

## Runtime Level Changes

```go
logx.SetLevel(slog.LevelDebug)
```

`SetLevel` changes the level used by loggers built with `Configure` or `Logger`.
It does not change independent loggers returned by `New`.
`SetLogger` clears that level variable. A later `SetLevel` does nothing until the next `Configure` or `Logger` initialization, and it does not change the handler you passed to `SetLogger`.

## Structured Logging

```go
logx.Info("user login",
	"user", "admin",
	"ip", "10.0.0.5",
)
```

## Error Helpers

```go
err := doSomething()
logx.ErrorErr("operation failed", err, "job_id", "job-42")
```

Context version:

```go
logx.ErrorErrContext(ctx, "operation failed", err)
```

## Custom Structured Errors

```go
type APIError struct {
	Status int
	Code   string
}

func (e APIError) Error() string { return e.Code }

func (e APIError) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.Int("status", e.Status),
		slog.String("code", e.Code),
	}
}
```

Usage:

```go
logx.ErrorErr("api failure", APIError{Status: 503, Code: "unavailable"})
```

Error helpers add `error` and `error_type` for non-nil errors and find `LogAttrs` through wrapped errors.

## Context Helpers

```go
ctx := logx.WithRequestID(ctx, "abc123")
id, ok := logx.RequestID(ctx)
```

Context-aware helpers (`InfoContext`, `ErrorErrContext`, `Timed`, and friends) use
`LoggerFromContext`, so request-scoped loggers from `httpx` middleware or
`logx.WithLogger` are picked up automatically.
Request IDs are separate context values; bind them with `logger.With("request_id", id)`
or pass the attribute explicitly for application logs.

## Timing Helpers

```go
done := logx.Timed(ctx, "process job", "job_id", "job-42")
defer done()
```

Custom level:

```go
done := logx.TimedLevel(
	logx.Logger(),
	slog.LevelDebug,
	ctx,
	"process job",
	"job_id", "job-42",
)
defer done()
```

## Color Output

- Color is only applied to text console output
- Disabled for JSON console output
- Disabled for file output
- Disabled when piped
- Disabled if `NO_COLOR` is nonempty

## Fatal

```go
logx.Fatal("unrecoverable error")
```

Logs at error level (there is no separate fatal level) and exits with status code `1`.
Deferred functions do not run after `Fatal`.

## Testing

```bash
go test ./...
go test -race ./...
go build ./...
go vet ./...
```

CI verifies module tidiness and runs pinned Staticcheck with the toolchain selected from `go.mod`.
Maintenance guidance lives in [../../docs/logx.md](../../docs/logx.md) and [../../.agents/skills/logx/SKILL.md](../../.agents/skills/logx/SKILL.md).

## Middleware

HTTP utilities live in the `httpx` subpackage.

```go
import "github.com/rannday/go-kit/modules/logx/httpx"
```

### Server Middleware

```go
handler := httpx.HTTPMiddleware(router)
```

`HTTPMiddleware` adds timing, status logging, panic recovery, and request ID propagation.
It stores the request ID in context and mirrors it to the `X-Request-ID` response header.
ID selection prefers the context value, then the incoming header, then a generated ID.
Middleware installs the package-global logger in the request context.
Completion logs include `request_id` from that context. The context logger does not also bind `request_id`, so the field is not logged twice.
Informational responses do not commit a final status, except `101 Switching Protocols`.
The first final `WriteHeader`, `Write`, or `Flush` determines the logged status; implicit writes and flushes use `200`.
A panic after tracked headers are sent does not append an error body or rewrite that status.
`http.ErrAbortHandler` is re-panicked so the HTTP server can abort the response.

### HTTP Client Transport

```go
client := &http.Client{
	Transport: httpx.Transport(nil),
}
```

`Transport` adds basic outbound request logging and request ID propagation.
For more control, use `httpx.NewTransportLogger(rt, logger)` and call `EnableBodyLogging(maxBytes)` when you want small body capture.
Configure body logging before sharing the transport across goroutines. A nonpositive limit uses 32 KiB.
An explicit logger takes precedence over the request context logger; nil uses the context/global logger.
The transport clones request metadata and headers before adding IDs or preparing captured bodies.
It preserves explicit `X-Request-ID` headers and does not write propagated IDs into the caller's request.
The body reader and trailer map remain shared so streaming bodies can populate trailer values at EOF.
Capture reads at most the limit plus one byte even when `ContentLength` is missing or too small.
Oversized bodies stay readable and are logged as skipped.
Parsed media types `application/json`, `application/*+json` (including `application/problem+json`),
and `application/x-www-form-urlencoded` use the package redaction keys, regardless of case or parameters.
With a nonempty key set, invalid JSON or malformed nonempty content-type metadata is logged as `REDACTED`;
the bytes on the wire are unchanged. Other content types are logged as raw text.
Enable body logging only for content you intend to expose in logs.
A body read error closes the failed body and fails the round trip instead of logging a normal completion.

## File Rotation

Size-based rotation is available through `Config.FileMaxSizeBytes` and `Config.FileMaxBackups`.
`FileMaxBackups <= 0` means unlimited rotated backups.
The size limit is in bytes. Rotation creates missing parent directories; plain file output requires them to exist.
It is intentionally simple: no time-based rotation, compression, or external dependency chain.
It is not a drop-in replacement for a full production log rotation system.

## Redaction

```go
// Extend defaults; SetRedactedKeys replaces them.
logx.AddRedactedKeys("secret")
```

Example output:

```text
password=REDACTED
```

Structured field redaction defaults to `apikey`, `password`, `token`, `key`, `access_token`, `refresh_token`, `id_token`, and `client_secret`.
Matching is exact and case-insensitive, including attrs bound with `Logger.With`, `LogValuer` groups, `map[string]string`, and `*url.URL` values.
`SetRedactedKeys` replaces that set exactly; `ClearRedactedKeys` disables structured field redaction.
The key set is package-global, including for independent `New` loggers and HTTP body logging.
Attrs bound with `With` are redacted when bound; later key changes cannot recover their original values.
Redaction does not scrub messages, arbitrary string contents, or error text. Avoid secrets there.
With redaction enabled, recursive map/slice cycles and containers at nesting depth 64 are replaced with `REDACTED` without mutating input values.
Clearing keys also disables JSON/form body redaction and structured traversal.
URL sanitization uses configured keys plus those built-in defaults, even after `ClearRedactedKeys`.
A query name also matches when it ends with `_` plus a redaction key of at least 5 characters, so `session_token` is masked. Short keys such as `key` stay exact-match. Structured fields do not use that suffix rule.
User credentials in the URL userinfo are also redacted.
