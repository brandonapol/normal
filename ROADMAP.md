# Roadmap

Where this goes from "a control plane that runs on a laptop" to "an OS you can hand someone."

Tickets live in [`docs/roadmap/`](docs/roadmap/), one file per milestone, IDs `NRM-nnn`. Each has
scope, testable acceptance criteria, and an explicit out-of-scope list.

## Where we are

| | status |
| --- | --- |
| Config schema as a CUE protocol, with the scroll invariants as types | done |
| Mutation engine: pure diff/plan, transactional apply, snapshot rollback | done |
| Agent tool boundary: nine tools, no filesystem, no self-approval | done |
| CI: static checks, race, coverage floor, frozen invariant corpus, arm64 build | done |
| Anything running on a phone | **not started** |

Everything so far runs against mocked ports. That was deliberate — the design is validated before
hardware constrains it — but it means the riskiest work is still ahead.

## Milestones

| | milestone | why it comes here | rough size |
| --- | --- | --- | --- |
| **M1** | [Supply chain & CI hardening](docs/roadmap/m1-supply-chain.md) | An OS people flash needs provenance before it has users, not after | 13 tickets |
| **M2** | [Config integrity & recovery](docs/roadmap/m2-config-integrity.md) | The config *is* the security policy; it needs its own integrity story | 6 tickets |
| **M3** | [Cross-compilation & mobile bindings](docs/roadmap/m3-cross-compilation.md) | Unblocks every device-side milestone | 8 tickets |
| **M4** | [Device integration](docs/roadmap/m4-device-integration.md) | Real ports, privileged helper, install on stock GrapheneOS | 7 tickets |
| **M5** | [The agent, for real](docs/roadmap/m5-agent.md) | On-device model, tool loop, and the approval UX that makes it safe | 8 tickets |
| **M6** | [Scroll enforcement](docs/roadmap/m6-scroll-enforcement.md) | The product claim; hardest and most novel | 6 tickets |
| **M7** | [Protocol & community](docs/roadmap/m7-protocol.md) | Turn the schema into something others build on | 6 tickets |

## Sequencing, and why

**M1 and M2 come before device work, not after.** Once there are users with flashed devices, adding
signing and provenance is a migration; before that it is a config change. The same applies to the
audit log and anti-rollback: they define the update semantics that everything downstream assumes.

**M3 gates M4, M5 and M6.** All three need code running on a phone. Do the binding work once,
properly, rather than three times badly.

**M6 can start before M4 finishes.** The WebView shim (NRM-501) and the surface registry (NRM-503)
are both testable off-device, and the registry is the piece the community can contribute to
earliest.

**M5 is last of the device work on purpose.** The agent is the most visible part and the least
load-bearing: the engine is what makes it safe, and the engine already exists. Wiring a model in
early would create pressure to loosen the boundary for demo purposes.

## Two things this roadmap says no to

**iOS as an OS target.** iOS cannot host this product: no launcher replacement, no system daemons,
no config writes, no cross-app scroll enforcement. What iOS *can* host is a client — config
authoring, the agent conversation, and a Safari extension covering the web half of scroll blocking.
NRM-204 delivers the library binding for that; it does not pretend the OS ships there. See
[m3](docs/roadmap/m3-cross-compilation.md) for the full argument.

**Framework-level scroll enforcement, for now.** NRM-504 is a spike, not a commitment. It requires
owning a signed OS image and taking on the GrapheneOS rebase treadmill, and that decision should be
made with a validated product rather than ahead of one. Until then the honest claim is
"user-space enforcement", not "OS-level". See [docs/base-os.md](docs/base-os.md).

## Definition of done, repo-wide

A ticket is done when `make ci` passes and:

- new invariants have fixtures in `testdata/invariants/` and are named in the manifest
- anything security-relevant has a threat-model note in `docs/security/`
- anything device-touching has a failure and rollback path, tested against a fault-injecting port
- public protocol changes bump `apiVersion` and ship a migration
