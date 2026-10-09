# buildx

`buildx` is a reusable Go build tool for Go projects.

It builds deterministic release archives, writes `checksums.txt`, and targets common Windows, Linux, and macOS binaries by default.
Binaries are built to a temp file per target, then streamed into archives with bounded memory via `io.Copy`.

## Install

The builder requires Go 1.18 or newer. Its `strings.Cut` calls and `-buildvcs=false` flag establish that minimum. Consumer projects can require newer Go versions.

With Go 1.18–1.23, install and run the standalone command:

```bash
go install github.com/rannday/go-kit/modules/buildx@latest
buildx -v 1.2.3
```

With Go 1.24 or newer, pin it in a consumer repo with Go tools:

```bash
go get -tool github.com/rannday/go-kit/modules/buildx@latest
```

That adds the tool to the consumer repo's `go.mod`. Installation changes the consumer's module files.

Run it from that repo with:

```bash
go tool buildx -h
go tool buildx -v 1.2.3
```

Pin specific version if you do not want latest:

```bash
go get -tool github.com/rannday/go-kit/modules/buildx@v1.2.3
```

## Usage

The examples below use Go 1.24+ module tools. For a standalone installation, replace `go tool buildx` with `buildx`.

```bash
go tool buildx -v VERSION [flags]
```

`-h, --help` prints help.

The tool walks upward from the current directory to the nearest `go.mod`. Builds run from that module root; relative `--main` and `--out` paths resolve there.

Default output directory is version-scoped:

```text
dist/<version>
```

`--out DIR` uses that directory without appending the version.

`--clean` recursively removes the resolved output directory before building. The tool rejects the module root, its ancestors (including symlink aliases), and directories containing Go source, `go.mod`, or `.git`. Use a dedicated release directory; these checks do not identify every kind of valuable file.

By default, the output directory must be empty except for recognized temporary build files.
`--force` allows overwriting artifacts in a nonempty directory and leaves unrelated files.
Archives and checksums are staged before publication. Failed builds leave existing artifacts intact; publication failures restore replaced files. If restoration fails, the error identifies retained recovery files. This is not crash-safe release-wide atomicity, and `--clean` still removes the old output before building.

Default package detection:

- `--name` defaults to the module-root directory name
- `--main` prefers `./cmd/<name>` when it contains non-test Go files
- otherwise `--main` falls back to the module root when it contains non-test Go files

Detection does not check for `package main` or target-specific build constraints. Set `--main` explicitly when automatic detection chooses the wrong package.

Generated `-ldflags` order:

1. `-s -w -buildid=` unless `--no-strip`
2. `-X <version-var>=<version>` when `--version-var` is set
3. user `--ldflags` value, appended last

Release builds also pass `-trimpath` and `-buildvcs=false` for reproducible binaries.
Every target uses `CGO_ENABLED=0`; projects requiring CGO are not supported by this workflow.
Fixed archive metadata does not guarantee identical binaries across different toolchains, dependencies, or build environments.

Archive filenames:

`<name>-<version>-<goos>-<goarch>.<format>`

Archive notes:

- Windows targets build `.exe` binaries.
- Non-Windows targets build plain binary names.
- Archive timestamps are fixed. Each release archive contains one binary; the checksum manifest is sorted by archive filename.
- Release binaries have mode `0755` in ZIP and tar headers, including Unix targets built on Windows.
- Default targets:
  - `windows/amd64:zip`
  - `linux/amd64:tar.gz`
  - `linux/arm64:tar.gz`
  - `darwin/amd64:tar.gz`
  - `darwin/arm64:tar.gz`
- Default checksum filename: `checksums.txt`
- Default Go binary: `go`

## Custom targets and options

Repeat `--target GOOS/GOARCH[:zip|tar.gz]` to select targets. Any explicit target list replaces all defaults. Omitting the format selects ZIP for Windows and tar.gz otherwise. Platforms are checked against the selected Go toolchain's `go tool dist list`.

```bash
go tool buildx -v 1.2.3 --name myapp --main ./cmd/myapp --target linux/amd64 --target windows/arm64:zip
```

Use `--go PATH` to select a Go executable, `--checksum-name NAME` to change the SHA-256 manifest filename, and `--verbose` to show build commands and Go output.
Keep `--name` and `--checksum-name` as plain filenames without path separators. Checksum names that collide with an archive filename are rejected before output changes, including case-only collisions.
`--no-strip` omits all default linker flags: `-s -w -buildid=`.

## Example

```bash
go tool buildx -v 1.2.3 --name myapp --main ./cmd/myapp --version-var example.com/myapp/internal/app.Version
```

Expected output directory:

```text
dist/1.2.3
```

## Release Workflow

Typical flow:

```bash
go test ./...
go tool buildx -v 1.2.3 --name myapp --main ./cmd/myapp --version-var example.com/myapp/internal/app.Version
```

Use the archives and `checksums.txt` under `dist/<version>` with your own release uploader.

## Development

The CLI entrypoint is `main.go` at the module root. Supporting packages live in the top-level `build/`, `archive/`, `checksum/`, and `atomicfile/` directories.

Run these checks from this repository:

```bash
go test ./...
go vet ./...
go run . --help
```

Agent guidance lives in [../../docs/buildx.md](../../docs/buildx.md). The repository skill at [../../.agents/skills/buildx/SKILL.md](../../.agents/skills/buildx/SKILL.md) covers consumer release builds and maintenance.

GitHub CI runs race tests, builds, and vet on Ubuntu with Go 1.18 and Go 1.26. Staticcheck is pinned and runs only with Go 1.26, reusing that toolchain (`install-go: false`). Keep the minimum CI version aligned with `go.mod`; update the modern lane and Staticcheck pin together after checking compatibility.
