# confx

`confx` only loads config files into Go structs.

It supports TOML, JSON, JSONC, YAML, YML, and INI. Format comes from file
extension: `.toml`, `.json`, `.jsonc`, `.yaml`, `.yml`, and `.ini`.

Module path is `github.com/rannday/go-kit/modules/confx`. Package name stays `confx`, so
import with alias:

```go
import confx "github.com/rannday/go-kit/modules/confx"
```

## Install

```sh
go get github.com/rannday/go-kit/modules/confx@latest
```

## Usage

```go
package main

import (
	confx "github.com/rannday/go-kit/modules/confx"
)

type AppConfig struct {
	Name    string `toml:"name" json:"name" yaml:"name" ini:"name"`
	Port    int    `toml:"port" json:"port" yaml:"port" ini:"port"`
	Enabled bool   `toml:"enabled" json:"enabled" yaml:"enabled" ini:"enabled"`
}

func DefaultConfig() AppConfig {
	return AppConfig{
		Name:    "example",
		Port:    8080,
		Enabled: true,
	}
}

func LoadConfig(path string) (AppConfig, error) {
	cfg := DefaultConfig()

	if err := confx.LoadInto(path, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
```

### Embedded configs

```go
//go:embed config.toml
var configFS embed.FS

cfg := DefaultConfig()
err := confx.LoadFSInto(configFS, "config.toml", &cfg)
```

### Bytes without a real file

```go
cfg := DefaultConfig()
err := confx.DecodeInto(data, "config.json", &cfg)
```

The `source` argument must include a supported extension so the decoder can be
selected. It is also included in decode errors.

## API

```go
func Load[T any](path string) (T, error)
func LoadFS[T any](fsys fs.FS, path string) (T, error)
func Decode[T any](data []byte, source string) (T, error)
func LoadInto(path string, dst any) error
func LoadFSInto(fsys fs.FS, path string, dst any) error
func DecodeInto(data []byte, source string, dst any) error
```

```go
var ErrEmptyPath = errors.New("invalid path: empty")
var ErrUnsupportedFormat = errors.New("unsupported config format")
```

Empty or whitespace-only paths are rejected with `ErrEmptyPath`. Unsupported
or missing extensions return errors wrapping `ErrUnsupportedFormat`. Read
errors include the path and preserve the underlying error. Decode errors
include the source and preserve the parser error. `LoadInto`, `LoadFSInto`,
and `DecodeInto` require a non-nil pointer destination and decode into the
existing value without zeroing it first.

## Struct tags

Each decoder reads its own struct tag:

| Format | Tag |
|--------|-----|
| TOML | `toml:"field"` |
| JSON / JSONC | `json:"field"` |
| YAML | `yaml:"field"` |
| INI | `ini:"field"` |

If you load more than one format into the same struct, include the tags for
every format you plan to use.

Put defaults in app code, not tags. Example:

```go
type AppConfig struct {
	Name    string `toml:"name" json:"name" yaml:"name" ini:"name"`
	Port    int    `toml:"port" json:"port" yaml:"port" ini:"port"`
	Enabled bool   `toml:"enabled" json:"enabled" yaml:"enabled" ini:"enabled"`
}

func DefaultConfig() AppConfig {
	return AppConfig{
		Name:    "example",
		Port:    8080,
		Enabled: true,
	}
}

func LoadConfig(path string) (AppConfig, error) {
	cfg := DefaultConfig()

	if err := confx.LoadInto(path, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
```

## Format notes

- `.json` expects standard JSON.
- `.jsonc` allows comments and trailing commas via JWCC parsing.
- `.yaml` and `.yml` use YAML 1.2 parsing. Valid JSON is often also valid YAML.
- `.ini` maps keys from the default section unless your struct is set up for
  named sections:

```go
type Config struct {
    Server struct {
        Name string `ini:"name"`
    } `ini:"server"`
}
```

## Edge cases

| Case | Behavior |
|------|----------|
| Missing file with `Load` / `LoadFS` / `LoadInto` / `LoadFSInto` | read error |
| Empty path | `ErrEmptyPath` |
| Unsupported extension | error wrapping `ErrUnsupportedFormat` |
| Wrong syntax for extension | decode error from selected parser |

### Empty files

| Extension | Behavior |
|-----------|----------|
| `.toml` | zero struct, no error |
| `.json` | error |
| `.jsonc` | error |
| `.yaml`, `.yml` | zero struct, no error |
| `.ini` | zero struct, no error |

An empty TOML, YAML, or INI file therefore loads as the zero value of your
struct. An empty JSON or JSONC file is rejected.
