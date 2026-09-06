# Secrets in configuration

**Policy: a Normal config carries no credentials. Not encrypted ones, not encoded ones, none.**

## Why this is a policy and not a feature

Config is rendered into `/etc/normal/*.json` and read by every service that needs it. Those files
are ordinary files, read by several processes, included in diagnostics, and — because the whole
product is GitOps-shaped — the exact thing a user is most likely to share when asking for help or
contributing a config to the community.

A credential in that pipeline leaks by design, not by accident.

v0 has no secret-bearing field, and that is worth keeping deliberate rather than discovering later
that one crept in.

## What is enforced today

Validation rejects anything that looks like a credential in a free-text field, with issue code
`plaintext-secret`. Detection covers PEM blocks, JWTs, PuTTY keys, and the distinctive token formats
of GitHub, Slack, AWS, Google, GitLab, npm, OpenAI, and Hugging Face.

Fields scanned are the ones a person or an agent could plausibly paste into: `metadata.description`
and labels, launcher titles, labels, widget providers and shortcut URLs, app labels and sandbox
profiles, notification channel and title matchers, detector patterns and surfaces, exemption
reasons, and budget domains.

Detection is **format-based, not entropy-based**. A token is recognised by its shape — `ghp_`
followed by twenty-plus base64 characters — anywhere in the value, including mid-sentence, which is
how one actually gets pasted in. Entropy scoring would fire on legitimate content: a regex detector
pattern is high-entropy by nature, and a false positive that blocks a valid config is worse than a
missed exotic token format, because it teaches people to work around the check.

That means the scanner is a **backstop, not a boundary**. It catches the realistic mistake — a token
pasted into a description — and will miss a credential that does not match a known shape. It is not
a reason to relax the policy above.

## When a secret-bearing field is genuinely needed

Some future field may legitimately need one: a wifi PSK, a sync token, a registry credential. The
design is decided in advance so the pressure of needing it does not decide it badly:

1. **The config stores a reference, never a value** — a key name resolved at runtime against secure
   storage, not a string that happens to be encrypted.
2. **The reference is typed**, so the validator can tell a reference from a value, and the
   plaintext scanner can be skipped for that field only.
3. **Rendered files carry the reference, not the resolved value.** Resolution happens in the service
   that needs the credential, at the point of use.
4. **Adding the first such field requires an ADR**, because it changes what `/etc/normal` is.

No `#Secret` type has been added to the schema. Shipping an unused type would be speculative, and a
type nobody uses drifts out of alignment with what the first real use actually needs. The design is
recorded here; the type gets written alongside its first field.

## Testing a secret scanner trips other secret scanners

Worth knowing before editing these tests: the first version used realistic token literals, and
GitHub push protection rejected the push, citing a Slack token in `secrets_test.go`. The literals
were fake, but they were shaped convincingly enough to be indistinguishable from real ones — which
is exactly what made them good test data.

Credential shapes are therefore **assembled at run time** from fragments, so no secret-shaped
literal exists in the source. The tests are unchanged in what they assert; the strings just do not
exist until the test runs. The committed corpus fixture uses a bare PEM header with no key material
for the same reason.

Incidentally, this confirms secret scanning and push protection are already enabled on the
repository, which is half of what NRM-107 was waiting on.

## Rendered files are safe to read

A test asserts the shipped baseline carries nothing that trips the scanner, and a corpus fixture
proves a config with a token in its description is rejected. Combined with the policy above, every
file under `/etc/normal` is safe to read by any service that reads it today, and safe to attach to a
bug report.
