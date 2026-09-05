# Architecture

## Why the packages split this way

The split is a dependency rule, not filing. `schema <- config <- engine <- agent`, never the
reverse.

**`schema/`** is the protocol, written in CUE and embedded in the binary with `go:embed`. It is the
one artifact the community forks and extends, and it is deliberately not Go: a Rust or Kotlin or
Python implementation of Normal reads the same `normal.cue`.

**`pkg/config/`** is the Go binding for that protocol: structs with JSON tags, JSON-pointer
utilities, and validation. It imports `schema` and nothing else of ours.

**`pkg/engine/`** turns two configs into a transaction. Everything except `apply.go` is pure:
`DiffConfigs`, `Render`, and `PlanApply` are functions of their arguments with no I/O. I/O happens
through four injected ports (`FileSystem`, `ServiceHost`, `Clock`, `Logger`), which is why the whole
engine tests on a dev machine.

**`pkg/agent/`** is the agent's cage. Tool definitions, argument parsing, a policy layer, and a
session that owns proposal state and revision history. The agent gets tools; it does not get ports.

## Where each constraint lives

The rule of thumb: CUE gets anything expressible as a type, Go gets anything needing context.

| constraint | enforced by | why |
| --- | --- | --- |
| field shapes, enums, required fields | CUE | that is what a type system is |
| numeric bounds (`columns: 1-8`, `pageSize: 5-100`) | CUE | expressible inline, and readable |
| collection caps (`list.MaxItems`) | CUE | ditto |
| no `off` enforcement mode | CUE | absence from a disjunction is stronger than a check |
| detectors cannot be emptied | CUE | `[#Detector, ...#Detector]` is a non-empty list type |
| webview shim cannot be disabled | CUE | `injectShim: true` — the field admits one value |
| exemption reason length | CUE | `strings.MinRunes(limits.minExemptionReasonLength)` |
| exemption expiry in the future, within 30 days | Go | needs `now`, which a schema does not have |
| dangling package references | Go | cross-section reachability, awkward in CUE |
| duplicate ids within a collection | Go | same |
| a notification rule matching nothing | Go | "at least one optional field set" |
| page item count vs the config's own `maxItemsPerPage` | Go | one field constraining another |

The numeric limits are declared once, in CUE, and Go reads them back out
(`config.SchemaLimits()`) rather than redeclaring them. Change `maxExemptions` in `normal.cue` and
the validator, the agent's guidance text, and the docs tool all follow.

**The cost:** embedding the CUE evaluator puts the `normalctl` binary at roughly 23MB. That is fine
for a dev tool and acceptable for a phone daemon, but if it ever isn't, the escape hatch is to
generate a Go validator from the CUE at build time and keep CUE as the spec plus a differential test
oracle. Worth knowing the exit exists; not worth taking yet.

## The pipeline

```
patch operations
      |  ApplyPatch          pure, immutable, JSON-pointer based
      v
candidate document
      |  config.Validate     CUE structural pass, then Go semantic pass
      v
valid config
      |  engine.PlanApply    pure: diff -> rendered files -> writes + restarts
      v
Plan
      |  engine.ApplyPlan    the only effectful step
      v
Report | *Failure
```

Structural validation runs first and returns early: if the shape is wrong, the semantic checks would
only produce noise on top of it.

## How a config becomes files

`Render` maps one config document to a `FileSet`:

| file | rendered from | read by |
| --- | --- | --- |
| `/etc/normal/metadata.json` | `/metadata` | (nothing) |
| `/etc/normal/launcher.json` | `/spec/launcher` | `normal-launcher` |
| `/etc/normal/apps.json` | `/spec/apps` | `normal-appd`, `normal-launcher` |
| `/etc/normal/notifications.json` | `/spec/notifications` | `normal-notifyd` |
| `/etc/normal/attention.json` | `/spec/attention` | `normal-attentiond` |
| `/etc/normal/generated/webview-shim.json` | `/spec/attention` + `/spec/apps` | `normal-webview-shim` |

Two consequences fall out of that table. Restarts are derived, not declared: a service restarts
exactly when a file it reads changes, so renaming a Spotify label restarts `normal-appd` and the
launcher but not the shim. And one config change can fan out to several files — the shim config is
derived from both the attention policy and per-app overrides.

Output is encoded with sorted keys, so byte-identical configs produce byte-identical files and the
file diff is trustworthy.

## Transactions

`ApplyPlan` captures the current contents of every file the plan touches, then executes writes,
then deletes, then restarts, in that order. After the restarts it verifies each affected service
reports `running`. Any failure — a write, a restart, or a service that comes back unhealthy —
triggers a rollback: restore every captured file, then restart every affected service.

If the rollback itself fails, the result says so: `RolledBack: false`, `DeviceDirty: true`, and the
list of rollback errors. The engine never claims a clean recovery it did not achieve. That case
needs recovery-mode handling on real hardware, which is not built yet.

`*Failure` implements `error`, so an apply is `report, err := ApplyPlan(...)` like any other Go
call, and callers who want the structure type-assert for it.

## Keyed collections

Arrays of objects with a natural key (`/spec/apps/entries` keyed by `package`, most others by `id`)
are addressed by key rather than index: `/spec/apps/entries/com.spotify.music/network`. Pointers
stay valid when a list is reordered, a diff of a reordered list is one `$order` change rather than N
replacements, and an agent can write a stable path without first counting array positions. The
registry lives in `pkg/config/keys.go`.

## Revisions

Every applied change advances `/metadata/revision` by one, and the engine refuses a plan whose
target revision does not advance. The agent cannot write that field. Rollback is not special-cased:
it is a proposal whose desired state happens to be an earlier revision's config, planned and applied
through the same path, and it produces a *new* revision rather than rewinding history.
