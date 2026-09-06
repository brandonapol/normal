# The sealed baseline

Every device ships with one configuration it can always fall back to: signed, never written at
runtime, and known to satisfy every invariant.

## Shape

`baseline.sealed.json` holds the config document as **exact bytes**, plus the key id and a signature
over those bytes:

```json
{
  "document": "{\"apiVersion\":\"normal.os/v0\",\"kind\":\"PhoneConfig\",...}",
  "keyId": "56475aa75463474c",
  "signature": "..."
}
```

The document is a string rather than embedded JSON on purpose. Re-serialising an embedded object
reformats it — a pretty-printer will happily change the bytes and invalidate a perfectly good
signature. Storing the exact bytes removes the question.

```bash
normalctl seal key.seed > baseline.sealed.json
normalctl verify /etc/normal          # checks the baseline alongside the audit chain
```

## What verification requires

A sealed baseline has to clear four bars, and failing any one makes it unusable rather than
degraded:

| check | catches |
| --- | --- |
| Signature validates over the exact document bytes | an edited baseline |
| Key id matches the expected key | a baseline substituted wholesale |
| The document passes full schema validation | **a baseline that breaks an invariant** |
| Revision is 0 | a mid-history config passed off as a baseline |

The third is the one that matters most. A signature only proves *who* wrote it, not that what they
wrote is safe. A baseline with `injectShim: false` is refused however impeccably it is signed —
otherwise the fallback state could itself be the attack.

## Reset

Reset is a proposal like any other. It goes through validation, planning, approval, transactional
apply, and the audit log, which is what makes it recoverable and reviewable rather than a special
path that bypasses everything.

Two properties worth stating:

**Reset moves forward.** Returning to the baseline's *content* produces a **new** revision above
everything before it, rather than rewinding to revision 0. Anti-replay (NRM-123) stays intact, and
the audit chain keeps its shape — a reset is a step in the history, not a hole in it.

**Reset always requires approval**, whatever the session's approval settings. Wiping a device is not
a change anyone should be able to auto-apply.

## No tool can factory-reset the device

`ProposeReset` is a session method, deliberately absent from the agent's tool surface — the same
treatment as `Approve`. A person can reset their phone; a manipulated model cannot propose it, and a
test asserts no tool name contains "reset".

That is a slightly stronger stance than the approval gate alone would need. The reasoning: approval
is a defence against a *bad change*, but a factory reset is destructive even when it works exactly as
intended, and there is no natural-language request worth routing through the agent to get there.

## Not built yet

- **Verification at boot** before any user revision is applied. The check exists and `normalctl
  verify` runs it; wiring it into device startup is NRM-306.
- **Falling back to the baseline when the user chain is corrupt.** Detection works today; acting on
  it automatically is NRM-125.
- **Immutability.** Nothing yet stops a privileged process rewriting `baseline.sealed.json`. It is
  signature-protected, so tampering is detected, but read-only mounting is device work.
