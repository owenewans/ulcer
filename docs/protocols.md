# protocol and engine policy

The reviewed source of truth is [`versions/engines.json`](../versions/engines.json).
Exact repository gitlinks are independently checked against
[`versions/sources.json`](../versions/sources.json) by `mage freeze`.
An engine shown in the catalog is not automatically deployable. The API and UI
must check `adapter_status` and the exact capability set for the frozen artifact.

## distinctions that matter

- REALITY is a transport security layer, not a separate Trojan or VLESS protocol.
- XHTTP is an Xray transport. sing-box does not currently implement it.
- XHTTP padding, VLESS Encryption padding, VMess global padding, multiplex
  padding and AnyTLS padding are different features.
- Hysteria Gecko exists only in official Hysteria and remains experimental.
- TUIC's current upstream repository is a specification, not a maintained server binary.
- An `h3` ALPN value does not turn a custom QUIC proxy into generic HTTP/3 CONNECT.

## integration policy

- Prefer a library only when upstream exposes a stable lifecycle and accounting API.
- Otherwise supervise a frozen binary behind the same private adapter contract.
- Xray remains necessary for VLESS + REALITY + XHTTP and its padding controls.
- OLCRTC is a strong library candidate, but immediate quotas need streaming counters.
- The custom SSH server will use virtual principals, never Linux users, and allow
  only `direct-tcpip`; shell, PTY, exec, subsystem, X11, agent and reverse
  forwarding are rejected.
- AnyTLS reference redistribution is blocked until its repository contains a
  license or written permission is obtained.

## freeze manifest

Every releasable artifact records upstream URL, tag, full commit, patch digest,
toolchain image digest, architectures, OCI digest, SBOM, licenses, provenance,
signature, config schema and adapter capability document. Updating a Git tag
never updates a deployed artifact implicitly.
