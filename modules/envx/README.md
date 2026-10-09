# envx

`envx` loads environment variables into Go structs, with an optional dotenv fallback. It uses only the standard library and requires Go 1.18 or newer.

```sh
go get github.com/rannday/go-kit/modules/envx
```

## Usage

```go
package main

import (
	"log"
	"time"

	envx "github.com/rannday/go-kit/modules/envx"
)

type Config struct {
	Addr     string        `env:"APP_ADDR" required:"true"`
	Timeout  time.Duration `env:"APP_TIMEOUT"`
	Features []string      `env:"APP_FEATURES"`
}

func main() {
	cfg, err := envx.Load[Config]()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Listening on %s with timeout %s", cfg.Addr, cfg.Timeout)
}
```

Set `APP_ADDR=:8080`, `APP_TIMEOUT=5s`, and `APP_FEATURES=search,metrics` in the process environment. For a dotenv fallback, pass `envx.Options{DotEnvPath: "config.env"}` to either loading function. The file must exist and parse successfully, even when process values supply every needed key. No file loads automatically.

## Application defaults

Initialize defaults in Go and overlay them with `LoadInto`:

```go
cfg := Config{Timeout: 10 * time.Second, Features: []string{"search"}}
if err := envx.LoadInto(&cfg); err != nil {
	return err
}
```

`LoadInto` requires a non-nil pointer to a struct. `Load[T]` starts from zero and requires a struct type. Both accept zero or one `Options` argument. `default` tags are unsupported.

## Sources and tags

Process values take precedence over dotenv, including explicit empty values. Missing keys preserve destination values. Loading never modifies the process environment.

| Tag | Behavior |
| --- | --- |
| `env:"KEY"` | Load the explicitly named key |
| `env:"-"` | Skip a field and its descendants, including unexported embeddings |
| `required:"true"` | Require a source key, even if the destination has a default |
| `allow_empty:"true"` | Permit an empty required source value |

Empty required values fail unless `allow_empty` is true. Without `required`, empties are accepted. Required/allow-empty tag values recognize `true` without regard to case.

Untagged nested structs and pointers to structs recurse without key prefixes. Tag their child fields. Nil nested pointers allocate when a child loads. Ordinary unexported fields are ignored; exported children of embedded structs can load.

## Supported values

| Type | Parsing |
| --- | --- |
| `string` | Raw source text |
| `bool` | `strconv.ParseBool` |
| Signed/unsigned integers | Base 10, checked against field width |
| Floating point | `strconv.ParseFloat`, checked against field width |
| `time.Duration` | `time.ParseDuration`, e.g. `250ms` or `5s` |
| `time.Time` | RFC3339 timestamps, including fractional seconds |
| `[]byte` | Raw source bytes |
| Other slices | Comma-separated elements with surrounding whitespace trimmed |
| `encoding.TextUnmarshaler` | Custom conversion from raw source text |

Scalar pointers and nested struct pointers are supported. Slices work when their element type has a supported conversion. Exact `time.Duration` and `time.Time` types use built-in conversions before custom unmarshalling. Exact `[]byte` uses raw bytes.

Empty numerics, bools, durations, and times overwrite destinations with zero. Empty slices become non-nil empty slices. Custom unmarshallers receive empty text and decide whether to accept it. Maps, arrays, interfaces, and tagged structs without a supported conversion return errors.

## Dotenv format

```dotenv
# Synthetic example
APP_ADDR=:8080
export APP_TIMEOUT=5s
APP_FEATURES="search,metrics" # optional features
```

The parser accepts blank lines, full-line comments, an initial UTF-8 BOM, and optional `export `. It trims surrounding whitespace and removes matching outer single or double quotes. A `#` outside quotes starts a comment at the beginning of a value or after whitespace; `abc#def` stays literal. Last duplicate key wins.

There is no variable expansion, escape decoding, or multiline syntax. Unbalanced quotes stay literal; keys are not validated as shell identifiers. Invalid assignments (missing `=` or an empty key) report path and line. Open/scanner errors report path. Scanner maximum token buffer: 1 MiB.

## Errors and partial updates

Exported value errors support `errors.As`:

- `MissingRequiredError`: required source key absent.
- `EmptyRequiredError`: required source value empty.
- `ParseError`: conversion failed; unwraps its original error.
- `UnsupportedFieldError`: unsupported field kind.
- `UnsettableFieldError`: resolved field cannot be assigned.

For example, with the standard `errors` package imported:

```go
var parseErr envx.ParseError
if errors.As(err, &parseErr) {
	log.Printf("Invalid configuration key %s", parseErr.Key)
}
```

Loading is incremental: errors can leave earlier fields updated or scalar pointers allocated. `Load[T]` returns the partially populated value with its error. Discard failed configurations, or load fresh configuration before replacing an active one.

Traversal stops at existing pointer cycles. Existing acyclic chains are loaded, but a nil struct pointer stays nil when its type already appears on the current traversal path. Nil unexported embedded pointers are skipped because reflection cannot allocate them; initialize them before loading if their exported children should load. `env:"-"` skips descendants for both exported and unexported embeddings.

## Development

```sh
go test ./...
go build ./...
go vet ./...
go test -race ./...
```

CI uses the Go version in `go.mod`, checks module tidiness, and runs pinned Staticcheck with Go installation disabled. See [../../docs/envx.md](../../docs/envx.md) for contributor guidance and [../../.agents/skills/envx/SKILL.md](../../.agents/skills/envx/SKILL.md) for agent workflows.
