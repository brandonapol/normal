# The base OS: GrapheneOS, examined

You asked to be pushed back on. Short version: **GrapheneOS is the right upstream, and it is
probably the wrong thing to fork on day one.** The base is right; the packaging is the part worth
reconsidering.

## Why the alternatives lose

**Waydroid on a Linux mobile stack** (postmarketOS, Mobian, SailfishOS). Android apps run in a
container. Spotify mostly works. Maps does not, in the way that matters: GNSS plumbing into the
container, Play Services location, and turn-by-turn on a device whose sleep/wake handling is not
Android's. By your own stated constraint — this project dies if Spotify and Maps don't work — this
is disqualifying, and it is disqualifying for years, not months.

**LineageOS.** Broadest hardware support, and that is genuinely valuable. But there is typically no
verified boot with your own keys on most devices, a weaker security model, and per-device
maintainer quality that varies wildly. Building a phone whose selling point includes an AI agent
with write access to system config, on a base with a soft security floor, is a bad pairing.

**CalyxOS.** microG instead of sandboxed Play. microG is a reimplementation, and app compatibility
is meaningfully worse in exactly the long tail you cannot predict. It also puts a compatibility
shim on the critical path for your two must-work apps.

**/e/OS.** Consumer-friendly, weakest security posture of the group, slow security patching. No.

**Raw AOSP.** Maximum control, and you would spend the first year reimplementing the hardening
GrapheneOS gives you for free — hardened malloc, exploit mitigations, the sandboxed Play compat
layer, attestation. Not a good use of the first year.

So the base-layer reasoning holds. Sandboxed Google Play is the single best answer to the
Spotify-and-Maps constraint, and it is GrapheneOS's.

## Where the plan is weaker than it looks

**Hardware is Pixel-only.** Official GrapheneOS support means Pixels, because Pixels are nearly
alone in allowing bootloader re-locking with custom AVB keys — which is what lets *you* ship a
verified-boot image users can trust. That is a hard product constraint: your addressable hardware is
one vendor's line, and you inherit that vendor's release cadence and end-of-support dates. Worth
deciding deliberately rather than discovering later.

**The rebase treadmill is the real cost.** GrapheneOS ships fast and ships often, tracking Android
security bulletins closely. A fork that lags is a fork that is less secure than its upstream while
claiming its upstream's reputation. That is a monthly, non-negotiable engineering commitment,
starting the day you fork and never stopping. It is the thing most likely to kill this project
quietly — not a technical wall, just an unglamorous recurring cost that competes with feature work
forever.

**Branding and community relations.** GrapheneOS has an explicit trademark policy and a documented
history of being unhappy with downstream distributions that trade on its name or confuse users about
what they are running. Whatever you build, do not describe it as "GrapheneOS-based" without reading
their policy and, ideally, talking to them first. This is a solvable problem, but it is solved by
conversation before launch, not by a rename after.

**Sandboxed Play is not perfect.** Spotify works, including offline. Maps works. Android Auto is a
known gap. Play Integrity affects some apps — mostly banking, not your two — and the failure mode is
per-app and unpredictable. Fine for your requirements; worth an explicit compatibility matrix in the
product rather than a promise.

## The recommendation: layer first, fork later

Structure the product as a **layer that installs on stock GrapheneOS**: the launcher, the config
services, the mutation engine, the agent, and the accessibility-based scroll enforcement, all as
apps and a system-app bundle. No fork, no rebase treadmill, no trademark question, and you can put
it on a Pixel this month.

That gets you everything in this repository running on real hardware, and everything in
`docs/infinite-scroll.md` except approach 3.

Take on your own signed image when — and only when — you need the thing the layer cannot give you:
framework-level scroll enforcement the user cannot switch off in Settings, and config the agent
manages below the app layer. That is a real reason to fork, and by then you will have a product
validating the cost instead of a bet funding it.

The honest tension: "no infinite scroll, enforced at the OS level" is only literally true in the
forked version. On the layer it is a very good user-space enforcement that a determined user can
disable. Decide now which claim you are making, because it changes what "v1" means — and it is
easier to under-promise on the layer and over-deliver on the image than to walk a claim back.

## What this repo assumes

Nothing above. Everything here — schema, engine, agent tools — is base-OS independent. The engine
touches the device through `FileSystemPort` and `ServiceHostPort`, so the layered version implements
those against app-private storage plus a privileged helper, and the forked version implements them
against real system paths and init services. Same code either way, which is the point of building
this part first.
