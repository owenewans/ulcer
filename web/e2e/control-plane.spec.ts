import { createHmac } from "node:crypto";
import { spawn, type ChildProcess } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { expect, test } from "@playwright/test";
import { strFromU8, unzipSync } from "fflate";
import type { Credentials, Instance, Meta } from "@/lib/api";

test("onboards operator and reconciles an mTLS instance", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Frontier Enterprise Proxy panel", exact: true })).toBeVisible();

  await page.getByLabel("setup token").fill("ulcer-e2e-setup-token");
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Bind your authenticator", exact: true })).toBeVisible();
  await expect(page.getByRole("img", { name: "Authenticator QR code" })).toBeVisible();

  const secret = (await page.getByTestId("totp-secret").textContent()) ?? "";
  await page.getByLabel("six digit code").fill(totp(secret));
  await page.getByRole("button", { name: "Verify authenticator" }).click();
  await expect(page.getByRole("heading", { name: "Store your recovery codes", exact: true })).toBeVisible();
  const recoveryCodes = await page.locator("code").allTextContents();
  expect(recoveryCodes).toHaveLength(10);
  const recoveryDownloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download recovery codes" }).click();
  const recoveryDownload = await recoveryDownloadPromise;
  expect(recoveryDownload.suggestedFilename()).toBe("ulcer-recovery-codes.txt");
  const recoveryPath = await recoveryDownload.path();
  expect(recoveryPath).not.toBeNull();
  const recoveryContents = await readFile(recoveryPath!, "utf8");
  for (const recoveryCode of recoveryCodes) expect(recoveryContents).toContain(recoveryCode);
  await page.getByRole("button", { name: "I stored them safely" }).click();

  await expect(page.getByRole("heading", { name: "Overview", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: /Source e2e-ref/ })).toBeVisible();

  const metaResponse = await page.request.get("/api/v1/meta");
  expect(metaResponse.ok()).toBe(true);
  const meta = (await metaResponse.json()) as Meta;
  expect(meta.source_ref).toBe("e2e-ref");

  await page.getByRole("button", { name: "Instances" }).click();
  await page.getByRole("button", { name: "Enroll first instance" }).click();
  await page.getByRole("button", { name: "Install over SSH" }).click();
  await expect(page.getByLabel("username")).toHaveValue("root");
  await expect(page.getByLabel("root password")).toBeVisible();
  await page.getByLabel("instance name").fill("ssh-probe-check");
  await page.getByLabel("host").fill("127.0.0.1");
  const probeResponsePromise = page.waitForResponse((response) =>
    response.url().endsWith("/api/v1/ssh/host-key") && response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Probe host key" }).click();
  expect((await probeResponsePromise).status()).toBe(400);
  await expect(page.getByRole("alert")).toContainText(/private|special|target/i);
  await page.getByRole("button", { name: "Enrollment methods" }).click();
  await page.getByRole("button", { name: "Download manual bundle" }).click();
  await page.getByLabel("instance name").fill("e2e-edge-01");

  const enrollmentResponse = page.waitForResponse((response) =>
    response.url().endsWith("/api/v1/instances") && response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Create bundle" }).click();
  const enrollment = (await (await enrollmentResponse).json()) as {
    instance: Instance;
    credentials: Credentials;
  };
  await expect(page.getByRole("heading", { name: "Manual bundle ready" })).toBeVisible();

  const bundleDownloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download ZIP bundle" }).click();
  const bundleDownload = await bundleDownloadPromise;
  expect(bundleDownload.suggestedFilename()).toBe("ulcer-instance-e2e-edge-01.zip");
  const bundlePath = await bundleDownload.path();
  expect(bundlePath).not.toBeNull();
  const bundle = unzipSync(new Uint8Array(await readFile(bundlePath!)));
  const serviceName = `ulcer-instance-${enrollment.instance.id}.service`;
  expect(Object.keys(bundle)).toEqual(expect.arrayContaining(["instance.env", "instance.crt", "instance.key", "ca.crt", "README.txt", "install.sh", serviceName]));
  expect(strFromU8(bundle["instance.crt"])).toBe(enrollment.credentials.certificate_pem);
  expect(strFromU8(bundle["instance.key"])).toBe(enrollment.credentials.private_key_pem);
  expect(strFromU8(bundle["ca.crt"])).toBe(enrollment.credentials.ca_pem);
  const instanceEnvironment = strFromU8(bundle["instance.env"]);
  expect(instanceEnvironment).toContain(`ULCER_INSTANCE_ID=${enrollment.instance.id}`);
  expect(instanceEnvironment).toContain("ULCER_INSTANCE_NAME=e2e-edge-01");
  expect(instanceEnvironment).toContain(`ULCER_HOST_GRPC=${meta.grpc_endpoint}`);
  expect(instanceEnvironment).toContain(`ULCER_HOST_SERVER_NAME=${meta.grpc_server_name}`);
  const installScript = strFromU8(bundle["install.sh"]);
  expect(installScript).toContain(`podman pull --quiet ${meta.instance_image}`);
  expect(installScript).toContain(`systemctl enable --now ${serviceName}`);
  const service = strFromU8(bundle[serviceName]);
  expect(service).toContain(`--read-only --user=65532:65532`);
  expect(service).toContain("--security-opt=no-new-privileges --cap-drop=all");
  expect(service).toContain(meta.instance_image);

  const agentDirectory = await mkdtemp(join(tmpdir(), "ulcer-instance-e2e-"));
  let agent: ChildProcess | undefined;
  let agentExit: Promise<number | null> | undefined;
  try {
    await Promise.all([
      writeFile(join(agentDirectory, "instance.crt"), bundle["instance.crt"], { mode: 0o600 }),
      writeFile(join(agentDirectory, "instance.key"), bundle["instance.key"], { mode: 0o600 }),
      writeFile(join(agentDirectory, "ca.crt"), bundle["ca.crt"], { mode: 0o600 }),
    ]);
    agent = spawn("go", ["run", "./cmd/ulcer-instance"], {
      cwd: resolve(process.cwd(), ".."),
      stdio: "pipe",
      env: {
        ...process.env,
        ULCER_INSTANCE_ID: enrollment.instance.id,
        ULCER_INSTANCE_NAME: enrollment.instance.name,
        ULCER_INSTANCE_DATA_DIR: agentDirectory,
        ULCER_HOST_GRPC: "127.0.0.1:18443",
        ULCER_HOST_SERVER_NAME: "localhost",
      },
    });
    agentExit = new Promise((resolveExit) => agent?.once("exit", resolveExit));

    await expect.poll(async () => {
      const response = await page.request.get("/api/v1/instances");
      const body = (await response.json()) as { items: Instance[] };
      return body.items.find((item) => item.id === enrollment.instance.id)?.online;
    }).toBe(true);

    const desired = await page.request.put(`/api/v1/instances/${enrollment.instance.id}/desired`, {
      data: {
        engine: "xray",
        artifact: "sha256:e2e",
        desired_phase: "stopped",
        config: {},
      },
    });
    expect(desired.ok()).toBe(true);
    await expect.poll(async () => {
      const response = await page.request.get(`/api/v1/instances/${enrollment.instance.id}`);
      const body = (await response.json()) as { instance: Instance };
      return `${body.instance.phase}:${body.instance.applied_generation}`;
    }).toBe("stopped:1");

    const unavailable = await page.request.put(`/api/v1/instances/${enrollment.instance.id}/desired`, {
      data: {
        engine: "xray",
        artifact: "sha256:e2e",
        desired_phase: "running",
        config: {},
      },
    });
    expect(unavailable.status()).toBe(422);

    await page.getByRole("button", { name: "Close" }).click();
    await expect(page.getByText("e2e-edge-01")).toBeVisible();
    await page.getByRole("button", { name: "Engines" }).click();
    await expect(page.getByRole("heading", { name: "Xray-core" })).toBeVisible();
    await expect(page.getByText("No runtime adapters are available.")).toBeVisible();

    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByRole("button", { name: "Open navigation" }).click();
    await expect(page.getByRole("button", { name: "Overview" })).toBeVisible();
    await page.getByRole("button", { name: "Overview" }).click();
    await expect(page.getByRole("heading", { name: "Overview", exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Open navigation" }).click();
    await page.getByRole("button", { name: "Instances" }).click();
    await page.getByRole("button", { name: "Delete e2e-edge-01" }).click();
    await expect(page.getByRole("heading", { name: "Delete instance?" })).toBeVisible();
    await expect(page.getByText(/Remote files and the service unit remain installed/)).toBeVisible();
    const deleteResponsePromise = page.waitForResponse((response) =>
      response.url().endsWith(`/api/v1/instances/${enrollment.instance.id}`) && response.request().method() === "DELETE",
    );
    await page.getByRole("button", { name: "Delete instance", exact: true }).click();
    expect((await deleteResponsePromise).status()).toBe(204);
    await expect(page.getByText("e2e-edge-01", { exact: true })).toHaveCount(0);
    const deleted = await page.request.get(`/api/v1/instances/${enrollment.instance.id}`);
    expect(deleted.status()).toBe(404);
    await expect.poll(async () => Promise.race([
      agentExit,
      new Promise<"running">((resolveRunning) => setTimeout(() => resolveRunning("running"), 100)),
    ])).toBe(0);
    agent = undefined;
  } finally {
    agent?.kill("SIGTERM");
    await rm(agentDirectory, { recursive: true, force: true });
  }
});

function totp(secret: string): string {
  const key = decodeBase32(secret);
  const counter = Math.floor(Date.now() / 30_000);
  const message = Buffer.alloc(8);
  message.writeBigUInt64BE(BigInt(counter));
  const digest = createHmac("sha1", key).update(message).digest();
  const offset = digest[digest.length - 1] & 0x0f;
  const binary =
    ((digest[offset] & 0x7f) << 24) |
    ((digest[offset + 1] & 0xff) << 16) |
    ((digest[offset + 2] & 0xff) << 8) |
    (digest[offset + 3] & 0xff);
  return String(binary % 1_000_000).padStart(6, "0");
}

function decodeBase32(value: string): Buffer {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const character of value.replace(/=+$/, "").toUpperCase()) {
    const index = alphabet.indexOf(character);
    if (index < 0) throw new Error("invalid base32 secret");
    bits += index.toString(2).padStart(5, "0");
  }
  const bytes: number[] = [];
  for (let offset = 0; offset + 8 <= bits.length; offset += 8) {
    bytes.push(Number.parseInt(bits.slice(offset, offset + 8), 2));
  }
  return Buffer.from(bytes);
}
