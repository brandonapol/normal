# M2 — Config integrity & recovery

**Goal:** the config is the device's security policy — app permissions, network access, attention
enforcement. It needs the integrity properties that implies, and a story for the failure the engine
currently reports honestly but cannot fix.

---

### NRM-121 — Hash-chained audit log
**Size:** M — **Depends on:** none

Every applied transaction already produces a `Report`. Nothing persists it, and nothing makes it
tamper-evident.

**Scope**
- Append-only log: each entry carries the previous entry's hash, the config digest before and after,
  the plan, the intent, and who approved
- Verification command: `normalctl audit verify`
- Log survives the transaction it records — written before services restart, sealed after

**Acceptance**
- [ ] Every applied transaction appends exactly one entry
- [ ] Editing or removing any entry makes verification fail, naming the first broken link
- [ ] A crash mid-apply leaves a log that verification reports as incomplete, not corrupt
- [ ] `normalctl audit log` renders history readably

---

### NRM-122 — Signed config revisions
**Size:** L — **Depends on:** NRM-121

**Scope**
- Device-local signing key in hardware-backed storage where available
- Every revision signed at apply time, recording author (user or agent) and approver
- `normalctl verify` checks the chain from the sealed baseline to the current revision
- Signature covers the rendered `FileSet`, not just the document, so drift is detectable

**Acceptance**
- [ ] A revision applied outside the engine fails verification
- [ ] Hand-edited `/etc/normal/*.json` is detected on next boot
- [ ] Key never leaves secure storage; signing is an operation, not an export
- [ ] Verification works offline

---

### NRM-123 — Anti-rollback for security-relevant fields
**Size:** M — **Depends on:** NRM-122

Rollback is a feature — but rolling back to a revision that granted an app microphone access, or
weakened attention enforcement, is a downgrade attack wearing a feature's clothes.

**Scope**
- Classify fields: freely reversible (launcher layout) vs. security-relevant (permissions, network,
  app state, everything under `/spec/attention`)
- A rollback that would weaken a security-relevant field requires the same explicit approval as
  making that change forward, and says exactly which fields regress
- Monotonic counter so a rollback cannot silently replay an old signed revision

**Acceptance**
- [ ] Rolling back a launcher change needs no extra confirmation
- [ ] Rolling back a permission grant lists the regressing fields before asking
- [ ] Replaying an old signed revision without going through the engine is rejected
- [ ] Fixtures cover both classifications

---

### NRM-124 — Sealed baseline and factory reset
**Size:** M — **Depends on:** NRM-122

**Scope**
- Immutable, signed baseline shipped with the image, never writable at runtime
- `normalctl reset` returns to it and starts a fresh revision chain
- Baseline satisfies every invariant by construction; CI proves it

**Acceptance**
- [ ] Baseline is verified at boot before any user revision is applied
- [ ] Reset produces a device in a known-good state with an auditable transition
- [ ] A corrupted user config chain falls back to the baseline rather than failing to boot

---

### NRM-125 — Recovery mode for a dirty device
**Size:** L — **Depends on:** NRM-121, NRM-124

Closes the gap the engine currently reports honestly: when rollback itself fails, `DeviceDirty` is
set and nothing resolves it.

**Scope**
- Persist the intended and captured state before the first write, so recovery has something to work
  from after a reboot
- Boot-time detector for an incomplete transaction; re-attempt rollback, then fall back to baseline
- Recovery attempts are themselves audited

**Acceptance**
- [ ] A device power-cut mid-apply boots into a consistent, known revision
- [ ] Recovery path is exercised by an integration test that kills the process mid-transaction
- [ ] A device that cannot recover says so on screen rather than boot-looping
- [ ] `DeviceDirty` becomes rare and actionable rather than terminal

---

### NRM-126 — Config secrets handling
**Size:** M — **Depends on:** NRM-122

v0 has no secret-bearing fields, and that is worth keeping deliberate rather than accidental.

**Scope**
- Decide and document: does any future field carry a credential (wifi PSK, sync token)?
- If yes, a `#Secret` type that is referenced but never rendered into world-readable files
- Validator rejects a plaintext secret in a field not typed as one

**Acceptance**
- [ ] `docs/security/secrets.md` states the policy
- [ ] A fixture proves a plaintext-looking secret in an ordinary field is rejected
- [ ] Rendered files under `/etc/normal` are safe to read by any service that reads them today
