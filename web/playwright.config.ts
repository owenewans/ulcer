import { randomUUID } from "node:crypto";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { defineConfig, devices } from "@playwright/test";

const dataDir = join(tmpdir(), `ulcer-playwright-${randomUUID()}`);
const proxyBypass = {
  HTTP_PROXY: "",
  HTTPS_PROXY: "",
  ALL_PROXY: "",
  http_proxy: "",
  https_proxy: "",
  all_proxy: "",
  NO_PROXY: "localhost,127.0.0.1,::1",
  no_proxy: "localhost,127.0.0.1,::1",
};
Object.assign(process.env, proxyBypass);
const localEnvironment = {
  ...process.env,
  ...proxyBypass,
};

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  timeout: 90_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: "http://127.0.0.1:13000",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command: "go run -ldflags=\"-X github.com/owenewans/ulcer/internal/buildinfo.SourceRef=e2e-ref\" ./cmd/ulcer-host",
      cwd: resolve(process.cwd(), ".."),
      url: "http://127.0.0.1:18080/healthz",
      timeout: 120_000,
      reuseExistingServer: false,
      env: {
        ...localEnvironment,
        ULCER_HTTP_ADDR: "127.0.0.1:18080",
        ULCER_GRPC_ADDR: "127.0.0.1:18443",
        ULCER_DATA_DIR: dataDir,
        ULCER_PUBLIC_NAME: "localhost",
        ULCER_PUBLIC_GRPC: "127.0.0.1:18443",
        ULCER_INSTANCE_IMAGE: `registry.example/ulcer-instance@sha256:${"0".repeat(64)}`,
        ULCER_SETUP_TOKEN: "ulcer-e2e-setup-token",
      },
    },
    {
      command: "bun run dev --port 13000",
      cwd: process.cwd(),
      url: "http://127.0.0.1:13000",
      timeout: 120_000,
      reuseExistingServer: false,
      env: {
        ...localEnvironment,
        ULCER_API_ORIGIN: "http://127.0.0.1:18080",
        NEXT_TELEMETRY_DISABLED: "1",
      },
    },
  ],
});
