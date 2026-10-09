# Migration record

This is a fresh source snapshot, with no imported Git history. The originals
remain unchanged backups. Consumers must change imports and use `buildx` as the
executable name. There are no compatibility repositories, wrappers, or redirects.

| Original | Source commit | Destination |
| --- | --- | --- |
| go-log | `4f9e7ac2b214ee6507aa595e3ead48ecbed9e43a` | modules/logx |
| go-env | `db14048faf621df4755801dd416fc0e9fe1cdfd3` | modules/envx |
| go-conf | `53ec6143150c7e1075cb7f2d4c38f662a4922f15` | modules/confx |
| go-build-bin | `62abecb969f90343a62bff74d13523fb6207d55f` | modules/buildx |

`source-inventory.json` records the 101 migrated tracked files, original commit,
original SHA-256, and destination for every file. Git metadata, original CI,
Serena metadata, and client configuration were excluded. Original tests and
fixtures were copied. Original module guidance was copied where it existed;
`docs/confx.md` was written for this consolidated module. Identical licenses
and formatting files are consolidated at the repository root, along with
merged ignore rules. The four module skills live in root `.agents/skills/`.
Module guidance lives in `docs/{logx,envx,confx,buildx}.md`, with source paths
resolved from the corresponding module directory. The original confx
repository has no AGENTS.md.
Inventory entries with
`repository: true` resolve their destination from the repository root; other
entries resolve from `modules/<module>/`. Original hashes remain unchanged.

The shared root LICENSE applies to all modules. Go includes that LICENSE in
submodule archives when no module-local LICENSE exists (see the
[Go module reference](https://go.dev/ref/mod#vcs-files)).

The original go-kit `.editorconfig`, `.gitattributes`, `.golangci.yml`, LICENSE,
`.serena/`, PLAN.md, and empty READM.md are retained. The empty AGENTS.md is filled;
TODO.md is now an implementation and release checklist. CI lives at the root.

## Consumer changes

Replace `github.com/rannday/go-log` with
`github.com/rannday/go-kit/modules/logx` (including `/httpx`), `go-env` with
`go-kit/modules/envx`, `go-conf` with `go-kit/modules/confx`, and `go-build-bin`
with `go-kit/modules/buildx`, retaining the `github.com/rannday/` prefix.
Replace standalone and `go tool` executable references with `buildx`.
Package names, exported symbols, error contracts, and dependency versions stay
the same. There are no cross-module dependencies or persistent replacements.

## Reproduce the migration audit

With the four original repositories beside go-kit:

```sh
go run scripts/verify-migration.go -sources ..
```

The verifier checks recorded hashes, module declarations and dependency text,
all Go source and tests after approved substitutions, and exported AST APIs.
Beyond path and CLI changes, the verifier permits the module minimum Go version
adjustments recorded in the module `go.mod` files, CRLF/final-newline normalization
and the two approved confx test corrections: exact quoted-path prefixes using
`strconv.Quote` and their import. Production errors and original tests are
unchanged. Independently compared `go doc -all` output also matches
all nine packages. See validation.md for API totals and baseline failures.

## Rollback

Before release, stop using the new checkout and return to the untouched original
repositories at the commits above. Consumer changes can be reverted separately.
Keep current source snapshots and original backups until acceptance is complete.
After publication, preserve every released tag/version. Release fixes under a
new version; never rewrite or delete published tags. Repository archival and
consumer migration are outside this implementation and require authorization.
