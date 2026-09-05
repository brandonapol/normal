# Licence policy

Normal ships as a binary on someone's phone. A dependency's licence is therefore a distribution
obligation, not a formality, and a copyleft dependency arriving unnoticed is the kind of surprise
that is expensive to unwind after a release.

## The allowlist

`ALLOWED_LICENCES` in the `Makefile`:

```
Apache-2.0, MIT, BSD-2-Clause, BSD-3-Clause, ISC
```

`make licences` fails the build on anything else, and it runs as part of `make ci`. The list lives
in the repository rather than in repository settings, so changing it is a reviewable diff.

## What is not on the list, and why

**Weak copyleft (MPL-2.0, LGPL).** File-level and library-level obligations are workable but need a
deliberate decision about static linking, which is how Go ships. Not blanket-denied — blocked
pending a decision, so the conversation happens before the dependency lands.

**Strong copyleft (GPL, AGPL).** Incompatible with shipping a device binary under most reasonable
readings. An AGPL dependency in a config daemon would be a serious problem discovered late.

**Unlicensed or unclear.** A dependency with no licence file grants no rights. Treated as a failure,
not a warning.

To add something outside the list, change `ALLOWED_LICENCES` in the same PR that adds the
dependency, so a reviewer sees both together.

## Current state

Every transitive dependency is permissively licensed:

| licence | libraries |
| --- | --- |
| Apache-2.0 | `cuelang.org/go`, `github.com/cockroachdb/apd/v3`, `github.com/protocolbuffers/txtpbfmt` |
| MIT | `github.com/emicklei/proto`, `github.com/mitchellh/go-wordwrap`, `github.com/pelletier/go-toml/v2`, `go.yaml.in/yaml/v3` |
| BSD-3-Clause | `github.com/google/uuid`, `golang.org/x/net/idna`, `golang.org/x/text`, `google.golang.org/protobuf` |

Regenerate with `make licence-report`.

## Normal's own licence — unresolved

**This repository has no `LICENSE` file.** The licence checker reports the project's own packages as
`Unknown`, which is accurate: without one, default copyright applies and nobody has permission to
fork, modify, or redistribute it.

That matters more here than in most projects. The stated goal is that the schema becomes the thing
other people fork and extend — the omarchy model — and that is not legally possible as things stand.

Choosing a licence is a maintainer decision with long consequences: relicensing later needs every
contributor's agreement, which gets harder with each contribution. It is deliberately left open here
rather than picked by default. Worth resolving before the first outside contribution, not after.

## Vulnerability scanning

Advisory coverage comes from `govulncheck` (`make vuln`, plus a weekly scheduled run), which reads
the Go vulnerability database and reports only vulnerabilities actually reachable from our code.

GitHub's `dependency-review-action` is **not** used: it requires the dependency review API, available
on public repositories and on private repositories with Advanced Security. Adding it to a private
repository without GHAS would produce a job that fails for permissions reasons rather than code
reasons — the same trap noted for CodeQL in NRM-107. It becomes worthwhile alongside that ticket.
