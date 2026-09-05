# Security policy

Normal is a phone operating system's control plane. It decides app permissions, network access, and
attention policy, and it is driven by an on-device AI agent. A vulnerability here is a vulnerability
on someone's phone, so we would much rather hear about a problem than not.

## Reporting a vulnerability

**Use GitHub's private vulnerability reporting**, from the Security tab of this repository. It
creates a private thread visible only to maintainers, and it does not expose the report while it is
being fixed.

> **Maintainer setup required:** private vulnerability reporting must be enabled once in
> Settings → Code security and analysis. Until that is done, and until a security contact address is
> published here, there is no confidential reporting channel — please do not open a public issue for
> a security problem in the meantime.

Please do not open a public issue, pull request, or discussion for a security problem.

### What helps

- What you were able to do that you should not have been able to do
- The smallest config, patch, or agent request that reproduces it
- Which component: schema validation, mutation engine, agent boundary, or CLI
- Whether it needs an already-compromised agent, or works from an ordinary one

Reproductions as an invariant-corpus fixture (`testdata/invariants/`) are especially welcome, since
that is the form the fix will need anyway.

### What to expect

| | |
| --- | --- |
| Acknowledgement | within 3 working days |
| Initial assessment | within 10 working days |
| Fix or mitigation plan | communicated with the assessment |
| Public disclosure | coordinated with you, by default after a fix ships |

This is a pre-release project maintained without a dedicated security team. These are honest
intentions rather than a contractual SLA, and we will say so if a report is going to take longer.

## Safe harbour

We will not pursue or support legal action against anyone who, in good faith:

- tests against their own devices and their own data
- avoids privacy violations, data destruction, and service degradation
- gives us reasonable time to fix an issue before disclosing it publicly

If you are unsure whether something is in scope, ask first through the reporting channel above.

## Supported versions

| version | supported |
| --- | --- |
| `main` | yes |
| tagged releases | none yet |

The config protocol is at `apiVersion: normal.os/v0` and makes no compatibility promises. There are
no releases and no users on hardware, so there is nothing to backport to yet. This table changes
when the first release ships.

## Scope

**In scope**

- Bypassing the agent boundary: any way an agent reaches the filesystem, escalates beyond its
  writable roots, or applies a change without approval
- Defeating the attention invariants through a config the validator accepts — enforcement disabled,
  detectors emptied, the webview shim off, exemption caps exceeded
- Resource exhaustion in validation or the mutation engine
- Leaving the device inconsistent through a crafted transaction, or defeating rollback
- Supply-chain weaknesses in the build and release pipeline

**Not in scope, yet**

- Anything requiring physical access to an unlocked device
- Vulnerabilities in GrapheneOS, AOSP, or upstream dependencies — report those upstream, though we
  want to hear if we use them unsafely
- Device-side components that do not exist yet: the privileged helper, the on-device agent runtime,
  and scroll enforcement are unbuilt (see [`ROADMAP.md`](ROADMAP.md))

## What is already hardened

Useful context for where to look, and honest about the gaps:

- The attention invariants are types in `schema/normal.cue`, not runtime checks. A frozen corpus in
  `testdata/invariants/` is verified by two independent paths on every CI run
- The agent has nine tools, no filesystem access, and no tool that can approve its own proposal
- Patches are confined to writable roots and re-validated against current state at apply time
- Document size, depth, node count, and detector regex length are capped before evaluation
- Fuzzing covers the pointer, patch, and diff layers; crashers are committed as regression seeds

The known gap is recovery: when a transaction fails *and* its rollback fails, the engine reports
`DeviceDirty` honestly but cannot resolve it. That is tracked as NRM-125.
