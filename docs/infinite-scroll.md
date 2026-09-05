# Enforcing "no infinite scroll"

This is the hardest and most novel piece, and the one place where the base-OS decision actually
binds. The schema already encodes the policy; this document is about what enforces it on a real
device. Nothing here is built yet.

The problem has two halves that need different machinery: **web content**, where you control the
runtime, and **native Android apps**, where you do not.

## Approach 1 — WebView and browser shim (web content)

Inject a content script at document-start into every WebView and browser tab, before page script
runs. It can:

- Wrap `IntersectionObserver` so sentinel elements near the scroll bottom never fire a callback
  after `maxAutoLoads` — the single most common auto-load trigger on the modern web.
- Watch for the append-on-scroll signature: a `MutationObserver` seeing child nodes appended to a
  scroll container within ~500ms of a scroll event that approached the bottom.
- Cap document height growth at `maxDocumentHeightMultiplier × viewport-height-at-load`, and insert
  the continuation gate at the cap.
- Disable `history.scrollRestoration` so backing out of an article does not restore you into a feed
  at depth.
- Neutralise scroll-anchoring tricks used to keep users pinned in an updating list.

**Strong**: it is deterministic, needs no per-site knowledge, and works on the majority of feeds
people actually lose hours to. **Weak**: sites can detect a patched `IntersectionObserver`; some
apps ship their own WebView (the shim must be enforced at the system WebView provider, which means
controlling the WebView package); virtualised lists that recycle DOM nodes never grow document
height and evade the height cap.

Maturity: buildable today. This is where to start.

## Approach 2 — Accessibility service heuristics (native apps, no fork)

An `AccessibilityService` observes `TYPE_VIEW_SCROLLED` events with `itemCount` and
`fromIndex`/`toIndex`, plus window content changes. The endless-list signature is a collection whose
`itemCount` keeps growing across scroll events with no page boundary ever crossed. On detection,
draw a full-screen overlay carrying the continuation gate.

**Strong**: works on unmodified third-party apps, needs no OS fork, and can ship as an installable
layer on stock GrapheneOS. This is the only native-app option that does not require owning the
image. **Weak**: the user can revoke it in settings — so it is a *user preference*, not an OS
invariant; it is per-app fragile (`itemCount` is often `-1` in Compose and custom views); it costs
battery; and an overlay is a blunt instrument that can cover things it should not.

Maturity: buildable today, with real per-app tuning cost. Correct as a bridge, insufficient as the
final answer, because the product claim is "enforced at the OS level" and an app the user can
disable in Settings is not that.

## Approach 3 — Framework-level scroll budget (native apps, requires the image)

Patch input dispatch in the framework: give each app a scroll budget, measured in viewport-heights
travelled within one foreground session, and once exhausted, convert flings into short bounded
scrolls and surface the gate. Enforced below the app, in `InputDispatcher`/`ViewRootImpl` rather
than in any widget.

**Strong**: universal, unbypassable, app-agnostic, and cheap at runtime. It cannot be revoked from
Settings because it is not a service. **Weak**: it needs your own signed OS image, so it is
downstream of the base-OS decision; it is blunt — a long document, a map, and a chat backlog all
scroll a lot legitimately, so it needs the surface registry below to avoid punishing reading; and
patching input dispatch is a place where bugs are viscerally felt.

Note what will *not* work: patching `RecyclerView` or `ListView` in the framework. Apps bundle
their own AndroidX and Compose, so framework widget code is not what they run. The enforcement point
has to be below the widget layer — input, or the compositor.

Maturity: needs the fork. This is the endgame for the hard constraint.

## Approach 4 — Community surface registry (accuracy layer for all of the above)

A signed, versioned registry mapping `package + surface` (activity, view id, route) to a
classification: feed, reading surface, map, chat. The schema already has the shape for this —
`app-surface` detectors and per-app `attention` overrides.

This is also the community lock-in surface. It is exactly the artifact other people can contribute
to without touching the OS: an entry saying "this activity in this app is a feed, this one is a
reading view" is a pull request, not a kernel patch. Ship it as a separate versioned repo consumed
by the config, the way ad-blocking filter lists work — and note that filter lists are the proof this
model sustains a community for a decade.

Writing the registry entries in CUE rather than JSON would let contributors' entries be checked
against `#SurfaceDetector` at review time, in CI, without anyone running the OS.

It reduces false positives for approaches 1–3 rather than enforcing anything itself.

## Approach 5 — Network interception (rejected)

Block known pagination endpoints via a local VPN or per-app firewall. Rejected: endpoints are not
distinguishable from ordinary API traffic, TLS makes body inspection hostile, apps break in
confusing ways rather than degrading, and every app update can silently defeat it.

## Recommendation

Ship 1 + 2 + 4 first, as an installable layer, which validates the whole product on stock hardware
without owning a build. Add 3 when you own the image, and demote 2 to a fallback for apps the
input-budget approach handles badly.

Be honest in the product about the difference: on the layered install, the constraint is as strong
as the user's own commitment to leaving the accessibility service on. Only the framework-level
version makes "no infinite scroll" a property of the OS rather than a setting. That is a real
argument for eventually taking on the fork — and worth being clear-eyed about before promising it.
