# deployment

## host requirements

Production assumes Debian 12 Bookworm, systemd, cgroup v2 and rootful Podman.
The minimal host packages are:

```text
podman aardvark-dns nftables ca-certificates curl iproute2 procps
uidmap fuse-overlayfs slirp4netns containernetworking-plugins
```

`uidmap`, `fuse-overlayfs` and `slirp4netns` preserve the option to run
unprivileged utility containers. Engine instances are rootless inside their
containers even though the machine-level Podman service is rootful.

For a fresh Debian or Ubuntu host, the supported installer verifies and deploys
the digest-pinned release bundle:

```sh
curl -fsSL https://raw.githubusercontent.com/owenewans/ulcer/master/install.sh | sh
```

Set `ULCER_PUBLIC_ADDRESS` to a domain or public IP to override automatic IPv4
detection. On first installation, the installer refuses occupied ports 80/443;
upgrades reuse the installed Caddy units. It never flushes the host firewall.

The host kernel must expose cgroup v2 and BTF for CO-RE eBPF. A builder needs:

```text
build-essential git make pkg-config unzip tar xz-utils
clang llvm bpftool libbpf-dev libelf-dev zlib1g-dev
protobuf-compiler ca-certificates curl
```

Go and Rust are pinned from their official toolchain images rather than Debian
Bookworm's older compiler packages. NaiveProxy/Chromium and Owenclave/Android
use dedicated builders; they do not belong in the generic engine image.

## podman boundary

`deploy/quadlet` contains templates for `/etc/containers/systemd/`. Replace all
release digest markers before installation, then run:

```sh
sudo systemctl daemon-reload
sudo systemctl start ulcer-host.service ulcer-ui.service ulcer-caddy.service
```

An enrolled remote agent uses `ulcer-instance@.container` with credentials and
`instance.env` in `/etc/ulcer/instances/<id>/`, then starts as
`ulcer-instance@<id>.service`. That directory and its `0600` credential files
must be owned by numeric UID/GID `65532:65532`, because the agent never runs as
root.

Quadlet applies `[Install]` at generator time. Production units use read-only
roots, `NoNewPrivileges`, PID/memory limits and no automatic floating-tag update.
Only Caddy receives `NET_BIND_SERVICE`. HOST's future nftables/eBPF supervisor
receives narrowly scoped host privileges in its own reviewed unit; engine
instances never receive the Podman socket or `NET_ADMIN`.

## ip https

Let's Encrypt made six-day and IP address certificates generally available on
January 15, 2026. IP identifiers require the `shortlived` ACME profile and use
HTTP-01 or TLS-ALPN-01, so ports 80 and 443 must reach Caddy.

Set `ULCER_PUBLIC_ADDRESS` to the public IPv4, IPv6 or domain. The shipped
Caddyfile selects Let's Encrypt explicitly and requests `profile shortlived`.
Caddy 2.11.4 supports ACME profiles and IP identifiers.
Store that value alone in `/etc/ulcer/caddy.env`; Caddy must not receive HOST
setup tokens or other control-plane secrets.

If the address is behind CGNAT, validation fails, or ingress is blocked, do not
fall back to public plain HTTP. Bind the setup UI to loopback and use SSH:

```sh
ssh -L 8448:127.0.0.1:8080 root@server
```

Then complete setup at `http://127.0.0.1:8448`. Add a domain later from the UI
and restart Caddy only after the new certificate is ready.

## firewall ownership

Ulcer owns only the dedicated `inet ulcer` table. It never flushes the ruleset,
invokes iptables or edits operator tables. Blue/green instance rollout will use
atomic netlink transactions to switch destination mappings after a real
protocol handshake succeeds.

## artifacts

Git tags and commits are accepted as build inputs, never runtime identities.
Each release manifest must include source commit, patch digest, toolchain image,
architectures, OCI digest, SBOM, license report, provenance, signature, config
schema and adapter capability set. GitHub credentials stay in CI or HOST secret
storage and are never mounted into an instance.
