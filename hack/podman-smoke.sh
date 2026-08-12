#!/bin/sh
set -eu

export NO_PROXY=localhost,127.0.0.1,::1
export no_proxy=localhost,127.0.0.1,::1
export HTTP_PROXY=
export HTTPS_PROXY=
export ALL_PROXY=
export http_proxy=
export https_proxy=
export all_proxy=

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
suffix=$$
host_image="localhost/ulcer-host-smoke:${suffix}"
instance_image="localhost/ulcer-instance-smoke:${suffix}"
ui_image="localhost/ulcer-ui-smoke:${suffix}"
caddy_image="localhost/ulcer-caddy-smoke:${suffix}"
host_name="ulcer-host-smoke-${suffix}"

cleanup() {
	podman rm --force "$host_name" >/dev/null 2>&1 || true
	podman rmi --force "$host_image" "$instance_image" "$ui_image" "$caddy_image" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

podman build --file "$root/deploy/Containerfile.host" --tag "$host_image" "$root"
podman build --file "$root/deploy/Containerfile.instance" --tag "$instance_image" "$root"
podman build --file "$root/deploy/Containerfile.ui" --tag "$ui_image" "$root"
podman build --file "$root/deploy/Containerfile.caddy" --tag "$caddy_image" "$root"
podman run --rm --env ULCER_PUBLIC_ADDRESS=127.0.0.1 "$caddy_image" \
	validate --config /etc/caddy/Caddyfile --adapter caddyfile

podman run --detach \
	--name "$host_name" \
	--read-only \
	--cap-drop all \
	--security-opt no-new-privileges \
	--env ULCER_SETUP_TOKEN=podman-smoke-token \
	--env ULCER_DATA_DIR=/tmp/ulcer \
	--env ULCER_PUBLIC_NAME=localhost \
	--publish 127.0.0.1::8080 \
	"$host_image" >/dev/null

mapping=$(podman port "$host_name" 8080/tcp)
port=${mapping##*:}
attempt=0
until curl --fail --silent "http://127.0.0.1:${port}/healthz" >/dev/null; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 30 ]; then
		podman logs "$host_name"
		exit 1
	fi
	sleep 1
done
