# buildx

Source paths and commands are relative to `modules/buildx/`.

## Purpose and layout

- Go-only release builder for consumer Go projects.
- Defaults: Windows amd64 ZIP; Linux amd64/arm64 tar.gz; Darwin amd64/arm64 tar.gz.
- Keep the CLI entrypoint and `archive`, `atomicfile`, `build`, and `checksum` packages at module root. Keep README examples generic.

## Behavior contracts

- Preserve module-root path resolution, `CGO_ENABLED=0`, explicit targets replacing defaults, streamed archives, fixed timestamps, executable modes, sorted checksum entries, and staged publication with rollback on errors. Publication is not crash-safe release-wide atomicity.
- Validate checksum/archive collisions, including case-only collisions, before changing output.
- Preserve `--clean` protections for the module root, ancestors, symlink aliases, and directories containing Go source or repository metadata.
- Keep the Go 1.18 minimum and fixture aligned with CI. Check syntax and library APIs before proposing a minimum-version change.

## Focused checks

- CLI changes: check help, exit behavior, output paths, and archive names.
- Archive/checksum changes: cover deterministic bytes, modes, manifest ordering, and cleanup/recovery on failure.
- Fake-Go tests prove orchestration only; establish real consumer cross-compilation before claiming target support.

## Code map

| Area | Files |
| --- | --- |
| CLI entrypoint | `main.go` |
| Flags, help, and validation | `build/args.go`, `build/help.go`, `build/errors.go` |
| Targets and platform discovery | `build/targets.go`, `build/distlist.go`, `build/platform.go` |
| Module paths, tool invocation, and linker flags | `build/paths.go`, `build/run.go`, `build/ldflags.go` |
| Artifact publication | `build/output.go`, `archive/`, `atomicfile/`, `checksum/` |
| Regression coverage | matching `*_test.go` files |
