# go-kit repository guidance

Follow `~/.codex/AGENTS.md` and `~/.codex/workflow.md` for shared tool routing, authorization, and Go workflows. This file records go-kit-specific structure and constraints.

## Structure

- The root is a Go workspace, not a module. Independently versioned modules live in `modules/{logx,envx,confx,buildx}`; each `go.mod` declares its minimum Go version.
- The root README is the consumer-facing entrypoint. Module READMEs contain usage examples; `docs/<module>.md` is the canonical maintenance reference.
- Skills in `.agents/skills/<module>/SKILL.md` route module tasks to their canonical docs.
- Shared formatting, Git attributes, ignore rules, and license live at the root. Preserve unrelated skeleton files, including `READM.md` and root configuration.
- `buildx` keeps its CLI entrypoint and `archive`, `atomicfile`, `build`, and `checksum` packages at the module root.

## Invariants

- Preserve public APIs and behavior. Keep `logx` and `envx` dependency-free; keep `confx` parser versions unchanged.
- Leave the four original repositories untouched. Do not add compatibility wrappers, redirects, or legacy workflows.
- Do not add persistent local replacements.

## Validation

- Run isolated module checks with `GOWORK=off`; run workspace checks separately. Use `scripts/test-all.ps1` or `scripts/test-all.sh` for the full available suite.
- The root `.golangci.yml` is shared across modules. CI owns minimum-Go and pinned Staticcheck lanes; module-specific regression focus is documented in `docs/<module>.md`.

## Releases

Module tags use `modules/<module>/v<version>`. See `docs/releasing.md`.
