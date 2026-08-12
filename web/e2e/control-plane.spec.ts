import { createHmac } from "node:crypto";
import { spawn, type ChildProcess } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { expect, test } from "@playwright/test";
import type { Credentials, Instance } from "@/lib/api";

test("onboards operator and reconciles an mTLS instance", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "No domain. No ceremony." })).toBeVisible();

  await page.getByLabel("local setup token").fill("ulcer-e2e-setup-token");
  await page.getByRole("button", { name: "Claim host" }).click();
  await expect(page.getByRole("heading", { name: "Bind your authenticator." })).toBeVisible();

  const secret = (await page.getByTestId("totp-secret").textContent()) ?? "";
  await page.getByLabel("six digit code").fill(totp(secret));
  await page.getByRole("button", { name: "Verify and seal" }).click();
  await expect(page.getByRole("heading", { name: "Keep one way back in." })).toBeVisible();
  await expect(page.locator("code")).toHaveCount(10);
  await page.getByRole("button", { name: "I stored them safely" }).click();

  await expect(page.getByRole("heading", { name: /Infrastructure/ })).toBeVisible();
  await page.getByRole("button", { name: "Instances" }).click();
  await page.getByRole("button", { name: "Enroll first instance" }).click();
  await page.getByLabel("machine label").fill("e2e-edge-01");

  const enrollmentResponse = page.waitForResponse((response) =>
    response.url().endsWith("/api/v1/instances") && response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Issue machine identity" }).click();
  const enrollment = (await (await enrollmentResponse).json()) as {
    instance: Instance;
    credentials: Credentials;
  };
  await expect(page.getByRole("heading", { name: "Identity issued" })).toBeVisible();

  const agentDirectory = await mkdtemp(join(tmpdir(), "ulcer-instance-e2e-"));
  let agent: ChildProcess | undefined;
  try {
    await Promise.all([
      writeFile(join(agentDirectory, "instance.crt"), enrollment.credentials.certificate_pem, { mode: 0o600 }),
      writeFile(join(agentDirectory, "instance.key"), enrollment.credentials.private_key_pem, { mode: 0o600 }),
      writeFile(join(agentDirectory, "ca.crt"), enrollment.credentials.ca_pem, { mode: 0o600 }),
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
    await expect(page.getByText("blocked", { exact: true }).first()).toBeVisible();

    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByRole("button", { name: "Open navigation" }).click();
    await expect(page.getByRole("button", { name: "Overview" })).toBeVisible();
    await page.getByRole("button", { name: "Overview" }).click();
    await expect(page.getByRole("heading", { name: /Infrastructure/ })).toBeVisible();
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
