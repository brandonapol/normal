# M1 — Supply chain & CI hardening

**Goal:** anyone can verify that a binary claiming to be Normal was built from this source, by this
pipeline, with these dependencies — before there are users to migrate.

CI today covers correctness: static checks, race, coverage floor, the invariant corpus, arm64
builds. What it does not yet cover is *provenance* and *adversarial input*. That is this milestone.

---

### NRM-101 — Pin every GitHub Action to a commit SHA
**Size:** S — **Depends on:** none

Actions are pinned to mutable major tags. A compromised or retagged action runs with our token.

**Scope**
- Replace `@v4`-style refs with full SHAs, tag retained in a trailing comment
- Configure Dependabot to keep SHAs updated (it understands SHA pins)
- Document the resolution command in `docs/ci.md`

**Acceptance**
- [ ] No workflow references an action by tag
- [ ] Dependabot opens a PR when a pinned action publishes a new release
- [ ] `make ci` unchanged

---

### NRM-102 — Generate an SBOM per build
**Size:** M — **Depends on:** none

**Scope**
- CycloneDX SBOM from `go.mod` via `cyclonedx-gomod`, pinned by version
- Emitted for every CI run, attached to releases
- Include the CUE schema hash so the protocol version is part of the bill

**Acceptance**
- [ ] `make sbom` produces a valid CycloneDX 1.6 document
- [ ] CI uploads it as an artifact; releases attach it
- [ ] SBOM lists `cuelang.org/go` and its transitive set

---

### NRM-103 — Build provenance attestation
**Size:** M — **Depends on:** NRM-101

**Scope**
- `actions/attest-build-provenance` over release binaries and the SBOM
- SLSA build level 2 as the target; document what is and is not claimed
- Verification instructions using `gh attestation verify`

**Acceptance**
- [ ] Every released binary has a verifiable provenance attestation
- [ ] `docs/security/verifying-a-build.md` walks through verification end to end
- [ ] A deliberately modified binary fails verification (documented test)

---

### NRM-104 — Sign releases with cosign
**Size:** M — **Depends on:** NRM-103

Keyless OIDC signing, no long-lived keys to steal.

**Acceptance**
- [ ] Release binaries and SBOM carry cosign signatures
- [ ] Verification documented alongside NRM-103
- [ ] Signing identity is the workflow, asserted in the certificate

---

### NRM-105 — Reproducible builds
**Size:** L — **Depends on:** NRM-103

Provenance is worth much more when a third party can independently rebuild and get the same digest.

**Scope**
- Pin the Go toolchain exactly; `-trimpath`, cleared build IDs where needed
- CI job that builds twice on different runners and diffs the digests
- Document the remaining sources of nondeterminism honestly

**Acceptance**
- [ ] Two independent CI runs of the same commit produce byte-identical `linux/arm64` binaries
- [ ] The rebuild recipe is a documented one-liner
- [ ] Divergence fails the build rather than being reported and ignored

---

### NRM-106 — SECURITY.md and disclosure policy
**Size:** S — **Depends on:** none

**Acceptance**
- [ ] `SECURITY.md` with a reporting channel, expected response time, and safe-harbour language
- [ ] Supported-versions table
- [ ] Private vulnerability reporting enabled on the repo

---

### NRM-107 — CodeQL, secret scanning, push protection
**Size:** S — **Depends on:** repo made public, or GHAS

Deferred from the initial CI deliberately: on a private repo without Advanced Security these fail
for permissions reasons rather than code reasons.

**Acceptance**
- [ ] CodeQL runs on PRs and weekly
- [ ] Secret scanning with push protection is on
- [ ] `docs/ci.md` note about the deferral is removed

---

### NRM-108 — Dependency review and license policy
**Size:** S — **Depends on:** none

**Scope**
- `actions/dependency-review-action` on PRs, failing on new high-severity advisories
- Allowlist of acceptable licences; a copyleft dependency should block, not surprise

**Acceptance**
- [ ] A PR adding a vulnerable dependency fails
- [ ] A PR adding a non-allowlisted licence fails with a readable message
- [ ] The policy lives in-repo, not in repo settings

---

### NRM-109 — Branch protection and signed commits, documented as code
**Size:** S — **Depends on:** none

**Acceptance**
- [ ] `ci passed` is the sole required check; documented in `docs/ci.md`
- [ ] Signed commits required on `main`
- [ ] Settings captured in a file so drift is reviewable, even if applied by hand

---

### NRM-110 — Fuzz the pointer, patch, and diff layers
**Size:** M — **Depends on:** none

The highest-value security testing available here. These functions parse attacker-influenceable
input (an agent proposes pointers and values) and are pure, so they fuzz beautifully.

**Scope**
- `FuzzParsePointer`, `FuzzSetAtPath`, `FuzzRemoveAtPath`, `FuzzApplyPatch`, `FuzzDiffRoundTrip`
- Invariants to assert: no panic; `Set` then `Get` round-trips; `Set` never mutates its input;
  diff of a patched document reproduces the patch's effect
- Seed corpus committed; CI runs a short fuzz smoke, nightly runs longer

**Acceptance**
- [ ] Five fuzz targets, all clean for 10 minutes locally
- [ ] `make fuzz` and a nightly CI job
- [ ] Any crasher found is committed as a regression seed

---

### NRM-111 — Threat model for the control plane
**Size:** M — **Depends on:** none

**Scope**
- Trust boundaries: user ↔ agent, agent ↔ engine, engine ↔ device, device ↔ update server
- Attacker classes: malicious app, hostile network, compromised agent (prompt injection),
  physical access, malicious contributor
- For each: what stops it today, what does not, which ticket closes the gap

**Acceptance**
- [ ] `docs/security/threat-model.md` with a diagram and a gap table
- [ ] Every "does not" row links to a ticket or an accepted-risk note
- [ ] Referenced from `README.md`

---

### NRM-112 — Bound validation against adversarial input
**Size:** M — **Depends on:** NRM-110

Two concrete exposures found while writing this:

1. `url-pattern` detectors compile a user-supplied regex. Go's RE2 rules out classic catastrophic
   backtracking, but a large pattern can still consume significant memory and compile time.
2. CUE evaluation of a deeply nested or very large document is unbounded.

**Scope**
- Cap document size, nesting depth, and total node count before CUE sees the document
- Cap regex pattern length and compiled program size
- Evaluation timeout with a clean `Issue` rather than a hang
- Fixtures in the invariant corpus for each cap

**Acceptance**
- [ ] A 100MB config is rejected in bounded time with a clear issue code
- [ ] A 10,000-deep nested document is rejected, not stack-overflowed
- [ ] A pathological regex is rejected at validation, not at enforcement time
- [ ] Caps are declared in `schema/normal.cue`'s `limits` block, not hardcoded in Go

---

### NRM-113 — Agent request quotas
**Size:** S — **Depends on:** NRM-112

A compromised or looping agent should not be able to exhaust the device by proposing continuously.

**Acceptance**
- [ ] Per-session caps on proposals created and applies attempted, configurable via schema
- [ ] Exceeding a cap returns a normal `ToolResult` error, never a panic
- [ ] Applied transactions are rate-limited independently of proposals
