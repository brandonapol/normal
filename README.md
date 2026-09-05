# normal

An opinionated phone OS configured by conversation instead of settings menus.

This repository is the **control plane**, not the OS: a declarative config schema, a transactional
mutation engine, and the tool interface an on-device AI agent uses to change your phone. It is
deliberately independent of the base OS decision — everything here builds, runs, and tests on a dev
machine against a mocked filesystem and service host.

## The two invariants

1. **The agent never touches the filesystem.** It proposes a diff against a versioned schema. The
   mutation engine validates, plans, applies, and rolls back. That boundary is the safety story.
2. **No infinite scroll, anywhere.** This is encoded in the schema, not in a settings toggle. The
   enforcement enum has no `off` member, the detector list is typed as non-empty, and the webview
   shim flag is typed as the literal `true`. The strongest thing anyone — user or agent — can
   express is a bounded, justified, expiring exemption for one app.

## Layout

```
schema/normal.cue        the protocol: types, constraints, and limits, in CUE
pkg/config/              Go bindings, JSON pointers, validation (CUE + semantic)
pkg/engine/              diff -> plan -> apply -> rollback, over injected ports
pkg/agent/               the agent's entire surface: tool defs, policy, session
cmd/normalctl/           dev CLI: validate, render, diff, plan
examples/                the baseline config, generated from code
docs/                    architecture, schema, agent contract, scroll blocking, base OS
```

Dependencies flow one way: `schema <- config <- engine <- agent`. Nothing depends on Android, and
nothing outside `pkg/engine/apply.go` and the port implementations performs I/O.

## Two languages, on purpose

**CUE owns the protocol.** `schema/normal.cue` is the single source of truth for shape, enums,
bounds, and the product invariants. It is embedded in the binary and evaluated at runtime, so there
is no hand-written validator to drift from it, and the numeric limits are read back out of the
schema rather than duplicated in Go.

**Go owns everything else** — the engine, the agent boundary, the daemons. It cross-compiles to a
static ARM64 binary with no runtime to ship alongside it.

The division is not arbitrary. CUE expresses constraints Go's type system cannot:

```cue
#Enforcement: "warn" | "paginate" | "block"
detectors: [#Detector, ...#Detector]
webview: injectShim: true
exemptions: [...#Exemption] & list.MaxItems(limits.maxExemptions)
```

Go handles what CUE is bad at: cross-references between sections, anything involving the current
time, and duplicate-key detection. `docs/architecture.md` has the full split.

## Quickstart

```bash
go build ./...
go test ./...                                     # 73 tests, no device required
go run ./cmd/normalctl baseline > examples/baseline.config.json
go run ./cmd/normalctl validate examples/baseline.config.json
go run ./cmd/normalctl plan current.json desired.json
```

A change, end to end:

```go
ports := engine.NewMemoryPorts(engine.MemoryOptions{Files: files})
session := agent.NewSession(agent.SessionOptions{
    InitialConfig: config.Baseline(),
    Ports:         ports.Ports,
})

agent.Dispatch(ctx, session, agent.ToolCall{
    Name: "propose_change",
    Arguments: map[string]any{
        "intent": "put Spotify on wifi only",
        "operations": []any{map[string]any{
            "op":    "set",
            "path":  "/spec/apps/entries/com.spotify.music/network",
            "value": "wifi-only",
        }},
    },
})

session.Approve("proposal-0001", "user")   // not reachable from any tool
agent.Dispatch(ctx, session, agent.ToolCall{
    Name:      "apply_proposal",
    Arguments: map[string]any{"proposalId": "proposal-0001"},
})
```

Swap `NewMemoryPorts` for real ones and the same code runs on hardware.

## Status

v0. The schema covers launcher, apps, notifications, and attention policy. Theme, connectivity,
and update policy are not modelled yet. Nothing here has touched a phone.
