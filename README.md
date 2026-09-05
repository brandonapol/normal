# normal

An opinionated phone OS configured by conversation instead of settings menus.

This repository is the **control plane**, not the OS: a declarative config schema, a transactional
mutation engine, and the tool interface an on-device AI agent uses to change your phone. It is
deliberately independent of the base OS decision — everything here builds, runs, and tests on a dev
machine against a mocked filesystem and service host.

## The two invariants

1. **The agent never touches the filesystem.** It proposes a diff against a versioned schema. The
   mutation engine validates, plans, applies, and rolls back. That boundary is the safety story.
2. **No infinite scroll, anywhere.** This is encoded in the schema, not in a settings toggle. There
   is no `enforcement: "off"`; the detector list cannot be emptied; the webview shim cannot be
   disabled. The strongest thing anyone — user or agent — can express is a bounded, justified,
   expiring exemption for one app.

## Layout

```
packages/
  schema/        @normal/schema      the protocol: types, validator, pointers, baseline config
  engine/        @normal/engine      diff -> plan -> apply -> rollback, over injected ports
  agent-tools/   @normal/agent-tools the agent's entire surface: tool defs, policy, session
examples/
  baseline.config.json               the default config, generated from code
docs/
  architecture.md                    package boundaries and why they are where they are
  config-schema.md                   the v0 schema, field by field
  agent-interface.md                 the tool contract and its guarantees
  infinite-scroll.md                 four enforcement approaches, with a recommendation
  base-os.md                         GrapheneOS as the base: the case for, and against
```

Dependencies flow one way: `schema <- engine <- agent-tools`. Nothing depends on Android, and
nothing outside `engine/src/apply.ts` and the port implementations performs I/O.

## Quickstart

```bash
npm install
npm run build      # typecheck via project references
npm test           # 84 tests, no device required
npm run emit:example
```

A change, end to end:

```ts
const ports = createMemoryPorts({ files: renderConfig(BASELINE_CONFIG) });
const session = createAgentSession({ initialConfig: BASELINE_CONFIG, ports });

await dispatchTool(session, {
  name: "propose_change",
  arguments: {
    intent: "put Spotify on wifi only",
    operations: [
      { op: "set", path: "/spec/apps/entries/com.spotify.music/network", value: "wifi-only" },
    ],
  },
});

session.approve("proposal-0001", "user");   // not reachable from any tool
await dispatchTool(session, { name: "apply_proposal", arguments: { proposalId: "proposal-0001" } });
```

Swap `createMemoryPorts` for real ones and the same code runs on hardware.

## Status

v0. The schema covers launcher, apps, notifications, and attention policy. Theme, connectivity,
and update policy are not modelled yet. Nothing here has touched a phone.
