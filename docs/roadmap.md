# roadmap

## foundation

- HOST Badger store, TOTP bootstrap, REST API and SSE event stream
- INSTANCE mTLS stream, desired-state generations and status reconciliation
- frozen artifact catalog, capability model and release provenance
- Bun + Next.js operator UI and Podman deployment

## first real tunnel

- Xray adapter built from the pinned submodule
- VLESS + REALITY inbound and universal subscription output
- native Xray usage ledger and bounded quota leases
- real client transfer, disconnect, rollback and restart E2E tests

## engine coverage

- sing-box overlap adapter
- Shadowsocks 2022, Trojan, VMess, HTTP CONNECT, SOCKS and Hysteria 2
- AnyTLS, Mieru, NaiveProxy, TUIC, Juicity and TrustTunnel
- embedded OLCRTC, ShadowQUIC and restricted virtual-principal SSH

## product surface

- users, prices, plans, Crypto Pay and FreeKassa
- Telegram storefront and independent supergroup alert bots
- endpoint routing, cascades, Blocky, internal/external squads and metrics
- Owenclave branded builds with customer package ID and signing key
- configurable universal subscription paths and formats

## reachability intelligence

- signed probes bound to a selected interface or network namespace
- protocol-level handshake and small transfer, not TCP-only checks
- operator/network profiles, TTL, quorum and hysteresis
- per-audience subscription filtering without global false positives

Each phase becomes releaseable only after unit, fault-injection, real-core,
real-client, Podman and browser tests pass for the capability being enabled.
