# Config schema v0

`apiVersion: normal.os/v0`, `kind: PhoneConfig`. The canonical definition is `schema/normal.cue`;
this is the prose version, and `pkg/config/types.go` is the Go binding. A complete valid document is
in `examples/baseline.config.json` (regenerate with `go run ./cmd/normalctl baseline`).

```yaml
apiVersion: normal.os/v0
kind: PhoneConfig
metadata:
  name: normal-baseline
  revision: 0
spec:
  launcher: { ... }
  apps: { ... }
  notifications: { ... }
  attention: { ... }
```

`apiVersion` and `kind` are immutable: a mutation cannot change them, which forces version changes
through an explicit migration instead of an in-place edit. `metadata.revision` is engine-managed.

## `/spec/launcher`

`layout` (`grid` | `list`), `columns` (1–8), `maxItemsPerPage`, `pages`, `dock` (≤5 packages),
`appDrawer`, `gestures`.

A page holds `items`, each an `app`, a `shortcut` (open an app, open a URL, or toggle
flashlight/DND/wifi/airplane/grayscale), or a `widget`. Pages and items are keyed by `id`. The
validator rejects a page holding more items than the config's own `maxItemsPerPage` — the anti-
clutter constraint is enforced, not advisory.

The baseline is a single-column list of four apps. That is an opinion, and it is meant to be one.

## `/spec/apps`

`policy` is `allowlist` or `denylist`; `entries` is keyed by `package`.

Each entry carries `source` (`system` | `fdroid` | `play-compat` | `sideload`), `state`
(`installed` | `blocked` | `absent`), `network` (`allow` | `wifi-only` | `deny`), a `permissions`
map (`allow` | `ask` | `deny` per permission), an optional `sandboxProfile`, and an optional
`attention` override.

Every package referenced anywhere else in the document — the dock, a launcher item, a gesture, a
quiet-hours breakthrough, a notification rule, a scroll exemption, a session budget — must exist
here with `state: installed`. Dangling references are validation errors, so you cannot end up with a
dock icon for an app that is not on the device.

## `/spec/notifications`

`defaultDisposition` is one of `allow`, `silence`, `bundle`, `block`, and the baseline default is
`bundle` — batched delivery is the default posture, not interruption.

`bundling.deliveryWindows` are `HH:MM` times; enabling bundling with no windows is an error.
`quietHours` windows carry days and a `breakthrough` package list. `rules` are keyed by `id`, carry
a `priority`, and must constrain at least one field — an empty match that silently swallows
everything is rejected.

## `/spec/attention`

This is the section the product exists for.

### `infiniteScroll`

| field | meaning |
| --- | --- |
| `enforcement` | `warn`, `paginate`, or `block`. **There is no `off`.** |
| `pageSize` | items delivered before the continuation gate (5–100) |
| `maxAutoLoads` | automatic loads before the gate; capped at 3, baseline 0 |
| `continuation` | `tap`, `tap-with-delay`, `hold`, or `passphrase` |
| `continuationDelaySeconds` | friction on the gate |
| `detectors` | how endless lists are recognised; **cannot be empty** |
| `exemptions` | bounded escapes; at most 3 |
| `webview` | shim behavior; `injectShim: false` is a validation error |

Detectors come in four kinds: `dom-heuristic` (append-on-scroll mutations, sentinel
IntersectionObservers, unbounded scroll height, absent pagination controls),
`accessibility-heuristic` (unbounded collection item counts, RecyclerView refills, scroll events
without a page boundary), `url-pattern`, and `app-surface` (a specific package and surface).
See `docs/infinite-scroll.md` for what these mean in practice.

An exemption needs a `package`, a written `reason` of at least 12 characters, and an `expiresAt`
that is in the future and within 30 days. Validation takes `now` as an argument rather than reading
the clock, so it stays pure and testable — and so an expired exemption fails validation on the next
apply rather than lingering.

The design intent: the schema has no vocabulary for "turn it off". The most permissive sentence you
can write is "this one app, for this stated reason, until this date."

### `sessionBudgets`

Per-app, per-domain, or system-wide time budgets with `dailyMinutes`, `sessionMinutes`,
`cooldownMinutes`, and `onExhausted` (`warn` | `grayscale` | `lock`). A session budget cannot exceed
its daily budget.

## How the invariants are expressed

The point of writing the schema in CUE is that the product invariants are *types*, not checks that
someone could forget to run:

```cue
#Enforcement: "warn" | "paginate" | "block"

#InfiniteScrollPolicy: {
	detectors:  [#Detector, ...#Detector]
	exemptions: [...#Exemption] & list.MaxItems(limits.maxExemptions)
	webview:    #WebViewEnforcement
}

#WebViewEnforcement: {
	injectShim: true
	...
}

#Exemption: {
	reason:    string & strings.MinRunes(limits.minExemptionReasonLength)
	expiresAt: time.Time
}
```

`#Enforcement` has no `off` member, so "off" is not a value the type admits. `detectors` is typed as
a non-empty list, so emptying it is a type error. `injectShim: true` is a field whose type is a
single value, so `false` does not unify. None of these are validator branches that a refactor could
drop.

Definitions (`#Name`) are closed in CUE, so unknown fields are rejected everywhere, at every depth,
without a single line of code.

## Validation

`config.Validate(document, now)` returns `[]Issue`, each with a JSON pointer, a stable
machine-readable `code`, and a human message. Codes are part of the protocol: tooling can branch on
`dangling-reference` or `policy-violation` without parsing prose.

Two passes. First CUE unifies the document with `#PhoneConfig` and every structural failure is
reported at once; CUE's own message is kept except where a friendlier one exists for a product
invariant. If the shape is wrong, validation stops there. Otherwise the Go semantic pass runs the
checks CUE cannot express — see the table in `docs/architecture.md`.

The `limits` block in `normal.cue` is regular data, not a definition, so the same numbers constrain
the schema *and* are readable from Go via `config.SchemaLimits()`. Nothing is declared twice.

## Extending it

Add a field: extend `normal.cue` and the matching Go struct, then add it to the ownership table in
`pkg/engine/ownership.go` so the engine knows which file it renders into and which service reads it.
An unowned path is a plan error, so you cannot ship a field the engine silently ignores.

Add a keyed collection: register it in `pkg/config/keys.go` and pointers and diffs pick it up.

Breaking changes bump `apiVersion` and ship a migration. v0 makes no compatibility promises yet.
