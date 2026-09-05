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
| `build` | host build and a static `linux/arm64` daemon binary |
| `ci passed` | one gate job to require in branch protection |

`security.yml` runs `govulncheck` on pushes, PRs, and weekly on a schedule, so a dependency that
becomes vulnerable while nobody is touching the repo still surfaces.

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

**SHA-pinned actions.** Actions are pinned to major tags, which are mutable. Hardening step, when
you want it:

```bash
gh api repos/actions/checkout/git/ref/tags/v4 --jq .object.sha
```

then replace `@v4` with `@<sha>` and keep the tag in a trailing comment.

**CodeQL.** Not wired up, because it needs Advanced Security on a private repo and would fail for
permissions reasons rather than code reasons. `gosec` runs inside golangci-lint and covers the
common Go footguns. Add CodeQL when the repo goes public.

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
