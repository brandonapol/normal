# M7 — Protocol & community

**Goal:** make `schema/normal.cue` the stable, documented surface other people build on — the thing
that gets forked and extended, the way omarchy's dotfiles did.

This is the "community lock-in" objective. It is mostly not code.

---

### NRM-601 — Schema versioning and migration framework
**Size:** L — **Depends on:** none

`apiVersion` is immutable per document by design, which forces version changes through explicit
migration. The migration machinery does not exist yet.

**Scope**
- Migration as a pure function `(document at vN) → (document at vN+1)`, testable in isolation
- Registry of migrations, applied in sequence, audited when run on-device
- Corpus of real v0 documents that must survive migration to v1
- Deprecation policy: how long a version stays supported

**Acceptance**
- [ ] A v0 → v1 migration exists as a worked example, even if v1 is otherwise trivial
- [ ] Migrations are pure and property-tested: migrating twice equals migrating once
- [ ] The invariant corpus is migrated alongside, proving invariants survive version changes
- [ ] Policy documented in `docs/config-schema.md`

---

### NRM-602 — Conformance suite
**Size:** M — **Depends on:** NRM-601

If the schema is a protocol, an independent implementation must be able to prove it conforms.

**Scope**
- Language-agnostic test corpus: documents plus expected issue codes, as data
- The existing invariant corpus is the seed; generalise its manifest into the published format
- A runner contract others can implement in any language

**Acceptance**
- [ ] Corpus published in a form with no Go dependency
- [ ] Our own validator is verified against it, using the same harness an outsider would
- [ ] Documented process for proposing new conformance cases

---

### NRM-603 — Publish the schema as a versioned CUE module
**Size:** S — **Depends on:** NRM-601

**Acceptance**
- [ ] `schema/` published as a consumable CUE module with semantic versioning
- [ ] Third parties can `cue vet` their config without cloning this repo
- [ ] Releases tied to `apiVersion`

---

### NRM-604 — Contribution infrastructure
**Size:** S — **Depends on:** none

**Acceptance**
- [ ] `CONTRIBUTING.md` covering the ticket format, `make ci`, and the invariant corpus rules
- [ ] `CODEOWNERS`, PR template, issue templates
- [ ] ADR directory with the decisions already made recorded retroactively: Go over TypeScript, CUE
      as protocol, layered install over fork, value semantics over pointer performance
- [ ] Stated rule that loosening a product invariant needs an ADR, not just a passing build

---

### NRM-605 — Documentation site
**Size:** M — **Depends on:** NRM-603

**Acceptance**
- [ ] Generated from `docs/`, published on every `main` merge
- [ ] Schema reference generated from the CUE, so it cannot drift from the protocol
- [ ] The agent tool contract is published as a page, since it is the other public surface

---

### NRM-606 — Reference client
**Size:** M — **Depends on:** NRM-203, NRM-602

Proves the protocol is genuinely reusable rather than merely documented.

**Scope**
- A small config editor built only against the published schema and the mobile facade
- Written as if by an outside contributor: no privileged access to internals

**Acceptance**
- [ ] Edits a config, validates it, previews a plan, and renders a diff
- [ ] Depends only on published artifacts
- [ ] Every friction encountered becomes a ticket against the protocol, not a local workaround
