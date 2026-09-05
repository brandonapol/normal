# Architecture

## Why three packages

The split is a dependency rule, not filing. `schema <- engine <- agent-tools`, never the reverse.

**`@normal/schema`** is the protocol. Types, a runtime validator, JSON-pointer utilities, and the
baseline config. It has no dependencies at all — not on the engine, not on Node, not on Android.
Anything that wants to read or write a Normal config, in any tool, depends only on this. If the
community forks one thing, it should be this package, and it should be boring and stable.

**`@normal/engine`** turns two configs into a transaction. Everything except `apply.ts` is pure:
`diffConfig`, `renderConfig`, `planApply` are functions of their arguments with no I/O. I/O happens
through four injected ports (`FileSystemPort`, `ServiceHostPort`, `ClockPort`, `LoggerPort`), which
is why the whole engine tests on a dev machine.

**`@normal/agent-tools`** is the agent's cage. Tool definitions, argument parsing, a policy layer,
and a session that owns proposal state and revision history. The agent gets tools; it does not get
ports.

## The pipeline

```
patch operations
      |  applyPatch          pure, immutable, JSON-pointer based
      v
candidate config
      |  validateConfig      pure, returns every issue at once
      v
valid config
      |  planApply           pure: diff -> rendered files -> file writes + service restarts
      v
Plan
      |  applyPlan           the only effectful step
      v
ApplyReport | ApplyFailure
```

Each arrow is a `Result<T, E>`. No throwing, no partial state escaping a failure.

## How a config becomes files

`renderConfig` maps one config document to a `FileSet`:

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

Output is stable-stringified with sorted keys, so byte-identical configs produce byte-identical
files and the file diff is trustworthy.

## Transactions

`applyPlan` captures the current contents of every file the plan touches, then executes writes,
then deletes, then restarts, in that order. After the restarts it verifies each affected service
reports `running`. Any failure — a write, a restart, or a service that comes back unhealthy —
triggers a rollback: restore every captured file, then restart every affected service.

If the rollback itself fails, the result says so: `rolledBack: false`, `deviceDirty: true`, and the
list of rollback errors. The engine never claims a clean recovery it did not achieve. That case
needs recovery-mode handling on real hardware, which is not built yet.

## Keyed collections

Arrays of objects with a natural key (`/spec/apps/entries` keyed by `package`, most others by `id`)
are addressed by key rather than index: `/spec/apps/entries/com.spotify.music/network`. Pointers
stay valid when a list is reordered, a diff of a reordered list is one `$order` change rather than N
replacements, and an agent can write a stable path without first counting array positions. The
registry lives in `schema/src/keys.ts`.

## Revisions

Every applied change advances `/metadata/revision` by one, and the engine refuses a plan whose
target revision does not advance. The agent cannot write that field. Rollback is not special-cased:
it is a proposal whose desired state happens to be an earlier revision's config, planned and applied
through the same path, and it produces a *new* revision rather than rewinding history.
