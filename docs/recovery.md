# Recovery

The engine has always been honest about the one failure it could not fix: when an apply fails *and*
its rollback fails, it reported `DeviceDirty` and stopped. Worse, a power cut mid-apply left no code
running at all — no rollback, no report, just a half-written device.

Recovery closes both.

## What an interrupted device looks like

Before touching anything, the engine writes a pending marker carrying the transaction's intent, the
files it is about to change, the services that read them, and **the exact contents of every file
before the change**. A device that loses power mid-transaction therefore boots with:

- an intact, verifiable audit chain
- an orphaned pending marker
- some files new, some old

That is enough to put things back.

```bash
normalctl recover /etc/normal            # report what it would do
normalctl recover --apply /etc/normal    # carry it out
```

Recovery is a dry run by default. Repairing a device is destructive enough to deserve an explicit
`--apply`, and the dry run prints exactly which files it would restore or remove.

## The ladder

Recovery tries the least drastic thing that works, and stops as soon as one succeeds:

| step | outcome | meaning |
| --- | --- | --- |
| No pending marker | `nothing-to-do` | the device is consistent |
| Restore every file from the snapshot, restart affected services | `restored-from-snapshot` | the device is back where it was before the interrupted change |
| Write the sealed baseline instead | `fell-back-to-baseline` | the snapshot could not be restored, so the device is at a known-good state |
| Neither worked | `unrecoverable` | stated plainly, with the reason |

`unrecoverable` is a report, not a crash and not a loop. The device says which transaction left it
inconsistent and why repair failed, and returns a non-zero exit rather than retrying forever. A
device that cannot be fixed automatically should say so and stop, not boot-loop.

Falling back to the baseline needs a baseline that is *trustworthy* — verified signature, valid
schema, revision zero — so a tampered baseline produces `unrecoverable` rather than a quiet
downgrade to an attacker's config. Missing baseline and untrusted baseline are separate messages,
because they need different fixes.

## Recovery is audited

Every attempt appends an entry: intent, outcome, and the transaction it was repairing. A device that
has been recovered says so in its own history, and the chain still verifies afterwards. Repair that
leaves no trace would undermine the point of having the log.

## Paths are canonical

The snapshot records canonical paths (`/etc/normal/launcher.json`), and `normalctl recover` maps
them onto whatever directory you point it at. That is what makes an interrupted device inspectable
and repairable from a laptop, not only in place. On a real device the two are the same path and the
mapping is the identity.

## Not built yet

- **Running at boot.** `normalctl recover` is the logic; invoking it before any user revision is
  applied is NRM-306.
- **Atomic restoration.** Recovery writes through the same filesystem port as everything else, so it
  inherits NRM-301's pending atomicity work. A power cut *during recovery* is currently not covered.
- **Telling the user on screen.** `unrecoverable` returns a message and a non-zero exit; surfacing it
  in a UI is device work.
