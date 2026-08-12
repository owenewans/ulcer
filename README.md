<div align="center">

# ulcer

protocol infrastructure without ceremony. frozen cores, isolated instances and one reproducible api.

<a href="https://count.owenewans.org/owenewans/ulcer?theme=moebooru-h&notitle"><img src="https://count.owenewans.org/owenewans/ulcer?theme=moebooru-h&notitle" alt="repository views"></a>

`go` `badger` `grpc` `mtls` `next.js` `bun` `shadcn` `caddy` `podman` `nftables`

</div>

Ulcer separates the control plane into `HOST` and independently reconciled
`INSTANCE` agents. The UI is only an API client. Every engine is frozen to
reviewed source, built by Ulcer and deployed by immutable artifact digest.

Upstreams are commit-pinned Git submodules. Initialize only what an adapter
build needs, for example `git submodule update --init engines/xray-core`.
Owenclave has nested modules, so initialize it with
`git submodule update --init --recursive clients/owenclave`.

current foundation:
- local-token onboarding and a single TOTP operator
- hashed, expiring, HttpOnly sessions and single-use recovery codes
- Badger desired state, generation and event ledger
- bidirectional gRPC control stream over TLS 1.3 mTLS
- SPIFFE-bound certificate identity per instance
- replay-safe contiguous traffic acknowledgements
- REST API, live SSE and responsive Next.js operator console
- Podman images, Quadlet templates and Caddy IP/domain HTTPS
- reviewed engine/version/license/capability catalog

The catalog is not a support claim. Runtime deployment stays disabled until an
adapter and its real-client E2E suite are enabled. The first tunnel adapter is
Xray with VLESS + REALITY + XHTTP; it is tracked in the
[roadmap](docs/roadmap.md), not faked in this foundation.

architecture:
```text
browser -> Caddy -> UI -> REST/SSE -> HOST -> gRPC/mTLS -> INSTANCE -> engine
                                      |                        |
                                   Badger                   Badger
                                   durable                 in-memory
```

- HOST owns desired state, operator auth, artifacts, subscriptions and totals
- INSTANCE owns one engine runtime, local reconciliation and quota enforcement
- nftables belongs only to HOST; an engine never receives `NET_ADMIN`
- eBPF provides exact cgroup/socket totals, not invented per-user attribution
- native engine APIs or instrumented streams provide user accounting

development:
```sh
go run ./cmd/ulcer-host

cd web
bun install --frozen-lockfile
bun run dev
```

The first launch creates `data/host/setup.token` with mode `0600`. Open
`http://127.0.0.1:3000`, enter that token and confirm a TOTP code. Development
uses local HTTP; production terminates trusted HTTPS at Caddy.

checks:
```sh
mage check
mage freeze
mage build
mage e2e
mage podman:smoke
```

releases:
- native `ulcer-host` and `ulcer-instance` binaries for Linux, macOS and
  Windows on amd64/arm64
- a deployment bundle with Quadlet, Caddy and nftables configuration
- signed `ulcer-host`, `ulcer-instance`, `ulcer-ui` and `ulcer-caddy` images
  in GHCR, plus SHA-256 checksums, SPDX SBOMs and build provenance
- semantic release tags starting with `v0.0.1`

Download immutable artifacts from [GitHub Releases](https://github.com/owenewans/ulcer/releases).
The UI and Caddy are distributed as runtime images rather than pretending they
are standalone Ulcer binaries.

deployment:
- Debian Bookworm, cgroup v2 and rootful Podman Quadlet
- Caddy 2.11.4 with an explicit Let's Encrypt `shortlived` ACME profile
- trusted IP certificates when ports 80/443 are publicly reachable
- SSH tunnel to loopback as the safe fallback behind CGNAT or blocked ports
- release image digests must replace every `REPLACE_WITH_RELEASE_DIGEST` marker

See [deployment details](docs/deployment.md). Do not pipe an unaudited installer
into a root shell; download the release manifest, verify its signature and
SHA-256, then execute the pinned installer.

protocols:
- Shadowsocks 2022, Trojan, Hysteria 2, AnyTLS, Mieru and NaiveProxy
- TUIC, Juicity, VMess, VLESS, TrustTunnel and restricted SSH
- HTTP CONNECT, SOCKS, OLCRTC, ShadowQUIC and HTTP/3
- overlapping engines remain selectable per frozen capability document

research notes and exact pins live in
[`versions/engines.json`](versions/engines.json) and
[`versions/sources.json`](versions/sources.json), with
[`docs/protocols.md`](docs/protocols.md). AnyTLS reference redistribution is
blocked until upstream supplies a license. TUIC upstream is currently treated
as a specification, not a maintained server binary.

links:
- [architecture](docs/architecture.md)
- [security model](docs/security.md)
- [protocol policy](docs/protocols.md)
- [roadmap](docs/roadmap.md)
- [owenclave](https://github.com/owenewans/owenclave)
