# M6 — Scroll enforcement

**Goal:** make the product claim true on a real device. The approaches are analysed in
[`docs/infinite-scroll.md`](../infinite-scroll.md); this milestone builds them.

The honest framing: on a layered install the constraint is as strong as the user's commitment to
leaving the enforcement enabled. Only the framework-level approach makes it a property of the OS.
NRM-504 is a spike toward that, not a commitment to fork.

Can start before M4 finishes — NRM-501 and NRM-503 are both testable off-device.

---

### NRM-501 — WebView and browser shim
**Size:** L — **Depends on:** none

Approach 1. Highest value per unit of effort: it covers the feeds people actually lose hours to, and
needs no per-site knowledge.

**Scope**
- Content script injected at document-start, driven by the rendered
  `/etc/normal/generated/webview-shim.json` the engine already produces
- Wrap `IntersectionObserver` so sentinels stop firing past `maxAutoLoads`
- Detect append-on-scroll via `MutationObserver` correlated with scroll position
- Cap document height growth at `maxDocumentHeightMultiplier`; insert the continuation gate at the cap
- Disable `history.scrollRestoration` so backing out of an article does not restore into a feed
- Honour per-app overrides and exemptions

**Acceptance**
- [ ] Test corpus of real feed pages, captured offline, where enforcement demonstrably engages
- [ ] A paginated site and a long article are *not* gated — false positives are the failure mode
      that gets the feature disabled by users
- [ ] Shim reads config from the rendered file; changing `pageSize` changes behaviour with no rebuild
- [ ] Measured page-load overhead within a stated budget

---

### NRM-502 — Accessibility-service enforcement
**Size:** L — **Depends on:** NRM-304

Approach 2. The only native-app option that does not require owning the image.

**Scope**
- `AccessibilityService` watching `TYPE_VIEW_SCROLLED`, correlating item count growth against page
  boundaries
- Continuation gate as an overlay; must not cover system UI or trap the user
- Per-app tuning driven by the surface registry (NRM-503)
- Battery cost measured, not assumed

**Acceptance**
- [ ] Enforcement engages on at least three major feed apps
- [ ] Reading, maps, and chat surfaces are not gated
- [ ] Measured battery impact within a stated budget
- [ ] Revoking the permission degrades gracefully and says what was lost
- [ ] Documentation states plainly that the user can disable this

---

### NRM-503 — Community surface registry
**Size:** M — **Depends on:** none

Approach 4, and the community lock-in surface. The piece outsiders can contribute to earliest and
most easily: classifying an app's screens is a pull request, not a kernel patch.

**Scope**
- Separate versioned repo of `package + surface → classification` entries
- Entries written in CUE and checked against `#SurfaceDetector` in the registry's own CI, so a
  contributor's entry is validated at review time without running the OS
- Signed, versioned releases consumed by the device config
- Contribution guide with the evidence expected for a classification

**Acceptance**
- [ ] Registry repo with schema, CI validation, and ten seed entries
- [ ] A malformed contribution fails CI with a readable message
- [ ] Device consumes a signed registry release and applies it through the normal engine path
- [ ] Rejecting a bad entry is a documented process, not an argument

---

### NRM-504 — Framework-level scroll budget (spike)
**Size:** XL — **Depends on:** owning a signed image — **Status:** spike only

Approach 3. Time-boxed investigation, explicitly not a commitment.

**Scope**
- Prototype a per-app scroll budget in input dispatch, converting flings to bounded scrolls past a
  threshold
- Measure jank and battery; input-path regressions are viscerally felt
- Confirm the assumption that widget-level patching is useless because apps bundle their own
  AndroidX and Compose

**Acceptance**
- [ ] Written findings: feasible or not, with measurements
- [ ] A recommendation on whether the fork is worth it, with the rebase cost priced in
- [ ] Time-boxed; an inconclusive spike that reports honestly is a success

---

### NRM-505 — False-positive reporting and effectiveness measurement
**Size:** M — **Depends on:** NRM-501, NRM-502

False positives are what get this feature switched off. They need a feedback path that does not
compromise the privacy stance.

**Scope**
- On-device, local-only record of gate events and user overrides
- One-tap "this was not a feed" that produces a shareable registry contribution the user reviews
  before sending
- Nothing leaves the device without explicit per-report consent

**Acceptance**
- [ ] User can see where enforcement engaged and correct it
- [ ] A correction produces a well-formed registry PR the user explicitly chooses to share
- [ ] Zero automatic transmission; verified by test

---

### NRM-506 — Exemption lifecycle UX
**Size:** S — **Depends on:** NRM-403

The schema caps exemptions at three, requires a written reason, and expires them within 30 days.
Users need to experience that as reasonable rather than arbitrary.

**Acceptance**
- [ ] Active exemptions are visible with their reason and expiry
- [ ] Expiry is surfaced before it happens, not as a surprise
- [ ] Requesting a fourth exemption explains the cap and offers to replace one
- [ ] Renewal is a new decision with a fresh reason, not a one-tap extension
