# envx

Source paths and commands are relative to `modules/envx/`.

## Purpose and contracts

- Standard-library-only struct configuration loader. Preserve the module path, package name, small API, and Go 1.18 minimum.
- `Load[T]` starts from zero; `LoadInto` overlays a non-nil struct pointer. Both accept zero or one `Options`.
- Process environment wins over the explicitly selected dotenv file, including empty process values. Missing keys preserve destination values. Defaults belong in Go initialization; `default` tags are unsupported.
- `required:"true"` checks source presence; `allow_empty:"true"` permits empty required values. Loading is incremental and may partially update values on error.
- Preserve conversions, pointer and embedded-field traversal, cycle handling, and exported value error types including `ParseError.Unwrap`.
- Dotenv is opt-in through `Options.DotEnvPath`; explicit missing files fail. Preserve BOM, comments, optional `export `, matching outer quotes, whitespace-preceded inline comments, duplicate precedence, and the 1 MiB scanner limit. No expansion, escape decoding, or multiline values.

## Code map

| File | Responsibility |
| --- | --- |
| `goenv.go` | API, precedence, reflection traversal, conversions |
| `dotenv.go` | Parsing and inline-comment handling |
| `goenv_test.go` | Loading, tags, conversions, pointers, embeddings, errors |
| `dotenv_test.go` | Grammar, duplicates, BOM, malformed lines, missing files |
| `dotenv_fixtures_test.go` | Synthetic temporary fixtures and cleanup |
| `../../.github/workflows/ci.yml` | Minimum-Go and current-toolchain checks |

## Regression focus

For parser changes, cover quotes/comments, precedence, empty versus absent, duplicates, and diagnostics. For reflection changes, cover nil and populated pointers, cycles, finite chains, sibling paths, and skipped embeddings.
