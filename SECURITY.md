# Security

ORYCA sits in front of other people's APIs, so please report problems privately.

## How to report

Go to the **Security** tab and choose **Report a vulnerability**. Only the
maintainers see it. Please do not open a normal issue, those are public.

Helpful to include. What an attacker could do, the version you tested, how you
deployed it, and steps to reproduce.

You will hear back within a week. We will agree a disclosure date with you, and
credit you unless you prefer we did not.

## Versions

Before 1.0, fixes go to the latest release only.

## Scope

The gateway, control plane, portal, and the defaults shipped here. Problems in
your own upstream APIs, or in a dependency we do not expose, belong upstream.

## Two settings that matter most

- `ORYCA_INTERNAL_SECRET` protects the endpoints the gateway uses to read
  services, keys, and users. Change it from the example value before anyone
  outside your network can reach the stack.
- The first admin password is generated on first start and printed once. If you
  set your own, treat it like any production credential.

Run both services behind TLS.
