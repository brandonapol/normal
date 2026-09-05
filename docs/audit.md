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

## Limits today

- **Append is read-modify-write** through the filesystem port, not `O_APPEND`. Correct, but the real
  port (NRM-301) should make it a genuine append.
- **The log is tamper-evident, not tamper-proof.** Anyone who can rewrite the whole file can produce
  a self-consistent chain. Detecting that needs a signature over the head, which is NRM-122.
- **Nothing rotates the log yet.** It grows without bound.
