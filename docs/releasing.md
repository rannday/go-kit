# Independent module releases

Releasing is a separate, explicitly authorized operation. This implementation
has created no commits, tags, pushes, or releases. Module README installation
examples assume a published version; no new paths are published yet.

## Go publishing rules

Each module has a location-correct declaration in its own go.mod. Git tags must
include the complete repository-relative module directory. Consumers specify
the unprefixed semantic version:

| Module | First Git tag | Consumer version |
| --- | --- | --- |
| logx | modules/logx/v0.1.0 | v0.1.0 |
| envx | modules/envx/v0.1.0 | v0.1.0 |
| confx | modules/confx/v0.1.0 | v0.1.0 |
| buildx | modules/buildx/v0.1.0 | v0.1.0 |

These conventions follow [Go's multiple-module repository guidance](https://go.dev/doc/modules/managing-source#multiple-module-source)
and [module version-to-commit rules](https://go.dev/ref/mod#vcs-version).
The workflow accepts canonical v0/v1 versions, including valid prereleases,
rejecting leading zeros, build metadata, and v2+. A v2 release would require an approved
module path ending `/v2`, with tags still rooted at the module's directory.
No root v0.1.0 tag is used for these modules. Each track advances independently.

## Before the first release

1. Review docs/validation.md. Resolve or explicitly accept pre-existing platform
   failures; do not silently suppress tests. Confirm Go 1.26 CI and buildx's Go
   1.18 lane, pinned analyzers, workspace integration, and Windows results.
2. Inspect the final worktree. Obtain authorization before committing or pushing
   the fresh repository. Originals remain untouched; no history is imported.
3. Confirm the intended GitHub repository and SSH remote. Before an authorized
   push use `git remote -v`; retain the correct account-specific SSH alias.
4. Configure GitHub Actions and review release permissions. Dispatch **Release
   module** only after explicit publication authorization, choosing one module
   and its first version `v0.1.0`. Never execute it merely to test automation.

The workflow validates names, declarations, version syntax, tag absence, and
isolated checks. buildx additionally requires its Go 1.18 check. Release runs
serialize per module and cannot cancel a publishing run. Buildx's five existing
targets are built with the `buildx` name, checked for executable archive members
and sorted SHA-256 entries, and checked again after artifact transfer.

Module-specific notes are generated from changes under that module since its
previous tag. Publication checks remote tag absence again, creates the tag at
the validated commit without force, creates a draft release, uploads verified
assets for buildx, then publishes. Library releases have no binary attachments.

## Local validation without publishing

```sh
bash scripts/validate-release.sh logx v0.1.0
go run scripts/verify-consumer.go
```

The consumer uses fresh temporary replacements and a fresh module cache, with
GOWORK=off. Nothing is installed to the user's GOBIN. Local replacements prove
integration and package naming; they cannot prove remote tag discovery.

## After publication

The workflow retries clean external resolution six times with a 30-second pause.
For libraries it imports the exact tagged module (plus httpx for logx), downloads
that version, and compiles outside the workspace without replacements. For
buildx it installs the exact tagged command into a temporary GOBIN and verifies
the executable filename and help. To reproduce independently:

```sh
go run scripts/verify-consumer.go -module logx -version v0.1.0
go run scripts/verify-consumer.go -module buildx -version v0.1.0
```

A failure after tag creation leaves the tag and any release intact. Inspect
proxy propagation or release/upload errors. Retry verification or complete the
existing draft only after approval; never delete, move, force-push, or reuse a
published tag. Fix source under a new version. Actual published resolution
remains pending until an approved first release.
