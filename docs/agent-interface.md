# The agent interface

The on-device agent's entire surface is nine tools. It has no filesystem, no shell, and no network.
This document is the contract; `packages/agent-tools/src/tools.ts` is the machine-readable version.

## Tools

**Read.** `get_config(section?)`, `describe_schema(path?)`, `list_apps()`, `list_revisions()`.

**Propose.** `propose_change(intent, operations)` takes patch operations (`{op: "set" | "remove",
path, value?}`) against JSON pointers, and `propose_rollback(revision)` targets an earlier applied
revision. Both return a proposal id, a rendered diff, the services that would restart, and any
policy review items. Neither touches the device.

**Act.** `preview_proposal(proposalId)`, `apply_proposal(proposalId)`, `discard_proposal(proposalId)`.

Note what is absent: there is no `approve_proposal`. Approval is `session.approve(id, actor)`, a
function on the session object that no tool call can reach. The agent can want a change; only a
human can consent to one. A test asserts no tool name contains "approve".

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
   default to requiring approval for everything; `approvalRequiredForEverything: false` narrows that
   to sensitive paths only, and no setting can make an attention change auto-applicable.
5. **Weakening detection.** The policy layer compares before and after semantically, not
   syntactically: dropping enforcement strength, raising `maxAutoLoads` or `pageSize`, adding an
   exemption, stopping IntersectionObserver interception, or raising a daily budget each produce a
   `weakens-attention-policy` review item with a plain-language explanation of what got weaker.
6. **Staleness.** A proposal is re-evaluated against the *current* config at apply time, not the
   config it was written against. If the world moved underneath it, the apply fails with `stale`
   rather than applying a diff computed against a config that no longer exists.
7. **Bounded blast radius.** At most 64 operations per proposal. Every tool returns a result object;
   `dispatchTool` never throws, including for unknown tool names.

## What the agent sees

Diffs exclude `/metadata/revision` — the revision bump is bookkeeping and would be noise in every
single review. Errors carry a `code` and a message written for a human reading it over the agent's
shoulder: "the result would not be a valid config: at least one detector is required; enforcement
cannot be disabled by emptying this list."

`SYSTEM_GUIDANCE` in `tools.ts` is the prompt fragment that tells the agent the workflow and the
attention invariant, including what to offer when a user asks to turn scroll blocking off: a
bounded, expiring exemption for one app, not a switch.

## Wiring a model

Not done yet, deliberately. `TOOL_DEFINITIONS` carries JSON Schema `inputSchema` objects that map
directly onto tool-use APIs, and `dispatchTool(session, {name, arguments})` is the whole executor.
An adapter is roughly: send `SYSTEM_GUIDANCE` plus `TOOL_DEFINITIONS`, route each tool call through
`dispatchTool`, feed results back, and surface `requiresApproval` proposals to the user for a real
confirmation before calling `session.approve`.

The model choice is deliberately not baked in. On-device inference will constrain it, and the tool
surface is small and typed specifically so a weaker local model can drive it safely: the boundary
does not trust the model's judgment about what is safe.
