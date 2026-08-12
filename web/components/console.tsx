"use client";

import { useEffect, useState } from "react";
import {
  Activity,
  ArrowDownToLine,
  ArrowUpFromLine,
  Box,
  Boxes,
  Braces,
  Check,
  ChevronRight,
  CircleGauge,
  Copy,
  Fingerprint,
  GitBranch,
  KeyRound,
  LockKeyhole,
  Menu,
  Network,
  Plus,
  Radio,
  RefreshCw,
  Server,
  ShieldCheck,
  Terminal,
  X,
  Zap,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  api,
  formatBytes,
  shortCommit,
  type Credentials,
  type Dashboard,
  type EngineCatalog,
  type Instance,
} from "@/lib/api";
import { cn } from "@/lib/utils";

type View = "overview" | "instances" | "engines" | "probers" | "api";
type BootState = "loading" | "setup" | "login" | "ready";

const emptyDashboard: Dashboard = {
  instances: { total: 0, online: 0, running: 0, failed: 0 },
  traffic: { uplink_bytes: 0, downlink_bytes: 0 },
  now: new Date().toISOString(),
};

const navigation = [
  { id: "overview" as const, label: "Overview", icon: CircleGauge },
  { id: "instances" as const, label: "Instances", icon: Boxes },
  { id: "engines" as const, label: "Engines", icon: Box },
  { id: "probers" as const, label: "Probers", icon: Radio },
  { id: "api" as const, label: "API", icon: Braces },
];

async function fetchControlData() {
  const [dashboard, instances, catalog] = await Promise.all([
    api<Dashboard>("/api/v1/dashboard"),
    api<{ items: Instance[] }>("/api/v1/instances"),
    api<EngineCatalog>("/api/v1/engines"),
  ]);
  return { dashboard, instances: instances.items, catalog };
}

export function Console() {
  const [boot, setBoot] = useState<BootState>("loading");
  const [view, setView] = useState<View>("overview");
  const [mobileOpen, setMobileOpen] = useState(false);
  const [dashboard, setDashboard] = useState<Dashboard>(emptyDashboard);
  const [instances, setInstances] = useState<Instance[]>([]);
  const [catalog, setCatalog] = useState<EngineCatalog | null>(null);
  const [error, setError] = useState("");

  async function loadData() {
    try {
      const data = await fetchControlData();
      setDashboard(data.dashboard);
      setInstances(data.instances);
      setCatalog(data.catalog);
      setError("");
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Could not load control plane");
    }
  }

  useEffect(() => {
    let cancelled = false;
    Promise.all([
      api<{ ready: boolean }>("/api/v1/bootstrap/status"),
      api<{ authenticated: boolean }>("/api/v1/auth/session"),
    ])
      .then(([setup, session]) => {
        if (cancelled) return;
        if (!setup.ready) setBoot("setup");
        else if (!session.authenticated) setBoot("login");
        else setBoot("ready");
      })
      .catch((nextError) => {
        if (!cancelled) setError(nextError instanceof Error ? nextError.message : "Host is unavailable");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (boot !== "ready") return;
    let active = true;
    const update = () => {
      void fetchControlData()
        .then((data) => {
          if (!active) return;
          setDashboard(data.dashboard);
          setInstances(data.instances);
          setCatalog(data.catalog);
          setError("");
        })
        .catch((nextError) => {
          if (active) setError(nextError instanceof Error ? nextError.message : "Could not load control plane");
        });
    };
    update();
    const events = new EventSource("/api/v1/events");
    events.addEventListener("instance.connected", update);
    events.addEventListener("instance.disconnected", update);
    events.addEventListener("instance.status", update);
    events.addEventListener("instance.enrolled", update);
    events.addEventListener("traffic.updated", update);
    events.onerror = () => setError("Live event stream reconnecting");
    return () => {
      active = false;
      events.close();
    };
  }, [boot]);

  if (boot === "loading") return <LoadingScreen error={error} />;
  if (boot === "setup") return <SetupScreen onReady={() => setBoot("ready")} />;
  if (boot === "login") return <LoginScreen onReady={() => setBoot("ready")} />;

  return (
    <div className="min-h-screen">
      <div className="noise" />
      <div className="pointer-events-none fixed inset-0 grid-field opacity-50" />
      <Sidebar
        view={view}
        onView={(next) => {
          setView(next);
          setMobileOpen(false);
        }}
        mobileOpen={mobileOpen}
        onClose={() => setMobileOpen(false)}
        online={dashboard.instances.online}
      />
      <main className="relative min-h-screen lg:pl-64">
        <header className="sticky top-0 z-30 flex h-18 items-center justify-between border-b border-[var(--line)] bg-[color:color-mix(in_oklab,var(--ink)_86%,transparent)] px-5 backdrop-blur-xl sm:px-8 lg:px-10">
          <div className="flex items-center gap-3">
            <button className="rounded-full p-2 hover:bg-white/5 lg:hidden" onClick={() => setMobileOpen(true)} aria-label="Open navigation">
              <Menu className="size-5" />
            </button>
            <div>
              <div className="eyebrow">control plane / {view}</div>
              <div className="mt-0.5 text-sm text-[var(--muted)]">{new Date(dashboard.now).toLocaleString()}</div>
            </div>
          </div>
          <div className="flex items-center gap-3">
            {error && <span className="hidden text-xs text-[var(--warning)] sm:block">{error}</span>}
            <Button variant="ghost" size="icon" onClick={() => void loadData()} aria-label="Refresh data">
              <RefreshCw className="size-4" />
            </Button>
            <div className="flex items-center gap-2 rounded-full border border-[var(--line)] bg-[var(--panel)] px-3 py-2 text-xs">
              <span className="status-dot" data-state="online" />
              HOST
            </div>
          </div>
        </header>
        <div className="mx-auto max-w-[1500px] px-5 py-8 sm:px-8 lg:px-10 lg:py-10">
          {view === "overview" && <Overview dashboard={dashboard} instances={instances} catalog={catalog} onNavigate={setView} />}
          {view === "instances" && <Instances instances={instances} onRefresh={loadData} />}
          {view === "engines" && <Engines catalog={catalog} />}
          {view === "probers" && <ComingSoon />}
          {view === "api" && <APISurface />}
        </div>
      </main>
    </div>
  );
}

function Sidebar({
  view,
  onView,
  mobileOpen,
  onClose,
  online,
}: {
  view: View;
  onView: (view: View) => void;
  mobileOpen: boolean;
  onClose: () => void;
  online: number;
}) {
  return (
    <>
      {mobileOpen && <button className="fixed inset-0 z-40 bg-black/70 backdrop-blur-sm lg:hidden" onClick={onClose} aria-label="Close navigation" />}
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-50 flex w-64 -translate-x-full flex-col border-r border-[var(--line)] bg-[#0b0f10] p-4 transition-transform lg:translate-x-0",
          mobileOpen && "translate-x-0",
        )}
      >
        <div className="mb-8 flex h-14 items-center justify-between px-3">
          <button className="flex items-center gap-3" onClick={() => onView("overview")}>
            <Logo />
            <div className="text-left">
              <div className="text-xl font-bold tracking-[-0.04em]">ulcer</div>
              <div className="technical text-[10px] uppercase tracking-[0.16em] text-[var(--dim)]">[ˈʌlsər]</div>
            </div>
          </button>
          <button className="rounded-full p-2 text-[var(--muted)] lg:hidden" onClick={onClose} aria-label="Close navigation">
            <X className="size-4" />
          </button>
        </div>
        <nav className="space-y-1">
          {navigation.map((item) => {
            const Icon = item.icon;
            const active = item.id === view;
            return (
              <button
                key={item.id}
                onClick={() => onView(item.id)}
                className={cn(
                  "flex h-11 w-full items-center gap-3 rounded-xl px-3 text-sm font-medium transition",
                  active ? "bg-[var(--panel-raised)] text-[var(--paper)]" : "text-[var(--muted)] hover:bg-white/[0.035] hover:text-white",
                )}
              >
                <Icon className={cn("size-4", active && "text-[var(--signal)]")} />
                {item.label}
                {item.id === "instances" && online > 0 && (
                  <span className="technical ml-auto rounded-full bg-[var(--signal)]/10 px-2 py-0.5 text-[10px] text-[var(--signal)]">{online}</span>
                )}
              </button>
            );
          })}
        </nav>
        <div className="mt-auto rounded-2xl border border-[var(--line)] bg-[var(--panel)] p-4">
          <div className="mb-3 flex items-center justify-between">
            <span className="eyebrow">security</span>
            <ShieldCheck className="size-4 text-[var(--signal)]" />
          </div>
          <div className="text-sm font-medium">Local authority active</div>
          <p className="mt-1 text-xs leading-5 text-[var(--muted)]">TLS 1.3 · mTLS · TOTP</p>
        </div>
        <a href="https://github.com/owenewans/ulcer" className="mt-3 flex items-center gap-2 px-3 py-2 text-xs text-[var(--dim)] transition hover:text-[var(--muted)]">
          <GitBranch className="size-3.5" /> master / foundation
        </a>
        <a href="https://github.com/owenewans/ulcer/blob/master/LICENSE" className="px-3 text-[10px] leading-4 text-[var(--dim)] transition hover:text-[var(--muted)]">
          GPL-3.0-only · no warranty · source
        </a>
      </aside>
    </>
  );
}

function Overview({
  dashboard,
  instances,
  catalog,
  onNavigate,
}: {
  dashboard: Dashboard;
  instances: Instance[];
  catalog: EngineCatalog | null;
  onNavigate: (view: View) => void;
}) {
  const totalTraffic = dashboard.traffic.uplink_bytes + dashboard.traffic.downlink_bytes;
  return (
    <div className="enter space-y-8">
      <section className="relative overflow-hidden rounded-[2rem] border border-[var(--line)] bg-[var(--panel)] p-7 sm:p-9 lg:p-11">
        <div className="absolute -right-20 -top-24 size-80 rounded-full bg-[var(--accent)]/[0.07] blur-3xl" />
        <div className="relative grid gap-10 xl:grid-cols-[1.35fr_0.65fr] xl:items-end">
          <div>
            <div className="mb-5 flex items-center gap-2">
              <span className="status-dot" data-state="online" />
              <span className="eyebrow">system nominal</span>
            </div>
            <h1 className="max-w-3xl text-4xl font-semibold leading-[1.02] tracking-[-0.055em] sm:text-6xl lg:text-7xl">
              Infrastructure,
              <br />without the ceremony.
            </h1>
            <p className="mt-6 max-w-xl text-base leading-7 text-[var(--muted)] sm:text-lg">
              One desired state. Frozen cores. Every instance converges through a verified machine identity.
            </p>
          </div>
          <div className="grid grid-cols-2 gap-px overflow-hidden rounded-2xl border border-[var(--line)] bg-[var(--line)]">
            <HeroMetric label="instances" value={`${dashboard.instances.online}/${dashboard.instances.total}`} detail="online" />
            <HeroMetric label="traffic" value={formatBytes(totalTraffic)} detail="accounted" />
            <HeroMetric label="engines" value={String(catalog?.engines.length ?? 0)} detail="frozen" />
            <HeroMetric label="failures" value={String(dashboard.instances.failed)} detail="active" danger={dashboard.instances.failed > 0} />
          </div>
        </div>
      </section>

      <section className="grid gap-5 xl:grid-cols-[1.15fr_0.85fr]">
        <div className="rounded-[1.75rem] border border-[var(--line)] bg-[var(--panel)] p-6 sm:p-7">
          <SectionHeading label="fleet pulse" title="Instances" action="Inspect fleet" onAction={() => onNavigate("instances")} />
          <div className="mt-6 space-y-2">
            {instances.length === 0 ? (
              <EmptyMini icon={Server} text="No instances enrolled yet" action="Enroll the first one" onAction={() => onNavigate("instances")} />
            ) : (
              instances.slice(0, 5).map((instance) => <InstanceRow key={instance.id} instance={instance} />)
            )}
          </div>
        </div>
        <div className="rounded-[1.75rem] border border-[var(--line)] bg-[var(--panel)] p-6 sm:p-7">
          <SectionHeading label="meter ledger" title="Transfer" />
          <div className="mt-8 flex items-end justify-between gap-6">
            <div>
              <div className="technical text-4xl font-semibold tracking-[-0.05em] sm:text-5xl">{formatBytes(totalTraffic)}</div>
              <div className="mt-2 text-sm text-[var(--muted)]">idempotently applied</div>
            </div>
            <Activity className="size-9 text-[var(--accent)]" strokeWidth={1.4} />
          </div>
          <div className="mt-9 grid grid-cols-2 gap-3">
            <TrafficMetric icon={ArrowUpFromLine} label="uplink" value={dashboard.traffic.uplink_bytes} />
            <TrafficMetric icon={ArrowDownToLine} label="downlink" value={dashboard.traffic.downlink_bytes} />
          </div>
          <div className="mt-6 border-t border-[var(--line)] pt-5 text-xs leading-5 text-[var(--dim)]">
            Deltas are acknowledged only in contiguous sequence. Replays never double-count.
          </div>
        </div>
      </section>

      <section className="grid gap-5 sm:grid-cols-3">
        <FeatureCard icon={Fingerprint} title="Scoped identity" text="Every agent certificate is bound to exactly one instance through a SPIFFE URI." />
        <FeatureCard icon={LockKeyhole} title="Honest limits" text="Per-user quotas appear only when the selected engine exposes exact accounting." />
        <FeatureCard icon={Zap} title="Level triggered" text="Reconnects replay complete desired state. No fragile chains of imperative commands." />
      </section>
    </div>
  );
}

function Instances({ instances, onRefresh }: { instances: Instance[]; onRefresh: () => Promise<void> }) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [name, setName] = useState("");
  const [credentials, setCredentials] = useState<{ id: string; bundle: Credentials } | null>(null);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function enroll() {
    setSubmitting(true);
    setError("");
    try {
      const response = await api<{ instance: Instance; credentials: Credentials }>("/api/v1/instances", {
        method: "POST",
        body: JSON.stringify({ name }),
      });
      setCredentials({ id: response.instance.id, bundle: response.credentials });
      await onRefresh();
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Could not enroll instance");
    } finally {
      setSubmitting(false);
    }
  }

  function closeDialog(open: boolean) {
    setDialogOpen(open);
    if (!open) {
      setName("");
      setCredentials(null);
      setError("");
    }
  }

  return (
    <div className="enter space-y-7">
      <PageHeading eyebrow="machine topology" title="Instances" description="One isolated runtime, one immutable generation, one exact machine identity." action={<Button onClick={() => setDialogOpen(true)}><Plus className="size-4" /> Enroll instance</Button>} />
      {instances.length === 0 ? (
        <div className="flex min-h-[28rem] flex-col items-center justify-center rounded-[2rem] border border-dashed border-[var(--line)] bg-[var(--panel)] px-6 text-center">
          <div className="mb-6 grid size-16 place-items-center rounded-2xl bg-[var(--signal)]/10 text-[var(--signal)]"><Network className="size-7" /></div>
          <h2 className="text-2xl font-semibold tracking-[-0.03em]">The fleet starts here.</h2>
          <p className="mt-3 max-w-md text-sm leading-6 text-[var(--muted)]">Enroll a machine identity, place the credentials beside the agent, then let desired state take over.</p>
          <Button className="mt-7" onClick={() => setDialogOpen(true)}><Plus className="size-4" /> Enroll first instance</Button>
        </div>
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          {instances.map((instance) => <InstanceCard key={instance.id} instance={instance} />)}
        </div>
      )}
      <Dialog open={dialogOpen} onOpenChange={closeDialog}>
        <DialogContent>
          {!credentials ? (
            <>
              <DialogHeader>
                <DialogTitle>Enroll an instance</DialogTitle>
                <DialogDescription>This creates a durable identity and issues a client certificate. The private key is returned once.</DialogDescription>
              </DialogHeader>
              <label className="eyebrow" htmlFor="instance-name">machine label</label>
              <Input id="instance-name" className="mt-2" placeholder="ams-edge-01" value={name} onChange={(event) => setName(event.target.value)} autoFocus />
              {error && <p className="mt-3 text-sm text-[var(--danger)]">{error}</p>}
              <Button className="mt-6 w-full" size="lg" disabled={!name.trim() || submitting} onClick={() => void enroll()}>
                {submitting ? <RefreshCw className="size-4 animate-spin" /> : <KeyRound className="size-4" />}
                Issue machine identity
              </Button>
            </>
          ) : (
            <CredentialBundle id={credentials.id} bundle={credentials.bundle} />
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}

function Engines({ catalog }: { catalog: EngineCatalog | null }) {
  return (
    <div className="enter space-y-7">
      <PageHeading eyebrow="version freeze" title="Engine catalog" description={`Reviewed ${catalog?.reviewed_at ?? "—"}. Tags are author input; deployments resolve to signed immutable artifacts.`} />
      <div className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-3">
        {catalog?.engines.map((engine) => (
          <article key={engine.id} className="group rounded-[1.6rem] border border-[var(--line)] bg-[var(--panel)] p-6 transition hover:-translate-y-0.5 hover:border-[#3c4846]">
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="mb-2 flex items-center gap-2"><EngineStatus status={engine.adapter_status} /></div>
                <h2 className="text-xl font-semibold tracking-[-0.035em]">{engine.name}</h2>
              </div>
              <a href={engine.repository} target="_blank" rel="noreferrer" className="rounded-full border border-[var(--line)] p-2.5 text-[var(--muted)] transition hover:text-white" aria-label={`Open ${engine.name} repository`}><GitBranch className="size-4" /></a>
            </div>
            <div className="technical mt-6 flex flex-wrap gap-x-4 gap-y-2 text-[11px] text-[var(--muted)]">
              <span>{engine.tag || "commit pin"}</span>
              <span>{shortCommit(engine.commit)}</span>
              <span>{engine.license}</span>
            </div>
            <div className="mt-5 flex flex-wrap gap-1.5">
              {engine.protocols.map((protocol) => <span key={protocol} className="rounded-full border border-[var(--line)] bg-black/15 px-2.5 py-1 text-[10px] text-[var(--muted)]">{protocol}</span>)}
            </div>
            {engine.capabilities.length > 0 && <div className="mt-5 border-t border-[var(--line)] pt-4 text-xs leading-5 text-[var(--dim)]">{engine.capabilities.join(" · ")}</div>}
          </article>
        ))}
      </div>
    </div>
  );
}

function ComingSoon() {
  return (
    <div className="enter">
      <PageHeading eyebrow="reachability intelligence" title="Probers" description="Real handshakes from real networks, bound to the interface you choose." />
      <div className="relative mt-8 min-h-[34rem] overflow-hidden rounded-[2rem] border border-[var(--line)] bg-[var(--panel)] p-8 sm:p-12">
        <div className="absolute inset-0 grid-field opacity-60" />
        <div className="relative flex max-w-xl flex-col items-start">
          <div className="grid size-14 place-items-center rounded-2xl bg-[var(--accent)]/10 text-[var(--accent)]"><Radio className="size-6" /></div>
          <h2 className="mt-8 text-3xl font-semibold tracking-[-0.04em]">Observe the network from outside it.</h2>
          <p className="mt-4 text-base leading-7 text-[var(--muted)]">A prober performs a protocol handshake and transfer through `SO_BINDTODEVICE` or an isolated network namespace. Signed reports use TTL, quorum and hysteresis before an endpoint disappears from a subscriber&apos;s view.</p>
          <div className="technical mt-8 rounded-xl border border-[var(--line)] bg-black/25 px-4 py-3 text-xs text-[var(--dim)]">capability: planned / roadmap phase 6</div>
        </div>
      </div>
    </div>
  );
}

function APISurface() {
  const commands = [
    ["Session", "GET /api/v1/auth/session"],
    ["Fleet", "GET /api/v1/instances"],
    ["Catalog", "GET /api/v1/engines"],
    ["Events", "GET /api/v1/events"],
  ];
  return (
    <div className="enter space-y-7">
      <PageHeading eyebrow="one control surface" title="Public API" description="The UI owns no hidden business logic. Everything visible here is reproducible with HTTP and gRPC." />
      <div className="grid gap-5 lg:grid-cols-[0.85fr_1.15fr]">
        <div className="rounded-[1.75rem] border border-[var(--line)] bg-[var(--panel)] p-7">
          <Terminal className="size-6 text-[var(--signal)]" />
          <h2 className="mt-7 text-2xl font-semibold tracking-[-0.035em]">Cookie-authenticated REST</h2>
          <p className="mt-3 text-sm leading-6 text-[var(--muted)]">Opaque sessions are HttpOnly and SameSite strict. Machine identities use a completely separate mTLS trust path.</p>
          <div className="mt-7 space-y-2">
            {commands.map(([label, command]) => <div key={command} className="flex items-center justify-between rounded-xl border border-[var(--line)] bg-black/15 px-4 py-3"><span className="text-xs text-[var(--muted)]">{label}</span><code className="technical text-[11px] text-[var(--paper)]">{command}</code></div>)}
          </div>
        </div>
        <div className="rounded-[1.75rem] border border-[var(--line)] bg-[#0c1011] p-7">
          <div className="flex items-center gap-2 text-xs text-[var(--dim)]"><span className="size-2 rounded-full bg-[var(--danger)]" /><span className="size-2 rounded-full bg-[var(--warning)]" /><span className="size-2 rounded-full bg-[var(--signal)]" /><span className="ml-2 technical">operator@ulcer</span></div>
          <pre className="technical mt-8 overflow-auto text-xs leading-7 text-[var(--muted)]"><code>{`curl --cookie ulcer.cookie \\
  https://panel.example/api/v1/instances

curl --no-buffer --cookie ulcer.cookie \\
  https://panel.example/api/v1/events

# INSTANCE control plane
grpc://panel.example:8443
TLS 1.3 · client certificate required`}</code></pre>
        </div>
      </div>
    </div>
  );
}

function SetupScreen({ onReady }: { onReady: () => void }) {
  const [step, setStep] = useState<"token" | "totp" | "recovery">("token");
  const [token, setToken] = useState("");
  const [code, setCode] = useState("");
  const [secret, setSecret] = useState("");
  const [uri, setURI] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function start() {
    setBusy(true);
    setError("");
    try {
      const response = await api<{ secret: string; uri: string }>("/api/v1/bootstrap/start", { method: "POST", body: JSON.stringify({ token }) });
      setSecret(response.secret);
      setURI(response.uri);
      setStep("totp");
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Setup failed");
    } finally {
      setBusy(false);
    }
  }

  async function complete() {
    setBusy(true);
    setError("");
    try {
      const response = await api<{ recovery_codes: string[] }>("/api/v1/bootstrap/complete", { method: "POST", body: JSON.stringify({ token, code }) });
      setRecoveryCodes(response.recovery_codes);
      setStep("recovery");
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Verification failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell label={`setup / ${step}`}>
      {step === "token" && <>
        <AuthHeading title="No domain. No ceremony." text="Use the token generated on this machine to claim the control plane." />
        <label className="eyebrow" htmlFor="setup-token">local setup token</label>
        <Input id="setup-token" className="technical mt-2" type="password" value={token} onChange={(event) => setToken(event.target.value)} placeholder="48 hexadecimal characters" autoFocus />
        <p className="technical mt-3 text-[11px] text-[var(--dim)]">ULCER_DATA_DIR/setup.token · mode 0600</p>
        <FormError error={error} />
        <Button className="mt-7 w-full" size="lg" disabled={!token || busy} onClick={() => void start()}>{busy ? <RefreshCw className="size-4 animate-spin" /> : <ChevronRight className="size-4" />} Claim host</Button>
      </>}
      {step === "totp" && <>
        <AuthHeading title="Bind your authenticator." text="Scan the URI with any TOTP application, then prove the key before it becomes active." />
        <div className="rounded-2xl border border-[var(--line)] bg-black/20 p-4">
          <div className="eyebrow">manual secret</div>
          <div data-testid="totp-secret" className="technical mt-2 break-all text-sm text-[var(--signal)]">{secret}</div>
          <Button variant="ghost" size="sm" className="mt-3 px-0" onClick={() => void navigator.clipboard.writeText(uri)}><Copy className="size-3.5" /> Copy otpauth URI</Button>
        </div>
        <label className="eyebrow mt-6 block" htmlFor="totp-code">six digit code</label>
        <Input id="totp-code" className="technical mt-2 text-center text-xl tracking-[0.35em]" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, ""))} autoFocus />
        <FormError error={error} />
        <Button className="mt-7 w-full" size="lg" disabled={code.length !== 6 || busy} onClick={() => void complete()}>{busy ? <RefreshCw className="size-4 animate-spin" /> : <ShieldCheck className="size-4" />} Verify and seal</Button>
      </>}
      {step === "recovery" && <>
        <AuthHeading title="Keep one way back in." text="These recovery codes are shown once. Each code is single-use; only its hash remains on the host." />
        <div className="grid grid-cols-2 gap-2 rounded-2xl border border-[var(--line)] bg-black/20 p-4">
          {recoveryCodes.map((recoveryCode) => <code key={recoveryCode} className="technical text-center text-xs text-[var(--paper)]">{recoveryCode}</code>)}
        </div>
        <Button variant="secondary" className="mt-4 w-full" onClick={() => void navigator.clipboard.writeText(recoveryCodes.join("\n"))}><Copy className="size-4" /> Copy recovery codes</Button>
        <Button className="mt-3 w-full" size="lg" onClick={onReady}><Check className="size-4" /> I stored them safely</Button>
      </>}
    </AuthShell>
  );
}

function LoginScreen({ onReady }: { onReady: () => void }) {
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function login() {
    setBusy(true);
    setError("");
    try {
      await api("/api/v1/auth/login", { method: "POST", body: JSON.stringify({ code }) });
      onReady();
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Login failed");
    } finally {
      setBusy(false);
    }
  }
  return (
    <AuthShell label="operator login">
      <AuthHeading title="One operator. One key." text="Enter a TOTP code or consume one offline recovery code." />
      <label className="eyebrow" htmlFor="login-code">authentication code</label>
      <Input id="login-code" className="technical mt-2 text-center text-xl tracking-[0.28em]" autoComplete="one-time-code" value={code} onChange={(event) => setCode(event.target.value.toUpperCase())} onKeyDown={(event) => { if (event.key === "Enter" && code) void login(); }} autoFocus />
      <FormError error={error} />
      <Button className="mt-7 w-full" size="lg" disabled={!code || busy} onClick={() => void login()}>{busy ? <RefreshCw className="size-4 animate-spin" /> : <KeyRound className="size-4" />} Enter control plane</Button>
    </AuthShell>
  );
}

function AuthShell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <main className="relative grid min-h-screen place-items-center overflow-hidden px-5 py-10">
      <div className="noise" /><div className="absolute inset-0 grid-field opacity-70" />
      <div className="absolute left-[8%] top-[10%] size-72 rounded-full bg-[var(--signal)]/[0.035] blur-3xl" />
      <section className="enter relative w-full max-w-lg rounded-[2rem] border border-[var(--line)] bg-[color:color-mix(in_oklab,var(--panel)_94%,transparent)] p-7 shadow-2xl backdrop-blur-xl sm:p-10">
        <div className="mb-10 flex items-center justify-between"><div className="flex items-center gap-3"><Logo /><span className="text-lg font-bold tracking-[-0.04em]">ulcer</span></div><span className="technical text-[10px] uppercase tracking-[0.15em] text-[var(--dim)]">{label}</span></div>
        {children}
      </section>
      <p className="technical absolute bottom-5 text-[10px] text-[var(--dim)]">TLS 1.3 / Badger / no ambient authority</p>
    </main>
  );
}

function AuthHeading({ title, text }: { title: string; text: string }) {
  return <div className="mb-8"><h1 className="text-3xl font-semibold tracking-[-0.045em] sm:text-4xl">{title}</h1><p className="mt-3 text-sm leading-6 text-[var(--muted)]">{text}</p></div>;
}

function LoadingScreen({ error }: { error: string }) {
  return <main className="grid min-h-screen place-items-center"><div className="text-center"><Logo large /><div className="technical mt-6 text-xs uppercase tracking-[0.18em] text-[var(--muted)]">discovering host</div>{error && <p className="mt-4 text-sm text-[var(--danger)]">{error}</p>}</div></main>;
}

function Logo({ large = false }: { large?: boolean }) {
  return <div className={cn("relative grid place-items-center overflow-hidden rounded-xl bg-[var(--signal)] text-[var(--ink)]", large ? "size-14" : "size-9")}><div className="absolute h-[140%] w-2 rotate-[32deg] bg-[var(--ink)]" /><div className="absolute h-2 w-[140%] -rotate-[12deg] bg-[var(--ink)]" /></div>;
}

function PageHeading({ eyebrow, title, description, action }: { eyebrow: string; title: string; description: string; action?: React.ReactNode }) {
  return <header className="flex flex-col justify-between gap-5 sm:flex-row sm:items-end"><div><div className="eyebrow">{eyebrow}</div><h1 className="mt-2 text-4xl font-semibold tracking-[-0.05em] sm:text-5xl">{title}</h1><p className="mt-3 max-w-2xl text-sm leading-6 text-[var(--muted)] sm:text-base">{description}</p></div>{action}</header>;
}

function SectionHeading({ label, title, action, onAction }: { label: string; title: string; action?: string; onAction?: () => void }) {
  return <div className="flex items-end justify-between"><div><div className="eyebrow">{label}</div><h2 className="mt-1 text-2xl font-semibold tracking-[-0.035em]">{title}</h2></div>{action && <Button variant="ghost" size="sm" onClick={onAction}>{action}<ChevronRight className="size-3.5" /></Button>}</div>;
}

function HeroMetric({ label, value, detail, danger = false }: { label: string; value: string; detail: string; danger?: boolean }) {
  return <div className="bg-[#0d1112] p-5"><div className="eyebrow">{label}</div><div className={cn("technical mt-4 text-2xl font-semibold", danger && "text-[var(--danger)]")}>{value}</div><div className="mt-1 text-[11px] text-[var(--dim)]">{detail}</div></div>;
}

function TrafficMetric({ icon: Icon, label, value }: { icon: typeof ArrowUpFromLine; label: string; value: number }) {
  return <div className="rounded-2xl border border-[var(--line)] bg-black/15 p-4"><Icon className="size-4 text-[var(--muted)]" /><div className="technical mt-4 text-lg">{formatBytes(value)}</div><div className="mt-1 text-[11px] text-[var(--dim)]">{label}</div></div>;
}

function FeatureCard({ icon: Icon, title, text }: { icon: typeof Fingerprint; title: string; text: string }) {
  return <article className="rounded-[1.5rem] border border-[var(--line)] bg-[var(--panel)] p-6"><Icon className="size-5 text-[var(--accent)]" /><h3 className="mt-6 font-semibold tracking-[-0.02em]">{title}</h3><p className="mt-2 text-xs leading-5 text-[var(--muted)]">{text}</p></article>;
}

function InstanceRow({ instance }: { instance: Instance }) {
  return <div className="flex items-center gap-4 rounded-2xl border border-transparent px-3 py-3 transition hover:border-[var(--line)] hover:bg-black/10"><span className="status-dot" data-state={instance.online ? "online" : instance.phase} /><div className="min-w-0 flex-1"><div className="truncate text-sm font-medium">{instance.name}</div><div className="technical mt-0.5 truncate text-[10px] text-[var(--dim)]">{instance.id}</div></div><div className="text-right"><div className="technical text-xs">g{instance.applied_generation}/{instance.desired_generation}</div><div className="mt-0.5 text-[10px] text-[var(--dim)]">{instance.phase}</div></div></div>;
}

function InstanceCard({ instance }: { instance: Instance }) {
  return <article className="rounded-[1.7rem] border border-[var(--line)] bg-[var(--panel)] p-6"><div className="flex items-start justify-between"><div className="flex items-center gap-3"><span className="status-dot" data-state={instance.online ? "online" : instance.phase} /><div><h2 className="font-semibold tracking-[-0.02em]">{instance.name}</h2><div className="technical mt-1 text-[10px] text-[var(--dim)]">{instance.id}</div></div></div><span className="rounded-full border border-[var(--line)] px-2.5 py-1 text-[10px] text-[var(--muted)]">{instance.phase}</span></div><div className="mt-7 grid grid-cols-3 gap-2"><SmallStat label="desired" value={`g${instance.desired_generation}`} /><SmallStat label="applied" value={`g${instance.applied_generation}`} /><SmallStat label="agent" value={instance.agent_version || "—"} /></div>{instance.reason && <p className="mt-5 border-t border-[var(--line)] pt-4 text-xs leading-5 text-[var(--warning)]">{instance.reason}</p>}</article>;
}

function SmallStat({ label, value }: { label: string; value: string }) {
  return <div className="rounded-xl bg-black/15 p-3"><div className="eyebrow !text-[9px]">{label}</div><div className="technical mt-2 truncate text-xs">{value}</div></div>;
}

function EngineStatus({ status }: { status: string }) {
  const tone = status === "available" ? "text-[var(--signal)] bg-[var(--signal)]/10" : status === "blocked" ? "text-[var(--danger)] bg-[var(--danger)]/10" : "text-[var(--muted)] bg-white/5";
  return <span className={cn("rounded-full px-2.5 py-1 text-[9px] font-bold uppercase tracking-[0.14em]", tone)}>{status}</span>;
}

function EmptyMini({ icon: Icon, text, action, onAction }: { icon: typeof Server; text: string; action: string; onAction: () => void }) {
  return <div className="flex min-h-44 flex-col items-center justify-center rounded-2xl border border-dashed border-[var(--line)] text-center"><Icon className="size-5 text-[var(--dim)]" /><div className="mt-3 text-sm text-[var(--muted)]">{text}</div><Button variant="ghost" size="sm" className="mt-2" onClick={onAction}>{action}<ChevronRight className="size-3" /></Button></div>;
}

function CredentialBundle({ id, bundle }: { id: string; bundle: Credentials }) {
  const env = `ULCER_INSTANCE_ID=${id}\nULCER_HOST_GRPC=host.example:8443\nULCER_HOST_SERVER_NAME=host.example`;
  return <><DialogHeader><DialogTitle>Identity issued</DialogTitle><DialogDescription>Save all three PEM values now. The private key is not retained by this browser and will not be returned again.</DialogDescription></DialogHeader><div className="rounded-2xl border border-[var(--warning)]/30 bg-[var(--warning)]/[0.06] p-4 text-xs leading-5 text-[var(--warning)]">Treat this response as a credential. Use mode 0600 and UID/GID 65532:65532 for the Podman agent, then close this dialog only after verification.</div><div className="mt-5 space-y-2"><CopyBlock label="instance.env" value={env} /><CopyBlock label="instance.crt" value={bundle.certificate_pem} /><CopyBlock label="instance.key" value={bundle.private_key_pem} /><CopyBlock label="ca.crt" value={bundle.ca_pem} /></div></>;
}

function CopyBlock({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  return <button className="flex w-full items-center justify-between rounded-xl border border-[var(--line)] bg-black/15 px-4 py-3 text-left transition hover:border-[var(--muted)]" onClick={() => { void navigator.clipboard.writeText(value); setCopied(true); window.setTimeout(() => setCopied(false), 1200); }}><span className="technical text-xs">{label}</span>{copied ? <Check className="size-4 text-[var(--signal)]" /> : <Copy className="size-4 text-[var(--muted)]" />}</button>;
}

function FormError({ error }: { error: string }) {
  return error ? <p className="mt-4 text-sm text-[var(--danger)]">{error}</p> : null;
}
