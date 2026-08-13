#!/bin/sh
set -eu

repository=${ULCER_REPOSITORY:-owenewans/ulcer}
version=${ULCER_VERSION:-v0.0.4}
release_url="https://github.com/${repository}/releases/download/${version}"
bundle="ulcer-deployment-${version}.tar.gz"

log() {
	printf '[ulcer] %s\n' "$*"
}

die() {
	printf '[ulcer] error: %s\n' "$*" >&2
	exit 1
}

cleanup() {
	rm -rf "${workdir:-}"
}

check_ports() {
	command -v ss >/dev/null 2>&1 || return 0
	[ -r /etc/containers/systemd/ulcer-caddy.container ] && return 0
	listeners=$(ss -H -ltn | awk '$4 ~ /:80$/ || $4 ~ /:443$/')
	[ -z "$listeners" ] || die "ports 80 or 443 are already in use:\n$listeners"
}

[ "$(id -u)" -eq 0 ] || die "run as root"
case "$version" in
	v[0-9]*.[0-9]*.[0-9]*) ;;
	*) die "ULCER_VERSION must be a semantic version such as v0.0.1" ;;
esac

public_address=${ULCER_PUBLIC_ADDRESS:-}
if [ -z "$public_address" ]; then
	public_address=$(curl --fail --silent --show-error --location --max-time 15 https://api.ipify.org) ||
		die "could not detect the public address; set ULCER_PUBLIC_ADDRESS"
fi
case "$public_address" in
	*[!A-Za-z0-9._:-]* | "" | -*) die "invalid ULCER_PUBLIC_ADDRESS" ;;
esac

workdir=$(mktemp -d)
trap cleanup EXIT INT TERM

log "downloading ${version} release metadata"
curl --fail --silent --show-error --location --retry 3 \
	--output "$workdir/SHA256SUMS" "$release_url/SHA256SUMS"
curl --fail --silent --show-error --location --retry 3 \
	--output "$workdir/$bundle" "$release_url/$bundle"
expected=$(awk -v name="$bundle" '$2 == name { print $1 }' "$workdir/SHA256SUMS")
[ -n "$expected" ] || die "$bundle is absent from SHA256SUMS"
actual=$(sha256sum "$workdir/$bundle" | awk '{ print $1 }')
[ "$actual" = "$expected" ] || die "release bundle checksum mismatch"

[ -r /etc/os-release ] || die "unsupported operating system"
os_id=$(awk -F= '$1 == "ID" { gsub(/"/, "", $2); print $2 }' /etc/os-release)
case "$os_id" in
	debian | ubuntu) ;;
	*) die "only Debian and Ubuntu are currently supported" ;;
esac

check_ports

log "installing Podman and host dependencies"
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install --yes --no-install-recommends \
	ca-certificates containernetworking-plugins curl fuse-overlayfs iproute2 \
	aardvark-dns nftables podman procps slirp4netns uidmap
check_ports

[ -r /sys/fs/cgroup/cgroup.controllers ] || die "cgroup v2 is required"
if [ ! -x /usr/libexec/podman/quadlet ] && [ ! -x /usr/lib/podman/quadlet ]; then
	die "the installed Podman package does not provide Quadlet"
fi

mkdir -p "$workdir/release"
tar --extract --gzip --file "$workdir/$bundle" --directory "$workdir/release"
[ -d "$workdir/release/ulcer/quadlet" ] || die "invalid deployment bundle"
instance_unit="$workdir/release/ulcer/quadlet/ulcer-instance@.container"
[ -r "$instance_unit" ] || die "release bundle has no INSTANCE unit"
instance_image=$(awk -F= '$1 == "Image" { print $2 }' "$instance_unit")
case "$instance_image" in
	*[!A-Za-z0-9._:/@-]* | *@*@* | *://* | *//* | */../* | */./*) die "release INSTANCE image is invalid" ;;
esac
instance_repository=${instance_image%%@sha256:*}
[ "$instance_image" != "$instance_repository" ] || die "release INSTANCE image is not digest-pinned"
instance_digest=${instance_image##*@sha256:}
[ -n "$instance_repository" ] || die "release INSTANCE image repository is invalid"
case "$instance_repository" in
	*/*) ;;
	*) die "release INSTANCE image repository is not fully qualified" ;;
esac
instance_registry=${instance_repository%%/*}
case "$instance_registry" in
	localhost | *.* | *:*) ;;
	*) die "release INSTANCE image registry is not fully qualified" ;;
esac
[ "${#instance_digest}" -eq 64 ] || die "release INSTANCE image digest is invalid"
case "$instance_digest" in
	*[!0-9a-f]*) die "release INSTANCE image digest is invalid" ;;
esac

case "$public_address" in
	*:*) public_grpc="[$public_address]:8443" ;;
	*) public_grpc="$public_address:8443" ;;
esac

log "installing digest-pinned Quadlet units"
install -d -m 0700 /etc/ulcer
install -d -m 0755 /etc/containers/systemd
for unit in "$workdir/release/ulcer/quadlet/"*; do
	install -m 0644 "$unit" "/etc/containers/systemd/$(basename "$unit")"
done
printf 'ULCER_HTTP_ADDR=0.0.0.0:8080\nULCER_GRPC_ADDR=0.0.0.0:8443\nULCER_DATA_DIR=/var/lib/ulcer\nULCER_PUBLIC_NAME=%s\nULCER_PUBLIC_GRPC=%s\nULCER_INSTANCE_IMAGE=%s\n' \
	"$public_address" "$public_grpc" "$instance_image" > "$workdir/host.env"
printf 'ULCER_PUBLIC_ADDRESS=%s\n' "$public_address" > "$workdir/caddy.env"
install -m 0600 "$workdir/host.env" /etc/ulcer/host.env
install -m 0600 "$workdir/caddy.env" /etc/ulcer/caddy.env

for unit in "$workdir/release/ulcer/quadlet/"*.container; do
	image=$(awk -F= '$1 == "Image" { print $2 }' "$unit")
	[ -n "$image" ] || continue
	log "pulling $image"
	podman pull "$image"
done

systemctl daemon-reload
systemctl cat ulcer-host.service >/dev/null 2>&1 || die "Quadlet did not generate ulcer-host.service"
log "starting Ulcer"
systemctl restart ulcer-host.service
systemctl restart ulcer-ui.service
systemctl restart ulcer-caddy.service

attempt=0
until podman exec ulcer-host curl --fail --silent http://127.0.0.1:8080/healthz >/dev/null 2>&1; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 60 ]; then
		journalctl --no-pager --lines 80 -u ulcer-host.service >&2 || true
		die "HOST did not become healthy"
	fi
	sleep 1
done

attempt=0
until podman exec ulcer-ui node -e \
	'fetch("http://127.0.0.1:3000/").then(response => { if (!response.ok) process.exit(1) }).catch(() => process.exit(1))' \
	>/dev/null 2>&1; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 60 ]; then
		journalctl --no-pager --lines 80 -u ulcer-ui.service >&2 || true
		die "UI did not become healthy"
	fi
	sleep 1
done

case "$public_address" in
	*:*) public_url="https://[$public_address]/" ;;
	*) public_url="https://$public_address/" ;;
esac
attempt=0
until curl --fail --silent --show-error --location --noproxy '*' \
	--max-time 15 "${public_url}healthz" >/dev/null 2>&1; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 120 ]; then
		journalctl --no-pager --lines 80 -u ulcer-caddy.service >&2 || true
		die "public HTTPS endpoint did not become healthy"
	fi
	sleep 1
done

printf '%s\n' "$version" > "$workdir/install.version"
install -m 0644 "$workdir/install.version" /etc/ulcer/install.version
setup_token=$(podman exec ulcer-host sh -c 'cat /var/lib/ulcer/setup.token 2>/dev/null || true')
log "installation complete"
printf '\nURL: %s\n' "$public_url"
if [ -n "$setup_token" ]; then
	printf 'Setup token: %s\n' "$setup_token"
else
	printf 'Setup is already complete; no setup token is available.\n'
fi
printf 'Status: systemctl status ulcer-host ulcer-ui ulcer-caddy\n'
