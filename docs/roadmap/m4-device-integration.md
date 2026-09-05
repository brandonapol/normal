# M4 — Device integration

**Goal:** replace the in-memory ports with real ones, on a real Pixel running stock GrapheneOS, as a
layered install rather than a fork. The engine does not change; only the ports do. If that turns out
to be false, the port abstraction was wrong and this milestone tells us early.

---

### NRM-301 — Real filesystem port
**Size:** M — **Depends on:** NRM-205

**Scope**
- Atomic writes: temp file, fsync, rename, fsync parent — the engine's rollback correctness depends
  on a write either landing or not
- Correct ownership, mode, and SELinux context per file
- Faithful `IOError` codes so the engine's existing rollback logic keeps working unchanged

**Acceptance**
- [ ] Passes the same test suite as the in-memory port, plus fault injection
- [ ] Power-cut during a write leaves either the old or new file, never a truncated one
- [ ] A read-only or full filesystem produces the right `IOError` code, not a generic failure
- [ ] No test in `pkg/engine` needs modifying to accommodate it

---

### NRM-302 — Real service host port
**Size:** M — **Depends on:** NRM-205

**Scope**
- Start, stop, restart, and status against Android init or an app-level service manager
- Health check semantics matching `ServiceRunning`, including "started but crash-looping"
- Timeouts: a service that never comes up must fail the transaction, not hang it

**Acceptance**
- [ ] A service that fails to start triggers engine rollback, verified on-device
- [ ] A crash-looping service reports unhealthy rather than running
- [ ] Restart storms are bounded

---

### NRM-303 — Privileged helper and authorization model
**Size:** L — **Depends on:** NRM-301, NRM-302

The agent runs unprivileged. Something privileged writes `/etc/normal`. That boundary needs its own
authorization model, not just Unix permissions.

**Scope**
- Helper accepts a *plan*, not arbitrary writes — it re-validates against the schema itself and
  refuses anything the engine would not have produced
- Caller identity checked; only the config service may submit plans
- Path allowlist: the helper cannot be induced to write outside `/etc/normal`
- Every accepted plan is audited (NRM-121)

**Acceptance**
- [ ] The helper rejects a plan that does not validate, even from an authorized caller
- [ ] The helper rejects writes outside its allowlist, with a fixture proving it
- [ ] An unauthorized caller is refused and the attempt is logged
- [ ] Threat-model entry in `docs/security/threat-model.md`

---

### NRM-304 — Layered install on stock GrapheneOS
**Size:** L — **Depends on:** NRM-303

The `docs/base-os.md` recommendation, made real: no fork, no rebase treadmill.

**Scope**
- System app bundle containing daemon, helper, and launcher
- Install, upgrade, and uninstall paths; uninstall must leave a working phone
- Document exactly which guarantees hold in a layered install and which need the image

**Acceptance**
- [ ] Installs on a stock GrapheneOS Pixel and survives a reboot
- [ ] Uninstall restores the stock launcher and leaves no orphaned services
- [ ] An OS update does not break the install, or fails loudly if it does
- [ ] `docs/base-os.md` claim table updated with measured reality

---

### NRM-305 — On-device integration test harness
**Size:** L — **Depends on:** NRM-301, NRM-302

**Scope**
- Android emulator in CI running the full apply/rollback suite against real ports
- Fault injection at the device layer: full disk, read-only mount, killed service
- Nightly rather than per-PR if runtime demands it

**Acceptance**
- [ ] The engine's transaction tests run green against real ports in CI
- [ ] Rollback and recovery paths are exercised, not just the happy path
- [ ] A failure produces logs sufficient to diagnose without a physical device

---

### NRM-306 — Config storage, migration, and reset semantics
**Size:** M — **Depends on:** NRM-124

**Acceptance**
- [ ] Config location and backup policy documented
- [ ] Schema migration runs at boot when `apiVersion` advances, and is audited
- [ ] Factory reset returns to the sealed baseline (NRM-124)
- [ ] User data and config are separable: resetting config does not wipe photos

---

### NRM-307 — Observability
**Size:** M — **Depends on:** NRM-121

**Scope**
- Structured logs from the daemon, on-device only by default
- Health surface: current revision, last transaction, verification status
- Explicit decision on whether anything leaves the device — the default should be no

**Acceptance**
- [ ] `normalctl status` reports revision, health, and last transaction
- [ ] No telemetry leaves the device without an explicit, revocable opt-in
- [ ] Logs redact anything that could identify apps or usage by default
