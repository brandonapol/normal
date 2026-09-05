# The audit log

Every transaction the mutation engine applies leaves one tamper-evident record. The log answers
"what changed this device, when, on whose authority, and has anyone edited the answer since."

## Shape

Newline-delimited JSON at `/etc/normal/audit.log`, one entry per completed transaction:

```
#1  2026-01-01 00:00:02Z  revision 1 -> 2  [applied]
    silence notifications by default
    approved by brandon
    transaction txn-0002
    files    /etc/normal/metadata.json, /etc/normal/notifications.json
    services normal-notifyd
    config   0bac54625752 -> 8f4481eb9684
    chain    f93f17f5aa6b <- 53f9f56ead29
```

Each entry hashes its own content together with the previous entry's hash. Editing any field changes
that entry's hash, which breaks every link after it, so verification names the *first* entry that
does not add up rather than reporting a wall of noise.

```bash
normalctl audit log      /etc/normal
normalctl audit verify   /etc/normal   # exits non-zero on a broken chain
```

## What verification checks

| check | catches |
| --- | --- |
| Entry hash matches its content | an entry edited in place |
| `previousHash` matches the entry before | an entry inserted or replaced |
| Sequence numbers are contiguous | an entry deleted |
| Outcome is one of the known results | a fabricated or corrupted status |
| **`configBefore` matches the previous `configAfter`** | **the config changed outside the engine** |

That last one is worth calling out. Each entry records the digest of the rendered config before and
after. Since a rolled-back transaction leaves the config where it started, an intact chain has
continuous config state throughout. A gap means something wrote `/etc/normal` without going through
the engine — the beginning of the detection NRM-122 completes with signing.

## Complete versus intact

These are different failures and the tooling keeps them apart.

An **intact** log is one where the cryptography adds up. A **complete** log is one where every
transaction that started also finished.

Before executing any action, the engine writes a pending marker naming the transaction. On
completion — applied, rolled back, or dirty — it appends the sealed entry and clears the marker. So
a device that loses power mid-transaction comes back with a valid chain plus an orphaned marker, and
verification reports:

```
The log is incomplete rather than corrupt: entries before this point are intact.
```

A log truncated mid-line is treated the same way: the entries that parsed are verified normally and
the partial tail is reported as truncation. Losing the tail of a log is a crash; losing its integrity
is an attack, and conflating them would make the log useless for either.

Acting on an incomplete log — re-attempting rollback, falling back to the sealed baseline — is
NRM-125's job. This ticket only makes the state legible.

## Failed transactions are recorded too

A rolled-back apply appends an entry with outcome `rolled-back` and `configAfter` equal to
`configBefore`, because that is what happened. A failed rollback records `dirty`. Recording only
successes would leave the most interesting events invisible.

A no-op apply records nothing: nothing changed, so there is nothing to attest.

## Signatures

Hash chaining alone is tamper-*evident*: anyone who can rewrite the whole file can recompute every
hash and produce a self-consistent chain. Signing closes that.

Each entry is signed with a device-local key over its own hash, and the key id is part of the hashed
content, so a substituted key changes the hash and invalidates the signature. Rewriting the log now
requires the private key, not just write access.

```bash
normalctl verify /etc/normal     # chain, signatures, and config drift
```

Verification with a public key present enforces three further properties:

| check | catches |
| --- | --- |
| Every entry carries a signature | an entry appended without the key |
| Signature validates against the hash | a rewritten chain |
| Key id matches the expected key | a chain signed with someone else's key |

The `Signer` interface is `KeyID`, `Sign`, and `PublicKey` — deliberately no way to export the
private key, so signing is an operation rather than a key hand-out. A test asserts the interface
offers no exporter. `SoftwareSigner` (ed25519) is the development implementation; the device swaps
in a hardware-backed one behind the same interface, which is why this is a port and not a function.

**Key storage is not solved here.** The interface is right and the cryptography works, but where the
private key lives on a real device — and how it resists an attacker with root — is device work, not
control-plane work.

## Replay and the revision ceiling

Signing stops a chain being *rewritten*. It does not stop an old, validly-signed chain being put
back — every entry in it is genuine, just stale. Restoring yesterday's log and config is how an
attacker regains a permission that was revoked today.

The store keeps a monotonic ceiling: the highest revision this device has ever reached. Verification
fails when the log ends below it:

```
entry 0: the log ends at revision 1 but this device has reached 3;
         an older log was replayed (revision-replay)
```

Rollback is unaffected, because rolling back moves *forward* — returning to revision 0's content
produces a new revision above the ceiling rather than rewinding to it. A test pins that distinction.

## Config drift

Each entry records the digest of the *rendered* config, not just the document, so `normalctl verify`
can hash what is actually on disk and compare it to what the last transaction claims to have left:

```
entry 0: the config on disk hashes to d98be7736b30 but the last transaction
         left 0bac54625752; it was changed outside the engine (config-drift)
```

That is the check that catches a hand-edited `/etc/normal/*.json`. Without `audit.pub` present,
verification still runs but says plainly that signatures were not checked, rather than passing
quietly and implying more assurance than it delivered.

## Limits today

- **Append is read-modify-write** through the filesystem port, not `O_APPEND`. Correct, but the real
  port (NRM-301) should make it a genuine append.
- **Key storage is unbuilt.** See above; the interface is ready, the device side is not.
- **Nothing rotates the log yet.** It grows without bound.
- **The revision ceiling is only as trustworthy as its storage.** It sits beside the log, so an
  attacker who can roll back the log can roll back the ceiling too. Real anti-replay needs a counter
  in secure storage, which is device work; the check and its port are ready for it.
