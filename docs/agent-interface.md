# The agent interface

The on-device agent's entire surface is nine tools. It has no filesystem, no shell, and no network.
This document is the contract; `pkg/agent/tools.go` is the machine-readable version.

## Tools

**Read.** `get_config(section?)`, `describe_schema(path?)`, `list_apps()`, `list_revisions()`.

**Propose.** `propose_change(intent, operations)` takes patch operations (`{op: "set" | "remove",
path, value?}`) against JSON pointers, and `propose_rollback(revision)` targets an earlier applied
revision. Both return a proposal id, a rendered diff, the services that would restart, and any
policy review items. Neither touches the device.

**Act.** `preview_proposal(proposalId)`, `apply_proposal(proposalId)`, `discard_proposal(proposalId)`.

Note what is absent: there is no `approve_proposal`. Approval is `session.Approve(id, actor)`, a
method on the session that no tool call can reach. The agent can want a change; only a human can
consent to one. A test asserts no tool name contains "approve".

## Guarantees

1. **No raw filesystem access.** The agent's paths are config pointers, not file paths. It cannot
   name `/etc/normal/launcher.json`, let alone `/etc/passwd`.
2. **Writable roots.** Operations are confined to `/spec`, `/metadata/description`, and
   `/metadata/labels`. `/apiVersion`, `/kind`, `/metadata/name`, and `/metadata/revision` are
   engine-managed and rejected with `engine-managed-field`.
3. **Validation before planning.** A patch that produces an invalid config is rejected before a plan
   exists, so a malformed proposal never becomes a file write.
4. **Approval gates.** Sensitive paths — anything under `/spec/attention`, plus per-app
   `permissions`, `network`, `state`, and the app `policy` — always require approval. Sessions
   default to requiring approval for everything; `ApprovalRequiredForEverything: false` narrows that
   to sensitive paths only, and no setting can make an attention change auto-applicable.
5. **Weakening detection.** The policy layer compares before and after semantically, not
   syntactically. Attention changes — dropping enforcement strength, raising `maxAutoLoads` or
   `pageSize`, adding an exemption, stopping IntersectionObserver interception, raising a daily
   budget — produce a `weakens-attention-policy` item. App changes that widen access — a permission
   moving toward `allow`, network moving toward `allow`, an app going from `blocked` to `installed`,
   or the app policy loosening from allowlist to denylist — produce a `security-regression` item
   naming the field and what it changes from and to.

   This applies to rollbacks as much as to forward changes, which is the point: rolling back to a
   revision that granted an app the microphone is a downgrade attack wearing a feature's clothes.
   A rollback that only touches cosmetics needs no extra confirmation; one that re-opens access
   lists exactly which fields regress before asking.
6. **Staleness.** A proposal is re-evaluated against the *current* config at apply time, not the
   config it was written against. If the world moved underneath it, the apply fails with `stale`
   rather than applying a diff computed against a config that no longer exists.
7. **Bounded blast radius.** At most 64 operations per proposal, and per-session quotas on
   proposals created and applies attempted, both declared in the schema's `limits` block. The apply
   quota is deliberately tighter than the proposal quota — thinking is cheap, changing the device is
   not. Exceeding either returns an ordinary `ToolResult` error. Every tool returns a `ToolResult`
   value; `Dispatch` never panics and never returns a bare Go error, including for unknown tool
   names.
8. **Bounded validation.** Document size, nesting depth, and node count are capped before the schema
   evaluator runs, and a detector's regex is length-capped before it is compiled, so a hostile
   proposal cannot spend unbounded time or memory. See `docs/config-schema.md`.

## What the agent sees

Diffs exclude `/metadata/revision` — the revision bump is bookkeeping and would be noise in every
single review. Errors carry a `code` and a message written for a human reading it over the agent's
shoulder: "the result would not be a valid config: at least one detector is required; enforcement
cannot be disabled by emptying this list."

`SystemGuidance()` in `tools.go` is the prompt fragment that tells the agent the workflow and the
attention invariant, including what to offer when a user asks to turn scroll blocking off: a
bounded, expiring exemption for one app, not a switch. It reads its numbers from the schema, so it
cannot describe limits the validator does not actually enforce.

## Wiring a model

Not done yet, deliberately. `ToolDefinitions()` carries JSON Schema `inputSchema` objects that map
directly onto tool-use APIs, and `Dispatch(ctx, session, ToolCall{...})` is the whole executor. An
adapter is roughly: send `SystemGuidance()` plus `ToolDefinitions()`, route each tool call through
`Dispatch`, feed results back, and surface `RequiresApproval` proposals to the user for a real
confirmation before calling `session.Approve`.

The model choice is deliberately not baked in. On-device inference will constrain it, and the tool
surface is small and typed specifically so a weaker local model can drive it safely: the boundary
does not trust the model's judgment about what is safe.
