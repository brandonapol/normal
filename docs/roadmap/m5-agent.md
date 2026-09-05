# M5 — The agent, for real

**Goal:** a model running on-device, driving the existing tool surface, with an approval flow that
stays trustworthy even when the model is wrong or manipulated.

Deliberately last of the device work. The engine is what makes the agent safe, and the engine
already exists. Wiring a model in earlier would create pressure to loosen the boundary for the sake
of a demo.

---

### NRM-401 — On-device model runtime evaluation
**Size:** L — **Depends on:** NRM-203

**Scope**
- Evaluate llama.cpp, MediaPipe LLM Inference, and MLC against: quality on tool-calling, memory
  footprint, battery cost, cold-start latency, and licence
- Tool-calling reliability is the deciding axis; a model that emits malformed tool calls is useless
  here regardless of benchmark scores
- Decide: fully local, or local with an opt-in remote fallback for hard requests

**Acceptance**
- [ ] Written comparison with measured numbers on target hardware
- [ ] A recommendation with the tradeoff stated plainly
- [ ] A prototype answering "put Spotify on wifi only" end to end on-device

---

### NRM-402 — Tool-use loop adapter
**Size:** M — **Depends on:** NRM-401

**Scope**
- Adapter from `ToolDefinitions()` and `Dispatch` to the chosen runtime's tool-call format
- Conversation state, tool-result feedback, turn limits
- Deterministic replay for testing: a recorded transcript re-runs identically

**Acceptance**
- [ ] The whole tool surface is reachable by the model
- [ ] Turn limits and malformed tool calls are handled without wedging the session
- [ ] A golden-transcript test suite runs in CI with no model in the loop

---

### NRM-403 — Approval UI renders the engine's diff, never the agent's summary
**Size:** M — **Depends on:** NRM-402

**The single most important control in this milestone.**

The agent produces a natural-language summary of what it proposes. If the model is manipulated, that
summary can lie: "just tidying your home screen" over a proposal that grants microphone access. The
approval surface must render from `Plan` and `Diff` — engine output the agent cannot influence —
with the agent's prose clearly subordinate.

**Scope**
- Approval screen renders engine-derived diff, affected services, and every `review` policy issue
- Agent's stated intent shown as a labelled claim, visually distinct from the facts
- Weakenings of attention or permission policy get distinct, unmissable treatment
- Approving requires seeing the diff; no bulk-approve, no "always allow"

**Acceptance**
- [ ] A test proves the rendered approval is a pure function of `Plan`, independent of agent text
- [ ] A deliberately mendacious summary over a permission grant still shows the grant prominently
- [ ] No path applies a `RequiresApproval` proposal without rendering the diff first

---

### NRM-404 — Prompt injection threat model and defences
**Size:** L — **Depends on:** NRM-111, NRM-403

The agent will read attacker-influenceable text: app names, notification content, and anything the
user pastes. Any of it can carry instructions.

**Scope**
- Enumerate every untrusted input reaching the model's context, and label it as data in the prompt
- Confirm the boundary holds under injection: the worst outcome should be a *rejected or
  visibly-flagged proposal*, never a silent apply
- Red-team corpus of injection attempts as a regression suite
- Explicit rule: the agent never gains a tool that approves, and never sees the approval token

**Acceptance**
- [ ] `docs/security/prompt-injection.md` enumerates inputs, defences, and residual risk
- [ ] A red-team corpus of at least 30 attempts runs in CI; none produce an unapproved apply
- [ ] Injection attempts that produce a flagged-for-approval proposal are documented as
      working-as-intended, not as failures
- [ ] Any new agent input source requires a threat-model update, enforced by review checklist

---

### NRM-405 — Agent action audit trail
**Size:** S — **Depends on:** NRM-121

**Acceptance**
- [ ] Every proposal, rejection, approval, and apply is recorded with its intent
- [ ] Discarded and rejected proposals are recorded too — attempts matter
- [ ] `normalctl audit log --agent` renders agent activity readably

---

### NRM-406 — Conversation persistence and privacy
**Size:** M — **Depends on:** NRM-401

**Acceptance**
- [ ] Conversation history stored encrypted at rest, on-device
- [ ] User can review and delete history; deletion is real
- [ ] History is never included in any diagnostic bundle by default

---

### NRM-407 — Degraded and offline behaviour
**Size:** S — **Depends on:** NRM-402

**Acceptance**
- [ ] Model unavailable degrades to direct config editing, with a clear explanation
- [ ] A mid-conversation failure never leaves a half-applied transaction
- [ ] The phone remains fully usable with the agent disabled

---

### NRM-408 — Agent evaluation suite
**Size:** M — **Depends on:** NRM-402

**Scope**
- Fixed set of natural-language requests with expected config outcomes
- Scored on: correct diff produced, unnecessary changes avoided, invariants respected, honest
  summaries
- Run against candidate models to make NRM-401's recommendation re-checkable over time

**Acceptance**
- [ ] At least 40 scenarios covering launcher, apps, notifications, and attention
- [ ] Includes scenarios that *should* be refused, such as "disable scroll blocking entirely"
- [ ] Runs offline against recorded model outputs in CI
