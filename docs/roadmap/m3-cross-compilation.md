# M3 — Cross-compilation & mobile bindings

**Goal:** the same validated core runs as an Android daemon, inside an Android app, and inside an
iOS app — without three divergent implementations.

## What actually builds today

Verified against the current tree with `CGO_ENABLED=0`:

| target | status |
| --- | --- |
| `linux/arm64` | builds (the daemon target, 15MB stripped) |
| `linux/amd64` | builds |
| `darwin/arm64` | builds |
| `android/arm64` | **builds already** |
| `windows/amd64` | builds |
| `ios/arm64` | fails: requires external (cgo) linking |

So Android is closer than it looks, and iOS needs a macOS runner with Xcode — a real CI cost line,
since macOS minutes bill at roughly ten times Linux.

## On iOS, plainly

iOS cannot host this product. No launcher replacement, no system daemons, no writes to system
config, no cross-app scroll enforcement. Anyone promising otherwise is describing a jailbreak.

What iOS *can* host is a **client**: config authoring, revision history, the agent conversation,
and — via a Safari Web Extension — the web half of scroll blocking, which is the half that covers
most feeds people actually lose hours to. That is a real, shippable product. It is not the OS, and
this roadmap does not let the two blur together.

---

### NRM-201 — Cross-compilation matrix in CI
**Size:** S — **Depends on:** none

**Acceptance**
- [ ] CI builds `linux/{amd64,arm64}`, `darwin/arm64`, `android/arm64` on every PR
- [ ] Each target's binary size is reported and a regression over budget fails (see NRM-206)
- [ ] `make build-all` reproduces the matrix locally

---

### NRM-202 — `pkg/mobile`: a bindable facade
**Size:** L — **Depends on:** none — **Blocks:** NRM-203, NRM-204

**This is the ticket that makes the other two possible, and it is not optional.**

`gomobile bind` supports a restricted type subset: basic types, `string`, `[]byte`, `error`, and
named struct/interface types with signatures drawn from that subset. It does **not** support maps,
slices of anything but bytes, `any`, or generics. Our public API is built on `map[string]any`
documents, `[]Issue`, `ToolResult{Data any}` — essentially none of it binds.

**Scope**
- A facade package whose entire surface speaks JSON strings and `[]byte` across the boundary
- Bindable operations: validate, diff, plan, apply, propose, preview, approve, list revisions
- A single opaque `Session` handle type with methods, rather than exported package functions over
  rich types
- Errors surface as an error return plus a structured JSON payload, so callers keep issue codes
- The facade is a *translation layer only* — no policy, no validation logic of its own

**Acceptance**
- [ ] `gomobile bind` succeeds against `pkg/mobile` with no type errors
- [ ] Every operation reachable from `normalctl` is reachable through the facade
- [ ] A test asserts the facade contains no decision-making: policy lives in `pkg/agent`
- [ ] Round-trip tests prove JSON-boundary fidelity for every issue code

---

### NRM-203 — Android AAR
**Size:** M — **Depends on:** NRM-202

**Scope**
- `gomobile bind -target=android` producing an AAR of `pkg/mobile`
- Minimal Kotlin sample that loads the baseline, proposes a change, and renders the diff
- Published as a CI artifact; versioned with the schema `apiVersion`

**Acceptance**
- [ ] AAR builds in CI on every PR touching `pkg/mobile`
- [ ] Sample app runs on an emulator in CI and asserts on the rendered diff
- [ ] AAR size reported against the budget

---

### NRM-204 — iOS XCFramework
**Size:** M — **Depends on:** NRM-202

**Scope**
- `gomobile bind -target=ios` on a macOS runner, producing an XCFramework
- Minimal Swift sample mirroring the Kotlin one
- Job runs only when `pkg/mobile` changes, to keep macOS minutes down

**Acceptance**
- [ ] XCFramework builds and a Swift sample exercises validate/propose/preview
- [ ] `docs/roadmap/m3` iOS scope statement is linked from the sample's README so nobody infers the
      OS ships on iOS
- [ ] Path-filtered so ordinary PRs do not pay for macOS runners

---

### NRM-205 — Android daemon build and packaging
**Size:** M — **Depends on:** NRM-201

Distinct from the AAR: this is the long-running config service, not a library in an app.

**Scope**
- `android/arm64` binary with the layered-install layout from `docs/base-os.md`
- Decide and document: system app bundle vs. privileged helper vs. init service
- Startup, config location, and permissions documented for the layered install

**Acceptance**
- [ ] Daemon binary builds in CI and starts on an emulator
- [ ] It reads and renders a config through the real filesystem port (stubbed service host)
- [ ] Install and uninstall are documented and reversible

---

### NRM-206 — Binary size budget
**Size:** S — **Depends on:** NRM-201

The embedded CUE evaluator costs roughly 15MB stripped. Acceptable for a daemon, questionable inside
an app that also ships a model.

**Acceptance**
- [ ] Per-target size budgets declared in the `Makefile`
- [ ] CI fails on a regression beyond the budget, with the delta reported
- [ ] Current sizes recorded as the starting baseline

---

### NRM-207 — `cuelite`: generated validator for size-constrained builds
**Size:** L — **Depends on:** NRM-206

The escape hatch documented in `docs/architecture.md`, built only if NRM-206 says it is needed.

**Scope**
- Generate a Go validator from `schema/normal.cue` at build time
- `cuelite` build tag swaps the runtime evaluator for the generated one
- **Differential test is the whole point:** both validators run over the entire invariant corpus and
  a fuzz corpus, and must agree on every issue — same codes, same paths

**Acceptance**
- [ ] `cuelite` builds are meaningfully smaller; the delta is recorded
- [ ] Differential test over the corpus and fuzz inputs passes with zero disagreements
- [ ] CUE remains the source of truth; the generated validator is never hand-edited
- [ ] Both variants run in CI

---

### NRM-208 — Release engineering
**Size:** M — **Depends on:** NRM-103, NRM-201

**Acceptance**
- [ ] Tagged releases produce binaries for every target, AAR, XCFramework, SBOM, and attestations
- [ ] Versioning policy documented, tying releases to schema `apiVersion`
- [ ] A release is one tag push; no manual steps
