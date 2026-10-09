# go-kit

Four independently versioned Go modules, one development workspace.

| Module | Import path | Minimum Go |
| --- | --- | --- |
| [logx](modules/logx/README.md), including httpx | `github.com/rannday/go-kit/modules/logx` | 1.26 |
| [envx](modules/envx/README.md) | `github.com/rannday/go-kit/modules/envx` | 1.26 |
| [confx](modules/confx/README.md) | `github.com/rannday/go-kit/modules/confx` | 1.26 |
| [buildx](modules/buildx/README.md), CLI and supporting packages | `github.com/rannday/go-kit/modules/buildx` | 1.18 |

logx and envx use only the standard library. confx retains its existing TOML,
YAML, INI, and JSONC parsers. buildx builds release binaries, archives, and SHA-256
manifests; its archive, atomicfile, build, and checksum packages remain public.

## Install after the first release

Each module starts its own version track at v0.1.0. These commands become valid
after the corresponding release is published:

```sh
go get github.com/rannday/go-kit/modules/logx@v0.1.0
go get github.com/rannday/go-kit/modules/envx@v0.1.0
go get github.com/rannday/go-kit/modules/confx@v0.1.0
go install github.com/rannday/go-kit/modules/buildx@v0.1.0
buildx --help
```

Import HTTP helpers from `github.com/rannday/go-kit/modules/logx/httpx`.
Consumers use plain versions such as `v0.1.0`; Git tags include the directory
prefix, such as `modules/logx/v0.1.0`. There is no root module to install.

## Develop

Use Go 1.26+ for the root workspace. Run from the repository root:

```sh
bash scripts/test-all.sh --race
bash scripts/test-all.sh --workspace --race
go run scripts/verify-consumer.go
```

PowerShell equivalents:

```powershell
./scripts/test-all.ps1 -Race
./scripts/test-all.ps1 -Mode workspace -Race
go run ./scripts/verify-consumer.go
```

The scripts enumerate module roots; root `go test ./...` is not the multi-module
test command. Isolated checks set `GOWORK=off`; workspace checks use go.work.
Tidiness checks run `go mod tidy` and reject any changed go.mod/go.sum, including
a newly created go.sum. Inspect resulting changes if a tidiness check fails.
The external verifier uses a disposable consumer, local replacements, module
cache, and binary directory. It also performs real five-target cross-compilation.

See [migration](docs/migration.md), [release procedure](docs/releasing.md),
[validation results](docs/validation.md), and [remaining work](TODO.md).
The four original repositories remain untouched backups. No compatibility
wrappers or old executable aliases are provided. All modules retain WTFPL v2.
