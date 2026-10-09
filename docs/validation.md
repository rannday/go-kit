# Migration validation

Recorded 2026-10-09. No commits, pushes, real tags, releases, archival, or original
repository edits were performed. All four originals remained clean at completion.
Their source commits and 101 copied-file SHA-256 values are in
[source-inventory.json](source-inventory.json).

## Baseline before copying

The previously failing native process helper was retried. Desktop Commander's
process runner successfully executed the original tests with `GOWORK=off`,
`GOTOOLCHAIN=local`, and `go test -mod=readonly ./...`.

| Original | Go 1.27.0, Windows amd64 | Go 1.19.8, Debian Linux amd64 |
| --- | --- | --- |
| go-log | Both packages passed | Cannot compile: standard-library log/slog unavailable |
| go-env | Passed | Passed; does not establish the then-declared Go 1.26 minimum |
| go-conf | Two failures below | Cannot compile: standard-library iter unavailable |
| go-build-bin | All five packages passed | All five packages passed |

The Windows confx failures are pre-existing:

- `TestReadErrorsIncludePath`, config_test.go:471.
- `TestUnsupportedFormatWrapsErrUnsupportedFormat`, config_test.go:522.

Both originally expected an unescaped Windows path substring while existing
`%q` error formatting doubles backslashes. The originals remain untouched.
The migrated test expectations were subsequently corrected as described below;
production error behavior remains unchanged. No tests were skipped or weakened.

## Initial migrated-module checks (before the test correction)

PowerShell isolated checks ran with Go 1.27.0 Windows amd64 and `GOWORK=off`:

| Gate | logx | envx | confx | buildx |
| --- | --- | --- | --- | --- |
| gofmt | Passed | Passed | Passed | Passed |
| go mod tidy, no module-file changes | Passed | Passed | Passed | Passed |
| Tests | Passed | Passed | Same two baseline failures | Passed |
| Race tests | Passed | Passed | Same two baseline failures | Passed |
| Build and vet | Passed | Passed | Passed | Passed |

The isolated validation script correctly returns failure for confx. Workspace
tests/race/build/vet also reproduce only those two confx failures; other modules
pass. Workspace mode uses `GOWORK=auto`, so the outer commands discover go.work
and buildx's child commands in temporary fixture modules discover their own
module context. Forcing the absolute workspace path initially caused four
fixture failures; correcting the validation harness removed those failures
without modifying source or tests.

Debian WSL Go 1.19.8 additionally passed buildx's isolated formatting, unchanged
tidy, tests, race tests, build, and vet. Go 1.18 and 1.26 are not installed in
these tested environments. Their explicit CI lanes are implemented but have not
run; newer-toolchain successes do not prove minimum-version compatibility.

Three original logx files lacked a final newline and appeared in the original
gofmt check: decorator.go, doc.go, and httpx/client.go. The copies gained only
their final newline to satisfy formatting. No other implementation changes were
made beyond namespace and approved CLI branding substitutions.

## Public API and implementation comparison

An independent comparison covered all nine packages using normalized
`go doc -all` output (481 lines). It found no differences. It counted 68 exported
functions, 10 methods on exported types, 15 types, 50 exported struct fields,
three constants, three variables, and five named envx error types.

The reproducible AST/source verifier also passed:

```sh
GOWORK=off go run scripts/verify-migration.go -sources ..
```

It verifies 101 original file hashes and copied-file presence, all 63 Go files
(33 production, 30 test), normalized implementations and tests, module
declarations, dependency versions, and exported declarations. Its method count
is 36 because it also includes exported methods on private implementation
receiver types; those too are unchanged. The only permitted source differences
are module/self-import paths, the approved flag-set/help branding changes, and
line-ending/final-newline normalization, plus the two explicitly approved confx
test expectation corrections and their `strconv` import. No other public API
differences were found. logx/envx remain dependency-free, confx retains all four exact parser
versions, and buildx retains its Go 1.18 directive and dependency-free module.

## Consumer and release automation

The local consumer verifier passed outside the workspace with a fresh temporary
module, fresh module cache, temporary replacements, and temporary GOBIN. It
compiled logx, httpx, envx, and confx, installed buildx under its new filename,
checked help branding, and performed real cross-compilation for Windows amd64,
Linux amd64/arm64, and Darwin amd64/arm64. Archive verification checked names,
nonempty executable members, permissions, archive integrity, exact asset count,
and SHA-256 entries sorted by filename. Temporary integration files were removed.

The real release build command was also exercised locally with `v0.1.0`, with
positive archive verification and rejection checks for an invalid checksum
manifest and an extra asset. No Git tag was created by this test.

Both Bash scripts passed syntax validation. Release guards accepted each
directory-prefixed first version and a valid prerelease, rejected invalid
modules, v2 paths, malformed/leading-zero versions, build metadata, and injected
shell text. An existing-tag rejection was tested with a simulated Git command,
without creating a real tag. CI/release/Dependabot YAML parsed successfully
using confx's already pinned YAML parser; the Staticcheck action inputs were
checked against its official documentation. This is local validation, not a
GitHub Actions execution or a complete Actions-schema linter result.

All three Go validation utilities passed formatting and vet. Go module
directory/tag conventions were checked against official Go documentation;
declarations and tags use `modules/<module>/v0.1.0`. No persistent replacements
or root go.mod were introduced. Actual exact-version remote resolution remains
unverified until an explicitly authorized publication. The workflow includes
clean post-publication checks and retries without deleting or rewriting tags.

## Pre-release validation after the Windows test correction

`modules/confx/config_test.go` now checks an anchored `read ` or `decode `
prefix, `strconv.Quote(path)`, and the exact `: ` delimiter. This matches the
existing production `%q` formatting on Windows and POSIX systems and is stronger
than the previous substring assertion. Both tests still require a non-nil error;
the unsupported-format test still requires
`errors.Is(err, ErrUnsupportedFormat)`. No production code, public API, error
behavior, parser dependency, module directive, or original repository changed.
The source verifier permits only these exact test substitutions and the import;
recorded original hashes remain unchanged.

Both affected tests were run individually with `-count=1 -mod=readonly` and
`GOWORK=off`, and passed on Windows Go 1.27.0 amd64. The complete available
validation suite was then rerun:

| Validation | Result |
| --- | --- |
| Windows Go 1.27.0: isolated checks for all four modules, `GOWORK=off` | Formatting, unchanged tidy, tests, race tests, build, vet passed |
| Windows Go 1.27.0: workspace checks for all four modules, `GOWORK=auto` | Tests, race tests, build, vet passed |
| Debian WSL Go 1.19.8 amd64: isolated buildx | Formatting, unchanged tidy, tests, race tests, build, vet passed |
| Migration verifier | All 101 original hashes, 63 normalized Go files, module/dependency declarations, and exported APIs passed |
| External consumer with fresh module/cache/GOBIN and local replacements | Compilation, buildx installation/help, five-target cross-compilation and archive verification passed; temporary files removed |
| Actual local buildx release build | Five archives/checksums passed; invalid checksum and extra asset were correctly rejected |
| Release automation | Bash syntax, valid versions, invalid/injected versions/modules and simulated existing-tag rejection passed; no real tag created |
| CI, release and Dependabot YAML | Parsed successfully with the existing pinned YAML parser |
| Three Go validation utilities | Formatting and individual vet checks passed |
| Gopls diagnostics for changed Go files | No errors; only existing modernization hints/information |
| Installed Staticcheck 2024.1.1 (v0.5.1), all four modules | Failed to import Go 1.27 export data (`unsupported version: 4`); not an analyzer pass |

Full-suite commands use Go's normal test cache; the two focused test runs were
explicitly uncached. All four original Git worktrees were rechecked clean.
The source/API audit still reports no public API differences.

## Toolchain vulnerability review

Gopls v0.23.0 scans of all four modules returned the same 13 toolchain advisories
using Go 1.27.0 and the database updated 2026-10-08 22:31:09 UTC. These were
reviewed individually against the official Go vulnerability database on
2026-10-09. Each standard-library entry
is fixed in **Go 1.26.9 and Go 1.27.2**; both versions are available in the
[official downloads](https://go.dev/dl/). The tested local Go 1.27.0 remains
affected. No toolchain was installed or upgraded and no dependency changed.

| Advisory | Issue | Applicability to these modules |
| --- | --- | --- |
| [GO-2026-6599](https://pkg.go.dev/vuln/GO-2026-6599) | JavaScript template context tracking | No `html/template` production dependency or usage |
| [GO-2026-6600](https://pkg.go.dev/vuln/GO-2026-6600) | JavaScript `yield` escaping | No `html/template` production dependency or usage |
| [GO-2026-6603](https://pkg.go.dev/vuln/GO-2026-6603) | HTTP/2 trailer memory exhaustion | Consumers serving logx/httpx handlers over HTTP/2 can be affected |
| [GO-2026-6604](https://pkg.go.dev/vuln/GO-2026-6604) | Windows os.Root junction escape | No os.Root API usage; ordinary os file operations do not establish this exposure |
| [GO-2026-6605](https://pkg.go.dev/vuln/GO-2026-6605) | HTTP/1 client CONNECT desynchronization | logx/httpx delegates to a RoundTripper, defaulting to http.DefaultTransport; affected CONNECT/proxy traffic can expose consumers |
| [GO-2026-6607](https://pkg.go.dev/vuln/GO-2026-6607) | TLS ECH server memory exhaustion | TLS is a transitive logx/httpx dependency; exposure requires a consumer's affected TLS server/ECH configuration, not an ECH server created here |
| [GO-2026-6608](https://pkg.go.dev/vuln/GO-2026-6608) | MIME header memory-limit bypass | Multipart/textproto are transitive logx/httpx dependencies; affected parsing is consumer-dependent, with no direct MIME/multipart parser calls here |
| [GO-2026-6609](https://pkg.go.dev/vuln/GO-2026-6609) | Excessive Range-header parsing | No file-serving calls here; consumers wrapping FileServer/ServeContent/ServeFile handlers can be affected |
| [GO-2026-6610](https://pkg.go.dev/vuln/GO-2026-6610) | HTTP/2 response framing headers | logx/httpx default transport can reach affected HTTP/2 processing; forwarding responses to HTTP/1 can expose consumer proxies |
| [GO-2026-6611](https://pkg.go.dev/vuln/GO-2026-6611) | HTTP/2 flow-control CPU exhaustion | Both HTTP/2 clients through logx/httpx transport and consumers serving its handlers can be affected |
| [GO-2026-6612](https://pkg.go.dev/vuln/GO-2026-6612) | HTTP/2 double flow-control refund | Consumers serving logx/httpx handlers over HTTP/2 can be affected |
| [GO-2026-6613](https://pkg.go.dev/vuln/GO-2026-6613) | HTTP/1 successful CONNECT desynchronization | Consumer handlers replying 2xx to CONNECT without hijacking can be affected; no CONNECT handler is implemented here |
| [GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617) | HTTP/2 HPACK encoder race/crash | Consumers serving logx/httpx handlers over HTTP/2 can be affected |

`GOWORK=off go list -deps ./...` confirmed the HTTP/TLS/MIME packages occur only
in logx's production dependency graph, not envx, confx or buildx. Source review
confirmed delegation to the default transport and no direct template, os.Root,
multipart or file-serving operations. This is an applicability review, not a
proof of exploitability or a symbol-level clean scan: govulncheck is not installed.
The gopls scan reported no third-party module advisories. Some HTTP/2 reports also
cover golang.org/x/net; none of these modules depends on that module.

Use the patched supported toolchain when building release artifacts and consumer
binaries, then repeat validation and a vulnerability scan. Go 1.18/1.19 are
compatibility-test toolchains, not supported security-patched release toolchains.
Updating the local toolchain requires approval.

## GitHub Actions readiness

CI and release workflows were reviewed without dispatching a release. The
Go 1.26.x lanes cover all four isolated modules, workspace integration and
Windows; the Go 1.18.x Linux lane covers buildx including race tests. Release
validation repeats the selected module and integration checks, and buildx's
minimum-version job gates publishing. All six setup-go steps now use
`check-latest: true` within the existing version selectors, avoiding an older
cached patch as documented by
[setup-go](https://github.com/actions/setup-go/blob/v6/docs/advanced-usage.md#check-latest).
The Go 1.26 series currently includes patched 1.26.9; check the actual selected
version in the Actions log before publication. No Go series or module directive
was changed.

Staticcheck remains explicitly pinned to v0.7.0 for envx and v0.8.1 for the other
modules in both workflows, with `install-go: false` and module working
directories. The [2026.1 notes](https://github.com/dominikh/go-tools/blob/master/website/content/changes/2026.1.md)
document Go 1.26 support. The pinned
[v0.7.0 go.mod](https://github.com/dominikh/go-tools/blob/v0.7.0/go.mod)
requires Go 1.25.0 and
[v0.8.1 go.mod](https://github.com/dominikh/go-tools/blob/v0.8.1/go.mod)
requires Go 1.26.0, so both can be built on the Go 1.26 lane. The
[action's declared inputs](https://github.com/dominikh/staticcheck-action/blob/v1/action.yaml)
match the workflow configuration. Staticcheck is intentionally skipped in the
Go 1.18 lane; the module's go.mod supplies the analyzer's language minimum in
the newer-toolchain lane.

Workflow paths, matrices, dispatch inputs, job dependencies, permission scopes,
artifact handoff and directory-prefixed tags were reviewed. YAML parsing and
local script/integration checks passed at that point. A later golangci-lint
workflow addition is recorded below. Neither actionlint nor the exact Go
1.18/1.26 toolchains nor the pinned analyzers are installed locally. Actual
Actions runs remain required before release; local review and Go 1.19/1.27
results do not establish those lanes passed.

## Remaining checks and limitations

- Run the Go 1.26 matrix, actual Go 1.18 buildx lane, pinned Staticcheck, and
  the newly added golangci-lint v2.13.2 checks in CI.
  The locally installed Staticcheck 2024.1.1 (v0.5.1) failed importing Go 1.27
  standard-library export data (`unsupported version: 4`). No analyzer pass is
  claimed and no new tool installation was performed.
- Approve a local patched Go toolchain update and rerun validation/security
  checks before using this environment to build release artifacts. All 13
  advisories are reviewed above; HTTP consumers can be affected. CI's existing
  Go 1.26.x selector can resolve patched 1.26.9 without changing module directives.
- Obtain authorization for the initial commit/push and each first release.
  Review GitHub remote, Actions permissions, and CI outcomes before dispatch.
- After approved publication, run exact tagged consumer checks without local
  replacements. Local integration cannot establish remote tag/proxy discovery.

TODO.md was updated incrementally with a current checklist and the original
proposal retained as explicitly superseded historical reference, after
automatic approval review rejected a replacement that could discard detail.
The root skeleton remains present; READM.md is still empty. PLAN.md is unchanged.

## Follow-up migration and guidance audit (2026-10-09)

The migration verifier was rerun against the four adjacent original repositories.
It verified all 101 inventoried source hashes and destinations, all 63 Go files,
module paths and dependency text, and exported APIs (68 functions, 36 methods,
15 types, 50 struct fields, three constants, three variables, and five named
error types). No API or implementation differences were reported beyond the
already approved namespace, buildx naming, confx test expectation, formatting,
and minimum Go-version changes. All four original worktrees remained clean.

The verifier now permits exactly the module Go directives recorded in `go.mod`:
logx 1.21, envx 1.18, confx 1.23, and buildx 1.18. It still compares dependency
text exactly. This follow-up ran the migration verifier only; it did not rerun
module tests or CI. Go-minimum CI coverage remains Go 1.18 for envx/buildx and
Go 1.23 for confx; hosted Actions results are still required before release.

## golangci-lint configuration and CI (2026-10-09)

The root .golangci.yml was migrated from v1 to v2 so CI can use the current
golangci-lint action. The explicitly enabled linters, govet analyzers
(excluding fieldalignment), goconst thresholds, gocritic tags, unlimited
issue counts, vendor-path exclusion, test-file errcheck/goconst exclusions,
and gofmt/goimports checks were carried forward. In v2, formatting checks are
configured under formatters.

CI now pins golangci-lint v2.13.2 and runs it for each module on the Go 1.26
matrix lane. It passes the root config path explicitly. Go minimum-version
lanes and the existing pinned Staticcheck steps remain in place. Module docs,
skills, and AGENTS.md point maintainers to this CI arrangement.

This change was not lint-validated locally: the installed binary is
golangci-lint v1.64.8 built with Go 1.26.0, and it rejects the local Go 1.27.0
target as newer than its export-data support. It cannot verify a v2 config
either. No tool was installed, and no hosted workflow has run; the new config
and action remain unverified until the Go 1.26 CI jobs execute successfully.

## logx minimum-version test compatibility (2026-10-09)

The full Windows isolated suite initially failed in `logx/httpx` because two
tests used `slog.DiscardHandler`, which requires Go 1.24 despite the module's
Go 1.21 directive. The untouched original module declares Go 1.26 and passed
the same package tests. Replaced those test handlers with
`slog.NewTextHandler(io.Discard, nil)`, preserving discard behavior while
keeping the tests compatible with Go 1.21. The focused logx tests and complete
isolated `scripts/test-all.ps1` suite then passed on Go 1.27.0 Windows/amd64;
gopls reported no diagnostics. The migration verifier was updated to allow
only this exact test compatibility substitution and is being rerun. Go 1.21
and hosted CI lanes were not available locally and remain unverified.
