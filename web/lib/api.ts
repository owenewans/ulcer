export type Instance = {
  id: string;
  name: string;
  created_at: string;
  last_seen_at?: string;
  agent_version?: string;
  capabilities: string[];
  desired_generation: number;
  desired_digest?: string;
  applied_generation: number;
  applied_digest?: string;
  phase: string;
  reason?: string;
  online: boolean;
};

export type Dashboard = {
  instances: { total: number; online: number; running: number; failed: number };
  traffic: { uplink_bytes: number; downlink_bytes: number };
  now: string;
};

export type Meta = {
  version: string;
  revision: string;
  source_ref: string;
  source_url: string;
  license_url: string;
  grpc_endpoint: string;
  grpc_server_name: string;
  instance_image: string;
  ssh_install_available: boolean;
};

export type Engine = {
  id: string;
  name: string;
  repository: string;
  tag: string;
  commit: string;
  license: string;
  distribution: string;
  adapter_status: "planned" | "blocked" | "research" | "available";
  protocols: string[];
  capabilities: string[];
};

export type EngineCatalog = {
  schema: number;
  policy: string;
  engines: Engine[];
};

export type Credentials = {
  certificate_pem: string;
  private_key_pem: string;
  ca_pem: string;
};

type APIError = { error?: { message?: string } };

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as APIError;
    throw new Error(payload.error?.message ?? `Request failed (${response.status})`);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export function formatBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1000)), units.length - 1);
  const value = bytes / 1000 ** index;
  return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`;
}

export function shortCommit(commit: string): string {
  return commit === "workspace" ? commit : commit.slice(0, 8);
}
