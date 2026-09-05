# CI

Every check runs through `make`, so CI runs exactly what you can run locally. There is no logic in
the workflow files beyond wiring — if `make ci` passes on your machine, CI passes.

```bash
make ci     # tidy-check fmt-check schema vet lint test-race cover invariants drift build-arm64
```

## Jobs

| job | what it defends |
| --- | --- |
| `static checks` | `go.mod` tidy, gofmt, `go vet`, golangci-lint, `cue fmt --check`, `cue vet` |
| `tests` | race detector, plus a coverage floor, on the pinned Go and on `stable` |
| `product invariants` | the frozen fixture corpus, and that generated files match their source |
| `build` | host build, a static `linux/arm64` daemon binary, and the SBOM |
| `ci passed` | one gate job to require in branch protection |

`security.yml` runs `govulncheck` and the licence gate on pushes, PRs, and weekly on a schedule, so
a dependency that becomes vulnerable — or that arrives under an unacceptable licence — surfaces even
while nobody is touching the repo. The allowlist and its reasoning are in
[`docs/security/licence-policy.md`](security/licence-policy.md); `make licences` also runs as part
of `make ci`.

Set **`ci passed`** as the only required status check. It fails if any upstream job fails, so
adding a job later does not mean editing branch protection.

## The invariant corpus

This is the part worth understanding, because it defends the product claim rather than the code.

"No infinite scroll" is only true if nobody can quietly loosen it. Tests can be edited by whoever
loosens it, in the same commit. So the invariants are guarded by a **frozen corpus** in
`testdata/invariants/`, checked by two independent paths:

1. `pkg/config/invariants_test.go` validates each fixture in-process and asserts the **specific**
   issue code. A rejection for the wrong reason fails.
2. `scripts/check-invariants.sh` runs the built `normalctl` binary against the same corpus and
   asserts only the exit status. This path shares no code with the Go tests, so deleting or
   weakening a test does not disarm it.

`manifest.json` lists every fixture with the invariant it defends, in prose:

```json
{
  "file": "reject/detectors-emptied.json",
  "code": "empty-detectors",
  "why": "the detector list must stay typed as non-empty"
}
```

Three further guards make the corpus hard to hollow out:

- a fixture on disk but missing from the manifest **fails** the build, so you cannot quietly
  unregister one;
- a fixture count below 20 fails the shell check, so you cannot empty the corpus;
- `TestAttentionInvariantsAreCovered` names the codes that must have a fixture
  (`invalid-enforcement`, `empty-detectors`, `policy-violation`, `reason-too-short`, `expired`,
  `too-distant`, `too-many`), so removing the *last* guard for an invariant fails loudly.

Fixtures are validated at a fixed instant (`manifest.json`'s `now`, exposed as
`normalctl validate --now`) so that time-dependent rules — exemption expiry — are deterministic and
the corpus never rots.

The corpus is deliberately frozen against v0. If a schema change makes an `accept/` fixture fail,
that is not a broken test: it means v0 compatibility broke, and that should be a conscious decision.

### Adding a fixture

1. Write the config into `testdata/invariants/reject/` or `accept/`.
2. Add it to `manifest.json` with the issue `code` you expect and a `why` in plain language.
3. `make invariants`.

## Fuzzing

Seven targets across the three packages, run two ways: `make fuzz-smoke` replays every committed
seed and crasher on each CI run, and a nightly workflow fuzzes each target for five minutes.

| target | property asserted |
| --- | --- |
| `FuzzParsePointer` | parse → format → parse round-trips to the same segments |
| `FuzzSetAtPath` | never mutates its input; a successful set is readable back at the same path |
| `FuzzRemoveAtPath` | never mutates its input; a successful remove changes the document |
| `FuzzValidate` | never panics; every issue carries a machine-readable code |
| `FuzzApplyPatch` | never mutates its input document; issues always explain themselves |
| `FuzzProposeOperations` | **no accepted proposal can leave the scroll invariants unsatisfied** |
| `FuzzDiffDocuments` | diff is empty iff the documents are equal; deterministic; paths are parseable |

`FuzzProposeOperations` is the one worth understanding: it throws arbitrary patch operations at the
agent boundary and asserts that anything which comes back accepted still has enforcement set, the
shim enabled, detectors present, and an advanced revision. It is the invariant corpus's argument
made adversarial — the corpus proves known attacks fail, the fuzzer looks for unknown ones.

Crashers land in `<package>/testdata/fuzz/` and are committed, so a bug found once is replayed on
every run forever.

### Bugs this has already found

Two, both within seconds of the first run:

- **`/spec/apps/entries/000` aliased to index 0.** The numeric check accepted leading zeros, so
  `0`, `00` and `000` all resolved to the same element. RFC 6901 forbids leading zeros in array
  indices, and in a keyed collection the aliasing meant a path could silently address a different
  element than the one it named.
- **`/spec/apps/entries/` silently appended a junk element.** A trailing slash produces an empty
  final segment, which matched no key and was therefore treated as a new key to append. Setting a
  keyed collection member now requires a non-empty segment, and the value's key field must match
  the path that names it — so a patch cannot write `com.foo` at the path `com.bar`. Appending to an
  *unkeyed* array by name is now rejected outright rather than silently placed at the end.

A third came out of writing the targets: `ApplyPatch` accepted the empty pointer, which addresses
the whole document, so a single operation could replace an entire config with `null`. The policy
layer already refused it, but the patch layer now does too.

## Reproducible builds

Two jobs build the same commit on two independent runners and a third compares the digests. If they
diverge, the build fails — provenance (NRM-103) is worth far more when a third party can rebuild and
get the same bytes.

`make verify-reproducible` does the local equivalent: it builds in place, builds again from a copy at
a different path, and compares. Building from a different path is the point — it is what proves
`-trimpath` is actually doing its job.

```bash
make verify-reproducible
make digest                # just print the digest
```

**What reproduction requires.** Go stamps VCS state into the binary — `vcs.revision`, `vcs.time`, and
`vcs.modified` — so matching a released digest means checking out the *same commit* with a *clean
tree*. `make verify-reproducible` refuses to run on a dirty tree rather than reporting a confusing
mismatch.

The determinism comes from three things, all already in place: `-trimpath` removes absolute paths,
`CGO_ENABLED=0` removes the host C toolchain from the equation, and the toolchain version is pinned
by the `go` directive in `go.mod`. Nothing extra was needed to make this work — the check exists to
notice if it ever stops being true.

**Known limits.** A different Go toolchain version produces different bytes; that is expected and is
why the toolchain is pinned. The digests compared in CI are for `linux/arm64`; other targets are
built but not yet digest-checked.

## Bill of materials

`make sbom` produces a CycloneDX 1.6 document covering every module including the standard library,
uploaded by CI and retained for 90 days.

Two properties are injected beyond what `cyclonedx-gomod` emits: `normal:schema:sha256` and
`normal:schema:apiVersion`. The schema is the protocol and it is embedded in the binary, so "which
version of the config contract does this build speak" is a supply-chain question, and the answer
belongs in the bill rather than only in a git tag.

The target self-checks: it fails if the output is not CycloneDX ≥ 1.6, if it has no components, or
if the CUE evaluator is missing from the bill. A generator that silently emits an empty document is
worse than no generator.

## Coverage

The floor is 75%; the repo currently sits a little above it. The floor is a ratchet against
regression, not a target — raise it when the real number moves up, and do not chase it with tests
that assert nothing.

## What is deliberately absent

**Third-party actions.** Only `actions/checkout`, `actions/setup-go`, `actions/cache`, and
`actions/upload-artifact` are used. Linters and scanners are installed with `go install` at versions
pinned in the `Makefile`, so the tool versions are reviewable in a diff rather than floating behind
a marketplace tag. For an OS that people will eventually flash onto a phone, a smaller CI supply
chain is worth a slower job.

**SHA-pinned actions.** Every action is pinned to a full commit SHA with the tag kept in a trailing
comment, so a retagged or compromised action cannot silently run with our token. Dependabot
understands SHA pins and opens PRs when a pinned action publishes a release.

To resolve a SHA without the `gh` CLI:

```bash
git ls-remote --tags --refs https://github.com/actions/checkout | awk '$2=="refs/tags/v4"'
```

The pins hold the majors that were in use when pinning happened, not the newest majors — pinning
freezes current behaviour, and upgrading is a separate change with its own testing. Dependabot will
propose the upgrades.

**CodeQL and `dependency-review-action`.** Neither is wired up: both need Advanced Security on a
private repo and would fail for permissions reasons rather than code reasons. `gosec` runs inside
golangci-lint and covers the common Go footguns, `govulncheck` covers advisories with reachability
analysis, and `make licences` covers the licence half of dependency review. Add both when the repo
goes public.

## Linter policy

`gocritic`'s `hugeParam` and `rangeValCopy` are disabled. They flag passing `config.Config` by value,
which is the point: value semantics are what make the pure diff/plan core safe to reason about.
Trading an immutability guarantee for a 480-byte copy on a config that changes a few times a day is
the wrong trade, so the linter loses that argument rather than the code churning around it.

`govet` runs with `enable-all` except `fieldalignment`, which would reorder struct fields away from
the order the schema declares them in. `errorlint` is on and worth keeping on: it caught three type
assertions on errors that would have silently taken the wrong branch once anything wrapped them.

## Known gap

`make vuln` could not be verified in the environment this was built in — `vuln.go.dev` is blocked by
its network policy, so the job is structurally correct but has never gone green. It is the one check
here that has not actually run. Confirm it on the first CI run.
