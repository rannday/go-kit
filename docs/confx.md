# confx

Source paths and commands are relative to `modules/confx/`.

## Purpose and contracts

- Load TOML, JSON, JSONC, YAML/YML, or INI into caller-defined Go structs. File, `fs.FS`, and byte-input APIs share destination validation and wrapped decode errors.
- Preserve the module path, package name, six public entrypoints, and exported sentinel errors. The module minimum is declared in `go.mod`.
- Preserve parser dependency versions in `go.mod` and `go.sum`.
- Use each decoder's expected tag (`toml`, `json`, `yaml`, or `ini`); multi-format structs need all relevant tags.
- `LoadInto`, `LoadFSInto`, and `DecodeInto` decode into the existing value. Do not add defaulting or normalization.
- Preserve errors wrapping `ErrEmptyPath` and `ErrUnsupportedFormat`, source context, and underlying filesystem/parser errors.

## Focused checks

Cover supported extensions, file and `fs.FS` failures, destination validation, preservation of initialized values, and `errors.Is` behavior when changing these contracts.

## Code map

| File | Responsibility |
| --- | --- |
| `config.go` | Public APIs, destination validation, format selection, parser dispatch |
| `config_test.go` | API, decoder behavior, destination and error contracts |
| `testdata/` | Synthetic examples for supported formats |
| `../../.github/workflows/ci.yml` | Module validation and analyzer lanes |
