import { strToU8, zipSync } from "fflate";
import type { Credentials, Instance, Meta } from "@/lib/api";

type ManualBundle = {
  instance: Instance;
  credentials: Credentials;
  meta: Meta;
};

function safeName(value: string, fallback: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "") || fallback;
}

function isDigestPinnedImage(image: string): boolean {
  if (!/^[a-z0-9](?:[a-z0-9._:/-]*[a-z0-9])?@sha256:[a-f0-9]{64}$/.test(image) || image.split("@").length !== 2 || image.includes("://") || image.includes("//")) return false;
  const name = image.slice(0, image.indexOf("@sha256:"));
  const slash = name.indexOf("/");
  if (slash < 1) return false;
  const registry = name.slice(0, slash);
  if (registry !== "localhost" && !registry.includes(".") && !registry.includes(":")) return false;
  return name.slice(slash + 1).split("/").every((component) => /^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$/.test(component));
}

export function instanceBundleFilename(name: string, id: string): string {
  return `ulcer-instance-${safeName(name, id.slice(0, 8))}.zip`;
}

export function createInstanceBundle({ instance, credentials, meta }: ManualBundle): Uint8Array {
  const instanceDirectory = `/etc/ulcer/instances/${instance.id}`;
  const unitStem = `ulcer-instance-${safeName(instance.id, "agent")}`;
  const unitFilename = `${unitStem}.service`;
  const pinnedImage = isDigestPinnedImage(meta.instance_image);
  const credentialDirectory = pinnedImage ? "/run/ulcer-identity" : instanceDirectory;
  const dataDirectory = pinnedImage ? "/var/lib/ulcer-instance" : instanceDirectory;
  const environment = [
    `ULCER_INSTANCE_ID=${instance.id}`,
    `ULCER_INSTANCE_NAME=${instance.name}`,
    `ULCER_HOST_GRPC=${meta.grpc_endpoint}`,
    `ULCER_HOST_SERVER_NAME=${meta.grpc_server_name}`,
    `ULCER_INSTANCE_DATA_DIR=${dataDirectory}`,
    `ULCER_INSTANCE_CERT=${credentialDirectory}/instance.crt`,
    `ULCER_INSTANCE_KEY=${credentialDirectory}/instance.key`,
    `ULCER_INSTANCE_CA=${credentialDirectory}/ca.crt`,
    "",
  ].join("\n");
  const installInstructions = pinnedImage
    ? [
        "Automated installation (Linux with systemd and rootful Podman):",
        "  sudo sh install.sh",
        `  systemctl status ${unitFilename}`,
        "",
        `The installer copies credentials to ${instanceDirectory}, installs ${unitFilename},`,
        "and starts the hardened, digest-pinned container service.",
      ].join("\n")
    : [
        "This host did not advertise a digest-pinned instance image, so this archive does not",
        "install a container. Do not substitute a mutable image tag. After obtaining a verified",
        "ulcer-instance executable from a trusted artifact source, install this archive with:",
        `  sudo install -d -m 0700 -o 65532 -g 65532 ${instanceDirectory}`,
        `  sudo install -m 0600 -o root -g root instance.env ${instanceDirectory}/instance.env`,
        `  sudo install -m 0600 -o 65532 -g 65532 instance.crt instance.key ca.crt ${instanceDirectory}/`,
        "Run the executable as UID/GID 65532 from a systemd service that loads:",
        `  EnvironmentFile=${instanceDirectory}/instance.env`,
        "The environment file contains the exact control endpoint and credential paths.",
      ].join("\n");
  const readme = [
    "Ulcer instance bundle",
    "=====================",
    "",
    `Name: ${instance.name}`,
    `Instance ID: ${instance.id}`,
    `Control endpoint: ${meta.grpc_endpoint}`,
    `TLS server name: ${meta.grpc_server_name}`,
    "",
    "The private key in this archive was returned once. Keep the archive private, use mode",
    "0600 for credential files, and delete surplus copies after installation.",
    "",
    installInstructions,
    "",
  ].join("\n");
  const files: Record<string, Uint8Array> = {
    "instance.env": strToU8(environment),
    "instance.crt": strToU8(credentials.certificate_pem),
    "instance.key": strToU8(credentials.private_key_pem),
    "ca.crt": strToU8(credentials.ca_pem),
    "README.txt": strToU8(readme),
  };

  if (pinnedImage) {
    const service = [
      "[Unit]",
      `Description=Ulcer instance agent ${instance.id}`,
      "After=network-online.target",
      "Wants=network-online.target",
      "",
      "[Service]",
      "Type=simple",
      `ExecStart=/usr/bin/podman run --rm --name=${unitStem} --pull=never --network=bridge --read-only --user=65532:65532 --security-opt=no-new-privileges --cap-drop=all --pids-limit=128 --memory=256m --cpus=1 --tmpfs=/tmp:rw,noexec,nosuid,nodev,size=16m --tmpfs=/var/lib/ulcer-instance:rw,noexec,nosuid,nodev,size=32m,uid=65532,gid=65532,mode=0700 --env-file=${instanceDirectory}/instance.env --volume=${instanceDirectory}:/run/ulcer-identity:ro,Z ${meta.instance_image}`,
      `ExecStop=-/usr/bin/podman stop --time=10 ${unitStem}`,
      `ExecStopPost=-/usr/bin/podman rm --force ${unitStem}`,
      "Restart=on-failure",
      "RestartSec=5s",
      "TimeoutStartSec=300s",
      "TimeoutStopSec=30s",
      "TasksMax=160",
      "MemoryMax=384M",
      "CPUQuota=100%",
      "NoNewPrivileges=yes",
      "",
      "[Install]",
      "WantedBy=multi-user.target",
      "",
    ].join("\n");
    const installScript = [
      "#!/bin/sh",
      "set -eu",
      "umask 077",
      "",
      'if [ "$(id -u)" -ne 0 ]; then',
      '  printf "%s\\n" "run this installer as root" >&2',
      "  exit 1",
      "fi",
      "command -v podman >/dev/null 2>&1 || { printf \"%s\\n\" \"podman is required\" >&2; exit 1; }",
      "command -v systemctl >/dev/null 2>&1 || { printf \"%s\\n\" \"systemd is required\" >&2; exit 1; }",
      'case "$(uname -m)" in x86_64|amd64) ;; *) printf "%s\\n" "this release image currently requires linux/amd64" >&2; exit 1 ;; esac',
      'script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)',
      `target=${instanceDirectory}`,
      'install -d -m 0700 -o 65532 -g 65532 "$target"',
      'install -m 0600 -o root -g root "$script_dir/instance.env" "$target/instance.env"',
      'for file in instance.crt instance.key ca.crt; do',
      '  install -m 0600 -o 65532 -g 65532 "$script_dir/$file" "$target/$file"',
      "done",
      `podman pull --quiet ${meta.instance_image}`,
      `install -m 0644 -o root -g root "$script_dir/${unitFilename}" "/etc/systemd/system/${unitFilename}"`,
      "systemctl daemon-reload",
      `systemctl enable --now ${unitFilename}`,
      `systemctl --no-pager --full status ${unitFilename}`,
      "",
    ].join("\n");
    files[unitFilename] = strToU8(service);
    files["install.sh"] = strToU8(installScript);
  }

  return zipSync(files, { level: 6 });
}
