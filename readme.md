<div align="center">

<img src="web/app/icon.svg" alt="ulcer application icon" width="96" height="96">

# ulcer

enterprise protocol infrastructure without ceremony.

</div>

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/owenewans/ulcer/master/install.sh | sh
```

> [!WARNING]
> Run the installer on a fresh Debian 12 or Ubuntu host as `root`. The host must
> use `systemd` and cgroup v2. Trusted public HTTPS requires ports `80` and `443`
> to reach Caddy for ACME validation. Export `ULCER_PUBLIC_ADDRESS` with a domain
> or public IP before running the command to override automatic IPv4 detection.
> Pipe code into a root shell only after auditing it. The installer downloads a
> pinned bundle from GitHub Releases and verifies it against the release
> `SHA256SUMS`; audit the installer and verify the release checksums and
> provenance independently when establishing the trust chain.

On first installation, occupied ports `80` or `443` cause the installer to stop;
upgrades reuse the installed Caddy units. The installer never flushes the host
firewall.

## Current Status

Ulcer separates the control plane into `HOST` and independently reconciled
`INSTANCE` agents. The UI is an API client without a privileged back channel.
Engines are built from reviewed, commit-pinned source and deployed by immutable
artifact digest.

The current foundation includes:

- local-token onboarding and one TOTP operator
- hashed, expiring, HttpOnly sessions and single-use recovery codes
- Badger desired state, monotonic generations, and an event ledger
- a bidirectional gRPC control stream over TLS 1.3 mTLS
- SPIFFE-bound certificate identity for every instance
- replay-safe, contiguous traffic acknowledgements
- a REST API, live SSE, and a responsive Next.js operator console
- Podman images, Quadlet templates, and Caddy IP/domain HTTPS
- manual ZIP enrollment, verified one-shot SSH installation, and instance revocation
- a reviewed engine, version, license, and capability catalog

The source catalog is not a runtime availability or support claim. Deployment
remains disabled until an adapter and its real-client end-to-end suite are
enabled for the exact frozen artifact. The first planned tunnel adapter is Xray
with VLESS, REALITY, and XHTTP.

## Architecture

Ulcer is one control plane with two core roles and a replaceable UI.

```text
browser -> Caddy -> UI -> REST + SSE -> HOST -> gRPC + mTLS -> INSTANCE -> engine
                                         |                         |
                                      Badger                    Badger
                                      durable                  in-memory
```

`HOST` owns desired state, identities, subscriptions, billing state, frozen
artifacts, and the durable traffic ledger. `INSTANCE` owns one isolated engine,
its rendered configuration, local counters, and immediate quota enforcement.
Every UI operation is reproducible through the public REST API.

### State Contract

- Every instance has an opaque UUID, a monotonic generation, and a canonical
  specification digest.
- A generation can never refer to two different digests.
- `HOST` sends a complete current snapshot after every reconnect.
- `INSTANCE` persists the snapshot before acknowledging it, then reconciles
  toward it.
- Lifecycle is level-triggered desired state; one-shot operations use idempotent
  command IDs.
- Status includes the applied generation, digest, phase, readiness, and failure
  reason.
- Artifact tags and commits are build inputs only. Runtime identity is an
  immutable digest.

### Traffic Contract

`INSTANCE` converts cumulative engine counters into deltas and stores them
before transmission. Meter events are identified by `(instance, principal,
quota epoch, sequence)`. `HOST` applies each event idempotently and acknowledges
the highest contiguous sequence. Unacknowledged events remain in the `INSTANCE`
write-ahead log.

eBPF is authoritative for process, cgroup, listener, and socket totals. It
cannot recover an authenticated user hidden inside shared encrypted or
multiplexed traffic. Per-user limits are enabled only when an adapter exposes
native user statistics, an instrumented stream wrapper, or explicit one-user
isolation.

### Isolation

- `HOST` is the only service allowed to mutate the `inet ulcer` nftables table.
- Engines run as non-root Podman containers without the Podman socket or
  `NET_ADMIN`.
- Every engine has a private filesystem, cgroup, secrets, and control socket.
- One process may contain multiple inbounds and transports; three transport
  profiles do not imply three processes.
- Engine control APIs bind to a Unix socket or private loopback interface.
- Production units use read-only roots, `NoNewPrivileges`, and PID and memory
  limits.

### Terminology

- **instance**: one reconciled engine runtime and its adapter
- **inbound**: one protocol listener inside an instance
- **endpoint**: an advertised address independent of physical placement
- **principal**: a proxy identity with managed credentials, limits, and usage
- **squad**: a reusable set of principals, endpoints, or routing policy
- **prober**: a remote observer that performs a real handshake through a named
  interface

## Protocols And Sources

The reviewed source catalog is [`versions/engines.json`](versions/engines.json).
[`versions/sources.json`](versions/sources.json) records the exact repository
gitlinks checked by `mage freeze`.

> [!IMPORTANT]
> A protocol or engine in the source catalog does not claim runtime
> availability. The API and UI must check `adapter_status` and the exact
> capability document for the frozen artifact before offering deployment.

- [Shadowsocks 2022](https://github.com/shadowsocks/shadowsocks-rust):
  [shadowsocks-rust](https://github.com/shadowsocks/shadowsocks-rust),
  [Xray-core](https://github.com/xtls/xray-core), and
  [sing-box](https://github.com/sagernet/sing-box)
- [Trojan](https://github.com/xtls/xray-core):
  [Xray-core](https://github.com/xtls/xray-core) and
  [sing-box](https://github.com/sagernet/sing-box)
- [Hysteria 2](https://github.com/apernet/hysteria):
  [Xray-core](https://github.com/xtls/xray-core),
  [sing-box](https://github.com/sagernet/sing-box), and
  [Hysteria](https://github.com/apernet/hysteria)
- [AnyTLS](https://github.com/anytls/anytls-go):
  [anytls-go](https://github.com/anytls/anytls-go) and
  [sing-box](https://github.com/sagernet/sing-box)
- [Mieru](https://github.com/enfein/mieru):
  [Mieru](https://github.com/enfein/mieru)
- [NaiveProxy](https://github.com/klzgrad/naiveproxy):
  [NaiveProxy](https://github.com/klzgrad/naiveproxy)
- [TUIC](https://github.com/tuic-protocol/tuic):
  [TUIC](https://github.com/tuic-protocol/tuic) and
  [sing-box](https://github.com/sagernet/sing-box)
- [Juicity](https://github.com/juicity/juicity):
  [Juicity](https://github.com/juicity/juicity)
- [VMess](https://github.com/xtls/xray-core):
  [Xray-core](https://github.com/xtls/xray-core) and
  [sing-box](https://github.com/sagernet/sing-box)
- [VLESS](https://github.com/xtls/xray-core):
  [Xray-core](https://github.com/xtls/xray-core) and
  [sing-box](https://github.com/sagernet/sing-box)
- [TrustTunnel](https://github.com/TrustTunnel/TrustTunnel):
  [TrustTunnel](https://github.com/TrustTunnel/TrustTunnel)
- [SSH](https://github.com/owenewans/owenclave): Ulcer's planned custom server
  uses restricted virtual principals rather than Linux users and permits only
  `direct-tcpip`. Shell, PTY, exec, subsystem, X11, agent forwarding, and reverse
  forwarding are rejected. The Owenclave server-side proxy is the reference to
  examine and replicate; its client is in the
  [Owenclave repository](https://github.com/owenewans/owenclave).
- [HTTP CONNECT](https://github.com/xtls/xray-core):
  [Xray-core](https://github.com/xtls/xray-core) and
  [sing-box](https://github.com/sagernet/sing-box)
- [SOCKS](https://github.com/xtls/xray-core):
  [Xray-core](https://github.com/xtls/xray-core) and
  [sing-box](https://github.com/sagernet/sing-box)
- [OLCRTC](https://github.com/openlibrecommunity/olcrtc):
  [OLCRTC](https://github.com/openlibrecommunity/olcrtc)
- [ShadowQUIC](https://github.com/spongebob888/shadowquic):
  [ShadowQUIC](https://github.com/spongebob888/shadowquic)
- [HTTP/3](https://github.com/xtls/xray-core):
  [Xray-core](https://github.com/xtls/xray-core) and
  [sing-box](https://github.com/sagernet/sing-box)

AnyTLS reference redistribution is blocked because the `anytls-go` repository
does not provide a license; redistribution requires an upstream license or
written permission. The TUIC upstream is GPL-3.0 and is currently retained as a
protocol specification, not treated as a maintained server binary. Any TUIC
implementation and distribution must satisfy the applicable GPL obligations.

Protocol distinctions that adapters must preserve:

- REALITY is a transport security layer, not a separate Trojan or VLESS
  protocol.
- XHTTP is an Xray transport and is not currently implemented by sing-box.
- XHTTP padding, VLESS Encryption padding, VMess global padding, multiplex
  padding, and AnyTLS padding are separate capabilities.
- Hysteria Gecko exists only in official Hysteria and remains experimental.
- An `h3` ALPN value does not make a custom QUIC proxy a generic HTTP/3 CONNECT
  implementation.
- Overlapping engines remain selectable according to each frozen capability
  document.

Adapters prefer a library only when upstream exposes a stable lifecycle and
accounting API. Otherwise, Ulcer supervises a frozen binary behind the same
private adapter contract. Xray remains necessary for VLESS, REALITY, XHTTP, and
their padding controls. OLCRTC is a library candidate, but immediate quotas
require streaming counters.

## Security Model

### Operator Access

Ulcer has one operator identity rather than an internal role hierarchy. Initial
enrolment requires the local setup token and a valid TOTP confirmation. Login
creates a random opaque session, and only its hash is persisted. Recovery codes
are single-use and shown once.

The setup token and generated CA private key are local credentials created with
mode `0600`. Production deployments should keep the data directory on encrypted
storage and back it up offline.

### Machine Identity

`HOST` and `INSTANCE` communicate through TLS 1.3 with mutually verified
certificates. An `INSTANCE` certificate carries its exact identity in a SPIFFE
URI SAN. Machine certificates never authorize REST or UI administration.

Automatic installation first displays the target SSH host-key fingerprint for
operator confirmation. `HOST` uses the submitted root password or private key
for that request only, never writes it to Badger or events, installs a
digest-pinned agent, and discards the credential after success or rollback. The
current automatic path supports `linux/amd64` Debian or Ubuntu hosts running
systemd and cgroup v2; SSH credentials are sent only after fingerprint
confirmation.
Deleting an instance removes its authorization and meter acknowledgement,
disconnects the active stream, and leaves historical aggregate usage intact.

### Artifact Trust

Instances run only Ulcer release artifacts whose digest, signature, provenance,
and SBOM match the frozen manifest. GitHub credentials remain in CI or `HOST`
secret storage and are never injected into engine containers.

### Known Boundaries

- Badger supports one active `HOST`; it is not a shared high-availability
  database.
- Generic eBPF accounting cannot provide per-user accounting for a shared,
  encrypted listener.
- A modified APK must be rebuilt or repackaged and signed again. Android package
  contents cannot remain mutable while retaining the original signature.
- Protocol engines retain their own licenses. A submodule or sidecar does not
  remove source-offer, notice, or copyleft obligations.

## Deployment

### Host And Build Requirements

Production assumes Debian 12 Bookworm or a supported Ubuntu installation with
`systemd`, cgroup v2, and rootful Podman. Minimal host packages are:

```text
podman aardvark-dns nftables ca-certificates curl iproute2 procps
uidmap fuse-overlayfs slirp4netns containernetworking-plugins
```

`uidmap`, `fuse-overlayfs`, and `slirp4netns` preserve the option to run
unprivileged utility containers. Engine processes remain non-root inside their
containers even though machine-level Podman is rootful.

The kernel must expose cgroup v2 and BTF for CO-RE eBPF. A builder additionally
needs:

```text
build-essential git make pkg-config unzip tar xz-utils
clang llvm bpftool libbpf-dev libelf-dev zlib1g-dev
protobuf-compiler ca-certificates curl
```

Go and Rust are pinned through official toolchain images instead of distribution
compiler packages. NaiveProxy/Chromium and Owenclave/Android use dedicated
builders rather than the generic engine image.

### Podman Boundary

`deploy/quadlet` contains templates for `/etc/containers/systemd/`. Every
`REPLACE_WITH_RELEASE_DIGEST` marker must be replaced before installation. After
installing the units:

```sh
sudo systemctl daemon-reload
sudo systemctl start ulcer-host.service ulcer-ui.service ulcer-caddy.service
```

An enrolled remote agent uses `ulcer-instance@.container`, with credentials and
`instance.env` under `/etc/ulcer/instances/<id>/`, and starts as
`ulcer-instance@<id>.service`. The directory and its `0600` credential files must
be owned by numeric UID/GID `65532:65532` because the agent does not run as root.

Quadlet applies `[Install]` at generator time. Production units do not update
floating tags automatically. Only Caddy receives `NET_BIND_SERVICE`. The future
`HOST` nftables/eBPF supervisor receives narrowly scoped privileges in its own
reviewed unit; engine instances never receive the Podman socket or `NET_ADMIN`.

### Trusted HTTPS

Set `ULCER_PUBLIC_ADDRESS` to the public IPv4, IPv6, or domain. Otherwise, the
installer detects the public IPv4 address. The value is stored alone in
`/etc/ulcer/caddy.env`; Caddy must not receive `HOST` setup tokens or other
control-plane secrets.

Let's Encrypt made six-day and IP address certificates generally available on
January 15, 2026. IP identifiers require the `shortlived` ACME profile and use
HTTP-01 or TLS-ALPN-01, so ports `80` and `443` must reach Caddy. The shipped
Caddyfile selects Let's Encrypt explicitly and Caddy 2.11.4 requests the
`shortlived` profile.

The current installer requires reachable public ingress for certificate
issuance and its final health check. It does not fall back to public plaintext
HTTP when ACME validation fails or the host is behind CGNAT.

### Firewall Ownership

Ulcer owns only the dedicated `inet ulcer` table. It never flushes the ruleset,
invokes iptables, or edits operator tables. Planned blue/green rollout uses an
atomic netlink transaction to switch destination mappings only after a real
protocol handshake succeeds.

## Artifacts And Releases

Git tags and commits are accepted as build inputs, never runtime identities.
Every release manifest records the upstream URL, tag, full commit, patch digest,
toolchain image digest, architectures, OCI digest, SBOM, license report,
provenance, signature, config schema, and adapter capability set. Updating a Git
tag never changes a deployed artifact implicitly.

Releases provide:

- native `ulcer-host` and `ulcer-instance` binaries for Linux, macOS, and Windows
  on amd64 and arm64
- a deployment bundle containing Quadlet, Caddy, nftables, catalog, and source
  metadata
- signed `ulcer-host`, `ulcer-instance`, `ulcer-ui`, and `ulcer-caddy` images in
  GHCR
- `linux/amd64` runtime images; native binaries are also built for arm64
- SHA-256 checksums, SPDX SBOMs, and build provenance
- semantic tags beginning with `v0.0.1`

Download immutable artifacts from
[GitHub Releases](https://github.com/owenewans/ulcer/releases). The UI and Caddy
are runtime images, not standalone Ulcer binaries.

Upstreams are commit-pinned Git submodules. Initialize only the source required
for an adapter build, for example:

```sh
git submodule update --init engines/xray-core
git submodule update --init --recursive clients/owenclave
```

Owenclave contains nested submodules and therefore requires `--recursive`.

## Development

```sh
go run ./cmd/ulcer-host

cd web
bun install --frozen-lockfile
bun run dev
```

The first launch creates `data/host/setup.token` with mode `0600`. Open
`http://127.0.0.1:3000`, enter the token, and confirm a TOTP code. Development
uses local HTTP; production terminates trusted HTTPS at Caddy.

Run the complete checks with:

```sh
mage check
mage freeze
mage build
mage e2e
mage podman:smoke
```

## Roadmap

### Foundation

- `HOST` Badger store, TOTP bootstrap, REST API, and SSE event stream
- `INSTANCE` mTLS stream, desired-state generations, and status reconciliation
- frozen artifact catalog, capability model, and release provenance
- Bun and Next.js operator UI with Podman deployment

### First Real Tunnel

- Xray adapter built from the pinned submodule
- VLESS and REALITY inbound with universal subscription output
- native Xray usage ledger and bounded quota leases
- real-client transfer, disconnect, rollback, and restart end-to-end tests

### Engine Coverage

- sing-box overlap adapter
- Shadowsocks 2022, Trojan, VMess, HTTP CONNECT, SOCKS, and Hysteria 2
- AnyTLS, Mieru, NaiveProxy, TUIC, Juicity, and TrustTunnel
- embedded OLCRTC, ShadowQUIC, and restricted virtual-principal SSH

### Product Surface

- users, prices, plans, Crypto Pay, and FreeKassa
- Telegram storefront and independent supergroup alert bots
- endpoint routing, cascades, Blocky, internal and external squads, and metrics
- Owenclave branded builds with a customer package ID and signing key
- configurable universal subscription paths and formats

### Reachability Intelligence

- signed probes bound to a selected interface or network namespace
- a protocol-level handshake and small transfer rather than TCP-only checks
- operator and network profiles with TTL, quorum, and hysteresis
- per-audience subscription filtering without global false positives

Each phase becomes releasable only after unit, fault-injection, real-core,
real-client, Podman, and browser tests pass for the capability being enabled.

## References

- [Engine catalog](versions/engines.json)
- [Frozen source catalog](versions/sources.json)
- [GitHub Releases](https://github.com/owenewans/ulcer/releases)
- [Owenclave client and server-side proxy reference](https://github.com/owenewans/owenclave)
- [Project license](LICENSE)
