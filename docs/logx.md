# logx

Source paths and commands are relative to `modules/logx/`.

## Scope and layout

- Zero-dependency library, not a CLI. Keep the module path and root package name.
- Root owns configuration, handlers, contextual logging, errors, timing, redaction, stacks, and rotation. HTTP helpers live in `httpx/`.
- Keep public API changes intentional and update README examples and focused tests when contracts change.

## Behavior to preserve

- `New` returns an independent logger and caller-owned closer. `Configure` manages the global logger and installs `slog.Default()` only after successful setup.
- Failed configuration preserves the previous global logger. Successful replacement closes the previous sink; closing a file-backed logger disables its derived loggers and console output too.
- Preserve `FileWriter` precedence, single closer ownership, idempotent close, and write/close serialization.
- Keep console and file JSON options independent, source attribution on caller frames, and explicit stack enablement at level zero.
- Redaction uses case-insensitive exact structured-key matching, recurses through groups, masks URL credentials, and preserves query suffix rules. It does not scrub arbitrary strings, messages, or error text.
- Preserve redaction cycle detection, bounded traversal, and input immutability. Context IDs and context loggers are separate; preserve middleware ID priority and outbound propagation.
- Clone outbound request metadata/headers before modification; preserve dynamic trailers, final status tracking, Flush, and re-panic of `http.ErrAbortHandler`.
- Body logging remains opt-in and bounded; preserve consumer-visible bytes, unknown-length behavior, oversized-body skipping, and read-error propagation.
- Preserve rotation size accounting, backup ordering, rename-failure recovery, and close behavior.

## Regression focus

Target lifecycle/concurrency tests for handler or configuration changes; redaction tests for nested values and URLs; rotation tests for boundaries and failures; HTTP tests for status, panic, IDs, body preservation, and closer ownership. Run benchmarks only for performance investigations.

## Code map

| Area | Files |
| --- | --- |
| Configuration, sinks, handlers, stacks | `config.go`, `logx.go`, `handler.go`, `stack.go` |
| Context, IDs, errors, timing | `context.go`, `id.go`, `decorator.go` |
| Structured values and URL sanitization | `redaction.go` |
| Rotation and recovery | `rotator.go` |
| Server middleware | `httpx/middleware.go` |
| Client logging, IDs, capture | `httpx/client.go`, `httpx/logging.go`, `httpx/transport.go` |
| Public examples and performance | `README.md`, `doc.go`, `httpx/doc.go`, matching example/benchmark tests |
| Regression tests | matching `*_test.go` files |
