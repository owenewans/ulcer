"use client";

import { useEffect, useRef, useState } from "react";
import {
  Activity,
  ArrowDownToLine,
  ArrowLeft,
  ArrowUpFromLine,
  Box,
  Boxes,
  Braces,
  Check,
  ChevronRight,
  CircleGauge,
  Download,
  FileArchive,
  Fingerprint,
  GitBranch,
  KeyRound,
  Menu,
  Network,
  Plus,
  Radio,
  RefreshCw,
  Server,
  Terminal,
  Trash2,
  X,
} from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
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
  type Engine,
  type EngineCatalog,
  type Instance,
  type Meta,
} from "@/lib/api";
import { createInstanceBundle, instanceBundleFilename } from "@/lib/instance-bundle";
import { cn } from "@/lib/utils";

type View = "overview" | "instances" | "engines" | "probers" | "api";
type BootState = "loading" | "setup" | "login" | "ready";
type EnrollmentMode = "manual" | "ssh" | null;

const instanceNamePattern = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;

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
  const [dashboard, instances, catalog, meta] = await Promise.all([
    api<Dashboard>("/api/v1/dashboard"),
    api<{ items: Instance[] }>("/api/v1/instances"),
    api<EngineCatalog>("/api/v1/engines"),
    api<Meta>("/api/v1/meta"),
  ]);
  return { dashboard, instances: instances.items, catalog, meta };
}

export function Console() {
  const [boot, setBoot] = useState<BootState>("loading");
  const [view, setView] = useState<View>("overview");
  const [mobileOpen, setMobileOpen] = useState(false);
  const [dashboard, setDashboard] = useState<Dashboard>(emptyDashboard);
  const [instances, setInstances] = useState<Instance[]>([]);
  const [catalog, setCatalog] = useState<EngineCatalog | null>(null);
  const [meta, setMeta] = useState<Meta | null>(null);
  const [error, setError] = useState("");

  function applyControlData(data: Awaited<ReturnType<typeof fetchControlData>>) {
    setDashboard(data.dashboard);
    setInstances(data.instances);
    setCatalog(data.catalog);
    setMeta(data.meta);
    setError("");
  }

  async function loadData() {
    try {
      applyControlData(await fetchControlData());
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
          if (active) applyControlData(data);
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
    events.addEventListener("instance.deleted", update);
    events.addEventListener("instance.ssh_install.failed", update);
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
        meta={meta}
      />
      <main className="relative min-h-screen lg:pl-64">
        <header className="sticky top-0 z-30 flex h-18 items-center justify-between border-b border-[var(--line)] bg-[color:color-mix(in_oklab,var(--ink)_86%,transparent)] px-5 backdrop-blur-xl sm:px-8 lg:px-10">
          <div className="flex min-w-0 items-center gap-3">
            <button className="shrink-0 rounded-full p-2 hover:bg-white/5 lg:hidden" onClick={() => setMobileOpen(true)} aria-label="Open navigation">
              <Menu className="size-5" />
            </button>
            <div className="min-w-0">
              <div className="eyebrow">control plane / {view}</div>
              <ServerClock serverTime={dashboard.now} />
            </div>
          </div>
          <div className="flex items-center gap-3">
            {error && <span className="hidden text-xs text-[var(--warning)] sm:block">{error}</span>}
            <Button variant="ghost" size="icon" onClick={() => void loadData()} aria-label="Refresh data">
              <RefreshCw className="size-4" />
            </Button>
          </div>
        </header>
        <div className="mx-auto max-w-[1500px] px-5 py-8 sm:px-8 lg:px-10 lg:py-10">
          {view === "overview" && <Overview dashboard={dashboard} instances={instances} catalog={catalog} onNavigate={setView} />}
          {view === "instances" && <Instances instances={instances} meta={meta} onRefresh={loadData} />}
          {view === "engines" && <Engines catalog={catalog} />}
          {view === "probers" && <Probers />}
          {view === "api" && <APISurface meta={meta} />}
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
  meta,
}: {
  view: View;
  onView: (view: View) => void;
  mobileOpen: boolean;
  onClose: () => void;
  online: number;
  meta: Meta | null;
}) {
  const revision = meta?.revision && meta.revision !== "unknown" ? shortCommit(meta.revision) : "";
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
            <div className="text-xl font-bold tracking-[-0.04em]">ulcer</div>
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
        <div className="mt-auto border-t border-[var(--line)] px-3 pt-5 text-[10px] leading-4 text-[var(--dim)]">
          {meta ? (
            <>
              <a
                href={meta.source_url}
                target="_blank"
                rel="noreferrer"
                className="flex items-center gap-2 transition hover:text-[var(--muted)]"
                aria-label={`Source ${meta.source_ref}${revision ? ` revision ${revision}` : ""}`}
              >
                <GitBranch className="size-3.5 shrink-0" />
                <span className="technical truncate">{meta.source_ref}{revision ? ` / ${revision}` : ""}</span>
              </a>
              <div className="mt-3">
                <a href={meta.license_url} target="_blank" rel="noreferrer" className="transition hover:text-[var(--muted)]">GPL-3.0-only</a>
                <span> / no warranty / source</span>
              </div>
            </>
          ) : (
            <span>Source metadata unavailable</span>
          )}
        </div>
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
  const availableAdapters = catalog?.engines.filter((engine) => engine.adapter_status === "available").length ?? 0;
  return (
    <div className="enter space-y-8">
      <PageHeading title="Overview" description="Current fleet status and accounted traffic." />
      <section className="grid gap-px overflow-hidden rounded-[1.75rem] border border-[var(--line)] bg-[var(--line)] sm:grid-cols-2 xl:grid-cols-4">
        <Metric label="instances" value={`${dashboard.instances.online}/${dashboard.instances.total}`} detail="online" />
        <Metric label="traffic" value={formatBytes(totalTraffic)} detail="total transfer" />
        <Metric label="adapters" value={String(availableAdapters)} detail="available" />
        <Metric label="failures" value={String(dashboard.instances.failed)} detail="active" danger={dashboard.instances.failed > 0} />
      </section>

      <section className="grid gap-5 xl:grid-cols-[1.15fr_0.85fr]">
        <div className="rounded-[1.75rem] border border-[var(--line)] bg-[var(--panel)] p-6 sm:p-7">
          <SectionHeading title="Instances" action="Inspect fleet" onAction={() => onNavigate("instances")} />
          <div className="mt-6 space-y-2">
            {instances.length === 0 ? (
              <EmptyMini icon={Server} text="No instances enrolled" action="Enroll instance" onAction={() => onNavigate("instances")} />
            ) : (
              instances.slice(0, 5).map((instance) => <InstanceRow key={instance.id} instance={instance} />)
            )}
          </div>
        </div>
        <div className="rounded-[1.75rem] border border-[var(--line)] bg-[var(--panel)] p-6 sm:p-7">
          <SectionHeading title="Traffic" />
          <div className="mt-8 flex items-end justify-between gap-6">
            <div className="technical text-4xl font-semibold tracking-[-0.05em] sm:text-5xl">{formatBytes(totalTraffic)}</div>
            <Activity className="size-9 text-[var(--accent)]" strokeWidth={1.4} />
          </div>
          <div className="mt-9 grid grid-cols-2 gap-3">
            <TrafficMetric icon={ArrowUpFromLine} label="uplink" value={dashboard.traffic.uplink_bytes} />
            <TrafficMetric icon={ArrowDownToLine} label="downlink" value={dashboard.traffic.downlink_bytes} />
          </div>
        </div>
      </section>
    </div>
  );
}

function Instances({ instances, meta, onRefresh }: { instances: Instance[]; meta: Meta | null; onRefresh: () => Promise<void> }) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState<EnrollmentMode>(null);
  const [deleting, setDeleting] = useState<Instance | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const [notice, setNotice] = useState("");

  function setEnrollmentOpen(open: boolean) {
    setDialogOpen(open);
    if (!open) setMode(null);
  }

  async function deleteInstance() {
    if (!deleting) return;
    setDeleteBusy(true);
    setDeleteError("");
    try {
      const name = deleting.name;
      await api<void>(`/api/v1/instances/${deleting.id}`, { method: "DELETE" });
      setDeleting(null);
      setNotice(`${name} was deleted.`);
      await onRefresh();
    } catch (nextError) {
      setDeleteError(nextError instanceof Error ? nextError.message : "Could not delete instance");
    } finally {
      setDeleteBusy(false);
    }
  }

  return (
    <div className="enter space-y-7">
      <PageHeading
        title="Instances"
        description="Enroll, inspect, and revoke instance agents."
        action={<Button onClick={() => setDialogOpen(true)}><Plus className="size-4" /> Enroll instance</Button>}
      />
      {notice && <div role="status" className="rounded-2xl border border-[var(--signal)]/25 bg-[var(--signal)]/[0.06] px-4 py-3 text-sm text-[var(--signal)]">{notice}</div>}
      {instances.length === 0 ? (
        <div className="flex min-h-[28rem] flex-col items-center justify-center rounded-[2rem] border border-dashed border-[var(--line)] bg-[var(--panel)] px-6 text-center">
          <div className="mb-6 grid size-16 place-items-center rounded-2xl bg-[var(--signal)]/10 text-[var(--signal)]"><Network className="size-7" /></div>
          <h2 className="text-2xl font-semibold tracking-[-0.03em]">No instances enrolled</h2>
          <p className="mt-3 max-w-md text-sm leading-6 text-[var(--muted)]">Download a manual bundle or install an agent over a verified SSH connection.</p>
          <Button className="mt-7" onClick={() => setDialogOpen(true)}><Plus className="size-4" /> Enroll first instance</Button>
        </div>
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          {instances.map((instance) => <InstanceCard key={instance.id} instance={instance} onDelete={() => { setDeleteError(""); setDeleting(instance); }} />)}
        </div>
      )}

      <Dialog open={dialogOpen} onOpenChange={setEnrollmentOpen}>
        <DialogContent className={mode === "ssh" ? "max-w-2xl" : undefined}>
          {mode === null && <EnrollmentModePicker meta={meta} onMode={setMode} />}
          {mode === "manual" && meta && <ManualEnrollment meta={meta} onBack={() => setMode(null)} onRefresh={onRefresh} />}
          {mode === "ssh" && meta && (
            <SSHEnrollment
              onBack={() => setMode(null)}
              onRefresh={onRefresh}
              onConnected={(name) => {
                setNotice(`${name} was installed and connected.`);
                setEnrollmentOpen(false);
              }}
            />
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={deleting !== null} onOpenChange={(open) => { if (!open && !deleteBusy) { setDeleting(null); setDeleteError(""); } }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete instance?</DialogTitle>
            <DialogDescription>
              Deleting {deleting?.name} revokes its control-plane access and stops the agent after its final rejected reconnect. Remote files and the service unit remain installed.
            </DialogDescription>
          </DialogHeader>
          {deleteError && <FormError error={deleteError} />}
          <div className="mt-7 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
            <Button variant="secondary" disabled={deleteBusy} onClick={() => setDeleting(null)}>Cancel</Button>
            <Button variant="danger" disabled={deleteBusy} onClick={() => void deleteInstance()}>
              {deleteBusy ? <RefreshCw className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
              Delete instance
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function EnrollmentModePicker({ meta, onMode }: { meta: Meta | null; onMode: (mode: Exclude<EnrollmentMode, null>) => void }) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>Enroll an instance</DialogTitle>
        <DialogDescription>Choose how the instance agent will be installed.</DialogDescription>
      </DialogHeader>
      <div className="grid gap-3 sm:grid-cols-2">
        <button
          className="rounded-2xl border border-[var(--line)] bg-black/15 p-5 text-left outline-none transition hover:border-[var(--muted)] focus-visible:ring-2 focus-visible:ring-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-45"
          disabled={!meta}
          onClick={() => onMode("manual")}
        >
          <FileArchive className="size-5 text-[var(--signal)]" />
          <span className="mt-5 block font-semibold">Download manual bundle</span>
          <span className="mt-2 block text-xs leading-5 text-[var(--muted)]">Create one ZIP with credentials and exact connection settings.</span>
        </button>
        <button
          className="rounded-2xl border border-[var(--line)] bg-black/15 p-5 text-left outline-none transition hover:border-[var(--muted)] focus-visible:ring-2 focus-visible:ring-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-45"
          disabled={!meta?.ssh_install_available}
          onClick={() => onMode("ssh")}
        >
          <Terminal className="size-5 text-[var(--accent)]" />
          <span className="mt-5 block font-semibold">Install over SSH</span>
          <span className="mt-2 block text-xs leading-5 text-[var(--muted)]">Verify the remote host key before sending root credentials.</span>
          {meta && !meta.ssh_install_available && <span className="mt-3 block text-[10px] uppercase tracking-[0.12em] text-[var(--dim)]">Unavailable on this host</span>}
        </button>
      </div>
      {!meta && <p className="mt-4 text-sm text-[var(--warning)]">Instance metadata is still loading.</p>}
    </>
  );
}

function ManualEnrollment({ meta, onBack, onRefresh }: { meta: Meta; onBack: () => void; onRefresh: () => Promise<void> }) {
  const [name, setName] = useState("");
  const [result, setResult] = useState<{ instance: Instance; credentials: Credentials } | null>(null);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const validName = instanceNamePattern.test(name);

  async function enroll() {
    setSubmitting(true);
    setError("");
    try {
      const response = await api<{ instance: Instance; credentials: Credentials }>("/api/v1/instances", {
        method: "POST",
        body: JSON.stringify({ name: name.trim() }),
      });
      setResult(response);
      await onRefresh();
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Could not enroll instance");
    } finally {
      setSubmitting(false);
    }
  }

  function downloadBundle() {
    if (!result) return;
    const archive = createInstanceBundle({ ...result, meta });
    downloadBlob(new Blob([new Uint8Array(archive)], { type: "application/zip" }), instanceBundleFilename(result.instance.name, result.instance.id));
  }

  if (result) {
    return (
      <>
        <DialogHeader>
          <DialogTitle>Manual bundle ready</DialogTitle>
          <DialogDescription>The ZIP contains the instance configuration, certificate, private key, CA certificate, and installation instructions.</DialogDescription>
        </DialogHeader>
        <div className="rounded-2xl border border-[var(--warning)]/30 bg-[var(--warning)]/[0.06] p-4 text-xs leading-5 text-[var(--warning)]">
          The private key is returned once. Store this bundle securely and remove surplus copies after installation.
        </div>
        <div className="mt-5 rounded-2xl border border-[var(--line)] bg-black/15 p-4">
          <div className="text-sm font-medium">{result.instance.name}</div>
          <div className="technical mt-1 break-all text-[10px] text-[var(--dim)]">{result.instance.id}</div>
          <div className="technical mt-3 text-xs text-[var(--muted)]">{instanceBundleFilename(result.instance.name, result.instance.id)}</div>
        </div>
        <Button className="mt-6 w-full" size="lg" onClick={downloadBundle}><Download className="size-4" /> Download ZIP bundle</Button>
      </>
    );
  }

  return (
    <>
      <DialogHeader>
        <Button variant="ghost" size="sm" className="mb-2 -ml-3 w-fit" onClick={onBack}><ArrowLeft className="size-3.5" /> Enrollment methods</Button>
        <DialogTitle>Download manual bundle</DialogTitle>
        <DialogDescription>Create a durable instance identity and download its one-time credentials as one ZIP archive.</DialogDescription>
      </DialogHeader>
      <label className="eyebrow" htmlFor="manual-instance-name">instance name</label>
      <Input id="manual-instance-name" className="mt-2" placeholder="ams-edge-01" value={name} onChange={(event) => setName(event.target.value)} maxLength={63} pattern={instanceNamePattern.source} aria-describedby="manual-name-help" autoFocus />
      <p id="manual-name-help" className="mt-2 text-xs text-[var(--dim)]">Use lowercase letters, digits, and hyphens.</p>
      <FormError error={error} />
      <Button className="mt-6 w-full" size="lg" disabled={!validName || submitting} onClick={() => void enroll()}>
        {submitting ? <RefreshCw className="size-4 animate-spin" /> : <KeyRound className="size-4" />}
        Create bundle
      </Button>
    </>
  );
}

function SSHEnrollment({ onBack, onRefresh, onConnected }: { onBack: () => void; onRefresh: () => Promise<void>; onConnected: (name: string) => void }) {
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("22");
  const [auth, setAuth] = useState<"password" | "private-key">("password");
  const [password, setPassword] = useState("");
  const [privateKey, setPrivateKey] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const [hostKey, setHostKey] = useState<{ algorithm: string; fingerprint: string; host: string; port: number } | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const [probing, setProbing] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [error, setError] = useState("");
  const probeVersion = useRef(0);
  const probeAbort = useRef<AbortController | null>(null);
  const installAbort = useRef<AbortController | null>(null);
  const numericPort = Number(port);
  const validName = instanceNamePattern.test(name);
  const validTarget = Boolean(host.trim()) && Number.isInteger(numericPort) && numericPort >= 1 && numericPort <= 65535;
  const hostKeyCurrent = hostKey?.host === host.trim() && hostKey.port === numericPort;
  const hasCredentials = auth === "password" ? Boolean(password) : Boolean(privateKey.trim());

  useEffect(() => {
    return () => {
      probeAbort.current?.abort();
      installAbort.current?.abort();
    };
  }, []);

  function resetHostKey() {
    probeVersion.current += 1;
    probeAbort.current?.abort();
    probeAbort.current = null;
    setHostKey(null);
    setConfirmed(false);
    setProbing(false);
    setError("");
  }

  async function probeHostKey() {
    if (!validTarget) return;
    setProbing(true);
    setError("");
    setHostKey(null);
    setConfirmed(false);
    const version = ++probeVersion.current;
    const controller = new AbortController();
    probeAbort.current?.abort();
    probeAbort.current = controller;
    try {
      const probedHost = host.trim();
      const probedPort = numericPort;
      const probedHostKey = await api<{ algorithm: string; fingerprint: string }>("/api/v1/ssh/host-key", {
        method: "POST",
        body: JSON.stringify({ host: probedHost, port: probedPort }),
        signal: controller.signal,
      });
      if (version !== probeVersion.current) return;
      if (!probedHostKey.fingerprint.startsWith("SHA256:")) throw new Error("Host returned an invalid SSH fingerprint");
      setHostKey({ ...probedHostKey, host: probedHost, port: probedPort });
    } catch (nextError) {
      if (version === probeVersion.current && !controller.signal.aborted) setError(nextError instanceof Error ? nextError.message : "Could not read the SSH host key");
    } finally {
      if (probeAbort.current === controller) probeAbort.current = null;
      if (version === probeVersion.current && !controller.signal.aborted) setProbing(false);
    }
  }

  function clearSecrets() {
    setPassword("");
    setPrivateKey("");
    setPassphrase("");
  }

  async function install() {
    if (!hostKey || !hostKeyCurrent || !confirmed || !hasCredentials) return;
    setInstalling(true);
    setError("");
    const controller = new AbortController();
    installAbort.current?.abort();
    installAbort.current = controller;
    try {
      const response = await api<{ instance: Instance }>("/api/v1/instances/ssh", {
        method: "POST",
        body: JSON.stringify({
          name: name.trim(),
          host: host.trim(),
          port: numericPort,
          user: "root",
          host_key_sha256: hostKey.fingerprint,
          ...(auth === "password" ? { password } : { private_key_pem: privateKey, ...(passphrase ? { passphrase } : {}) }),
        }),
        signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      clearSecrets();
      await onRefresh();
      if (!controller.signal.aborted) onConnected(response.instance.name);
    } catch (nextError) {
      if (!controller.signal.aborted) setError(nextError instanceof Error ? nextError.message : "SSH installation failed");
    } finally {
      if (installAbort.current === controller) installAbort.current = null;
      if (!controller.signal.aborted) setInstalling(false);
    }
  }

  return (
    <>
      <DialogHeader>
        <Button variant="ghost" size="sm" className="mb-2 -ml-3 w-fit" onClick={() => { clearSecrets(); onBack(); }}><ArrowLeft className="size-3.5" /> Enrollment methods</Button>
        <DialogTitle>Install over SSH</DialogTitle>
        <DialogDescription>Probe and verify the server identity before sending root credentials. Secrets stay in this dialog only.</DialogDescription>
      </DialogHeader>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="instance name" htmlFor="ssh-instance-name">
          <Input id="ssh-instance-name" placeholder="ams-edge-01" value={name} onChange={(event) => setName(event.target.value)} maxLength={63} pattern={instanceNamePattern.source} autoFocus />
        </Field>
        <Field label="username" htmlFor="ssh-user">
          <Input id="ssh-user" value="root" readOnly aria-readonly="true" />
        </Field>
        <Field label="host" htmlFor="ssh-host" className="sm:col-span-[1]">
          <Input id="ssh-host" placeholder="203.0.113.10" value={host} onChange={(event) => { setHost(event.target.value); resetHostKey(); }} autoComplete="off" />
        </Field>
        <Field label="port" htmlFor="ssh-port">
          <Input id="ssh-port" type="number" min={1} max={65535} inputMode="numeric" value={port} onChange={(event) => { setPort(event.target.value); resetHostKey(); }} />
        </Field>
      </div>
      <Button variant="secondary" className="mt-5 w-full" disabled={!validTarget || probing || installing} onClick={() => void probeHostKey()}>
        {probing ? <RefreshCw className="size-4 animate-spin" /> : <Fingerprint className="size-4" />}
        Probe host key
      </Button>

      {hostKey && hostKeyCurrent && (
        <section className="mt-5 rounded-2xl border border-[var(--accent)]/45 bg-[var(--accent)]/[0.07] p-5" aria-labelledby="ssh-host-identity">
          <div id="ssh-host-identity" className="eyebrow text-[var(--accent)]">host identity</div>
          <div className="mt-3 text-sm font-semibold">{hostKey.algorithm}</div>
          <code className="technical mt-2 block break-all text-sm text-[var(--paper)]">{hostKey.fingerprint}</code>
          <label className="mt-5 flex cursor-pointer items-start gap-3 text-sm leading-5">
            <input className="mt-1 size-4 shrink-0 accent-[var(--signal)]" type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} />
            <span>I verified this fingerprint for {hostKey.host}:{hostKey.port}.</span>
          </label>
        </section>
      )}

      <fieldset className="mt-6">
        <legend className="eyebrow">authentication</legend>
        <div className="mt-2 grid grid-cols-2 gap-2">
          <label className={cn("flex cursor-pointer items-center gap-2 rounded-xl border px-4 py-3 text-sm", auth === "password" ? "border-[var(--accent)] bg-[var(--accent)]/[0.06]" : "border-[var(--line)]")}>
            <input type="radio" name="ssh-auth" value="password" checked={auth === "password"} onChange={() => { setAuth("password"); setPrivateKey(""); setPassphrase(""); }} />
            Password
          </label>
          <label className={cn("flex cursor-pointer items-center gap-2 rounded-xl border px-4 py-3 text-sm", auth === "private-key" ? "border-[var(--accent)] bg-[var(--accent)]/[0.06]" : "border-[var(--line)]")}>
            <input type="radio" name="ssh-auth" value="private-key" checked={auth === "private-key"} onChange={() => { setAuth("private-key"); setPassword(""); }} />
            Private key
          </label>
        </div>
      </fieldset>

      {auth === "password" ? (
        <Field label="root password" htmlFor="ssh-password" className="mt-5">
          <Input id="ssh-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="off" />
        </Field>
      ) : (
        <div className="mt-5 space-y-4">
          <Field label="private key PEM" htmlFor="ssh-private-key">
            <textarea
              id="ssh-private-key"
              className="technical min-h-36 w-full resize-y rounded-2xl border border-[var(--line)] bg-black/20 px-4 py-3 text-xs leading-5 text-[var(--paper)] outline-none transition placeholder:text-[var(--dim)] focus:border-[var(--accent)] focus:ring-4 focus:ring-[color:color-mix(in_oklab,var(--accent)_14%,transparent)]"
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
              value={privateKey}
              onChange={(event) => setPrivateKey(event.target.value)}
              autoComplete="off"
              spellCheck={false}
            />
          </Field>
          <Field label="passphrase (optional)" htmlFor="ssh-passphrase">
            <Input id="ssh-passphrase" type="password" value={passphrase} onChange={(event) => setPassphrase(event.target.value)} autoComplete="off" />
          </Field>
        </div>
      )}
      <FormError error={error} />
      <Button className="mt-6 w-full" size="lg" disabled={!validName || !hostKeyCurrent || !confirmed || !hasCredentials || installing} onClick={() => void install()}>
        {installing ? <RefreshCw className="size-4 animate-spin" /> : <Terminal className="size-4" />}
        Install and connect
      </Button>
    </>
  );
}

function Engines({ catalog }: { catalog: EngineCatalog | null }) {
  const available = catalog?.engines.filter((engine) => engine.adapter_status === "available") ?? [];
  return (
    <div className="enter space-y-8">
      <PageHeading title="Engine catalog" description="Frozen upstream source references used for reproducible builds." />
      <section>
        <SectionHeading title="Available runtime adapters" />
        {available.length === 0 ? (
          <div className="mt-4 rounded-[1.5rem] border border-dashed border-[var(--line)] bg-[var(--panel)] px-6 py-10 text-sm text-[var(--muted)]">
            No runtime adapters are available.
          </div>
        ) : (
          <div className="mt-4 grid gap-3 lg:grid-cols-2">
            {available.map((engine) => <AvailableAdapter key={engine.id} engine={engine} />)}
          </div>
        )}
      </section>
      <section>
        <SectionHeading title="Frozen source catalog" />
        <div className="mt-4 grid gap-4 lg:grid-cols-2 2xl:grid-cols-3">
          {catalog?.engines.map((engine) => <EngineSource key={engine.id} engine={engine} />)}
        </div>
      </section>
    </div>
  );
}

function AvailableAdapter({ engine }: { engine: Engine }) {
  return (
    <article className="rounded-[1.5rem] border border-[var(--signal)]/20 bg-[var(--signal)]/[0.04] p-5">
      <h3 className="font-semibold">{engine.name}</h3>
      <div className="mt-4 flex flex-wrap gap-1.5">
        {engine.protocols.map((protocol) => <span key={protocol} className="rounded-full border border-[var(--line)] px-2.5 py-1 text-[10px] text-[var(--muted)]">{protocol}</span>)}
      </div>
    </article>
  );
}

function EngineSource({ engine }: { engine: Engine }) {
  return (
    <article className="rounded-[1.6rem] border border-[var(--line)] bg-[var(--panel)] p-6">
      <div className="flex items-start justify-between gap-4">
        <h2 className="text-xl font-semibold tracking-[-0.035em]">{engine.name}</h2>
        <a href={engine.repository} target="_blank" rel="noreferrer" className="rounded-full border border-[var(--line)] p-2.5 text-[var(--muted)] transition hover:text-white" aria-label={`Open ${engine.name} repository`}><GitBranch className="size-4" /></a>
      </div>
      <dl className="technical mt-6 grid grid-cols-[auto_1fr] gap-x-4 gap-y-3 text-[11px]">
        <dt className="text-[var(--dim)]">repository</dt><dd className="min-w-0 truncate text-[var(--muted)]" title={engine.repository}>{engine.repository}</dd>
        <dt className="text-[var(--dim)]">tag</dt><dd className="text-[var(--muted)]">{engine.tag || "none"}</dd>
        <dt className="text-[var(--dim)]">commit</dt><dd className="break-all text-[var(--muted)]" title={engine.commit}>{shortCommit(engine.commit)}</dd>
        <dt className="text-[var(--dim)]">license</dt><dd className="text-[var(--muted)]">{engine.license}</dd>
      </dl>
    </article>
  );
}

function Probers() {
  return (
    <div className="enter">
      <PageHeading eyebrow="reachability intelligence" title="Probers" description="Real handshakes from real networks, bound to the interface you choose." />
      <div className="relative mt-8 min-h-[34rem] overflow-hidden rounded-[2rem] border border-[var(--line)] bg-[var(--panel)] p-8 sm:p-12">
        <div className="absolute inset-0 grid-field opacity-60" />
        <div className="relative flex max-w-xl flex-col items-start">
          <div className="grid size-14 place-items-center rounded-2xl bg-[var(--accent)]/10 text-[var(--accent)]"><Radio className="size-6" /></div>
          <h2 className="mt-8 text-3xl font-semibold tracking-[-0.04em]">Observe the network from outside it.</h2>
          <p className="mt-4 text-base leading-7 text-[var(--muted)]">A prober performs a protocol handshake and transfer through `SO_BINDTODEVICE` or an isolated network namespace. Signed reports use TTL, quorum and hysteresis before an endpoint disappears from a subscriber&apos;s view.</p>
          <div className="technical mt-8 rounded-xl border border-[var(--line)] bg-black/25 px-4 py-3 text-xs text-[var(--dim)]">Runtime unavailable</div>
        </div>
      </div>
    </div>
  );
}

function APISurface({ meta }: { meta: Meta | null }) {
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
          <p className="mt-3 text-sm leading-6 text-[var(--muted)]">Opaque sessions are HttpOnly and SameSite strict. Machine identities use a separate mTLS trust path.</p>
          <div className="mt-7 space-y-2">
            {commands.map(([label, command]) => <div key={command} className="flex flex-col gap-1 rounded-xl border border-[var(--line)] bg-black/15 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"><span className="text-xs text-[var(--muted)]">{label}</span><code className="technical break-all text-[11px] text-[var(--paper)]">{command}</code></div>)}
          </div>
        </div>
        <div className="rounded-[1.75rem] border border-[var(--line)] bg-[#0c1011] p-7">
          <div className="flex items-center gap-2 text-xs text-[var(--dim)]"><span className="size-2 rounded-full bg-[var(--danger)]" /><span className="size-2 rounded-full bg-[var(--warning)]" /><span className="size-2 rounded-full bg-[var(--signal)]" /><span className="ml-2 technical">operator@ulcer</span></div>
          <pre className="technical mt-8 overflow-auto text-xs leading-7 text-[var(--muted)]"><code>{`curl --cookie ulcer.cookie \\
  "$ULCER_PANEL/api/v1/instances"

curl --no-buffer --cookie ulcer.cookie \\
  "$ULCER_PANEL/api/v1/events"

# Instance control plane
endpoint: ${meta?.grpc_endpoint ?? "metadata unavailable"}
server name: ${meta?.grpc_server_name ?? "metadata unavailable"}
client certificate required`}</code></pre>
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

  function downloadRecoveryCodes() {
    const contents = [
      "Ulcer recovery codes",
      "",
      "Each code is single-use. Store these offline; they will not be shown again.",
      "",
      ...recoveryCodes,
      "",
    ].join("\n");
    downloadBlob(new Blob([contents], { type: "text/plain;charset=utf-8" }), "ulcer-recovery-codes.txt");
  }

  return (
    <AuthShell>
      {step === "token" && <>
        <AuthHeading title="Frontier Enterprise Proxy panel" text="Enter the setup token generated on this host to configure operator authentication." />
        <label className="eyebrow" htmlFor="setup-token">setup token</label>
        <Input id="setup-token" className="technical mt-2" type="password" value={token} onChange={(event) => setToken(event.target.value)} autoComplete="off" autoFocus />
        <FormError error={error} />
        <Button className="mt-7 w-full" size="lg" disabled={!token || busy} onClick={() => void start()}>{busy ? <RefreshCw className="size-4 animate-spin" /> : <ChevronRight className="size-4" />} Continue</Button>
      </>}
      {step === "totp" && <>
        <AuthHeading title="Bind your authenticator" text="Scan the QR code, or enter the Base32 code manually, then submit a current six-digit code." />
        <div className="flex flex-col items-center rounded-2xl border border-[var(--line)] bg-black/20 p-5">
          <div data-testid="totp-qr" role="img" aria-label="Authenticator QR code" className="rounded-xl bg-white p-3">
            <QRCodeSVG value={uri} size={192} level="M" bgColor="#ffffff" fgColor="#090c0d" title="Ulcer authenticator setup" />
          </div>
          <div className="eyebrow mt-5">manual Base32 code</div>
          <code data-testid="totp-secret" className="technical mt-2 break-all text-center text-sm text-[var(--signal)]">{secret}</code>
        </div>
        <label className="eyebrow mt-6 block" htmlFor="totp-code">six digit code</label>
        <Input id="totp-code" className="technical mt-2 text-center text-xl tracking-[0.35em]" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, ""))} autoFocus />
        <FormError error={error} />
        <Button className="mt-7 w-full" size="lg" disabled={code.length !== 6 || busy} onClick={() => void complete()}>{busy ? <RefreshCw className="size-4 animate-spin" /> : <Check className="size-4" />} Verify authenticator</Button>
      </>}
      {step === "recovery" && <>
        <AuthHeading title="Store your recovery codes" text="These codes are shown once. Each code can be used only one time." />
        <div className="grid grid-cols-2 gap-2 rounded-2xl border border-[var(--warning)]/30 bg-[var(--warning)]/[0.05] p-4">
          {recoveryCodes.map((recoveryCode) => <code key={recoveryCode} className="technical text-center text-xs text-[var(--paper)]">{recoveryCode}</code>)}
        </div>
        <Button variant="secondary" className="mt-4 w-full" onClick={downloadRecoveryCodes}><Download className="size-4" /> Download recovery codes</Button>
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
      <AuthHeading title="Operator authentication" text="Enter a TOTP code or one unused recovery code." />
      <label className="eyebrow" htmlFor="login-code">authentication code</label>
      <Input id="login-code" className="technical mt-2 text-center text-xl tracking-[0.28em]" autoComplete="one-time-code" value={code} onChange={(event) => setCode(event.target.value.toUpperCase())} onKeyDown={(event) => { if (event.key === "Enter" && code) void login(); }} autoFocus />
      <FormError error={error} />
      <Button className="mt-7 w-full" size="lg" disabled={!code || busy} onClick={() => void login()}>{busy ? <RefreshCw className="size-4 animate-spin" /> : <KeyRound className="size-4" />} Enter control plane</Button>
    </AuthShell>
  );
}

function AuthShell({ label, children }: { label?: string; children: React.ReactNode }) {
  return (
    <main className="relative grid min-h-screen place-items-center overflow-hidden px-5 py-10">
      <div className="noise" /><div className="absolute inset-0 grid-field opacity-70" />
      <div className="absolute left-[8%] top-[10%] size-72 rounded-full bg-[var(--signal)]/[0.035] blur-3xl" />
      <section className="enter relative w-full max-w-lg rounded-[2rem] border border-[var(--line)] bg-[color:color-mix(in_oklab,var(--panel)_94%,transparent)] p-7 shadow-2xl backdrop-blur-xl sm:p-10">
        <div className="mb-10 flex items-center justify-between"><div className="flex items-center gap-3"><Logo /><span className="text-lg font-bold tracking-[-0.04em]">ulcer</span></div>{label && <span className="technical text-[10px] uppercase tracking-[0.15em] text-[var(--dim)]">{label}</span>}</div>
        {children}
      </section>
    </main>
  );
}

function ServerClock({ serverTime }: { serverTime: string }) {
  const [now, setNow] = useState(() => new Date(serverTime));
  useEffect(() => {
    const parsed = Date.parse(serverTime);
    const serverAnchor = Number.isNaN(parsed) ? Date.now() : parsed;
    const clientAnchor = Date.now();
    const update = () => setNow(new Date(serverAnchor + Date.now() - clientAnchor));
    update();
    const timer = window.setInterval(update, 1000);
    return () => window.clearInterval(timer);
  }, [serverTime]);
  return <time className="technical mt-0.5 block truncate text-xs text-[var(--muted)] sm:text-sm" dateTime={now.toISOString()}>{formatClock(now)}</time>;
}

function formatClock(value: Date): string {
  const weekdays = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${weekdays[value.getDay()]} ${months[value.getMonth()]} ${pad(value.getDate())} ${pad(value.getHours())}:${pad(value.getMinutes())}:${pad(value.getSeconds())} ${value.getFullYear()}`;
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.append(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
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

function PageHeading({ eyebrow, title, description, action }: { eyebrow?: string; title: string; description?: string; action?: React.ReactNode }) {
  return <header className="flex flex-col justify-between gap-5 sm:flex-row sm:items-end"><div>{eyebrow && <div className="eyebrow">{eyebrow}</div>}<h1 className={cn("text-4xl font-semibold tracking-[-0.05em] sm:text-5xl", eyebrow && "mt-2")}>{title}</h1>{description && <p className="mt-3 max-w-2xl text-sm leading-6 text-[var(--muted)] sm:text-base">{description}</p>}</div>{action}</header>;
}

function SectionHeading({ title, action, onAction }: { title: string; action?: string; onAction?: () => void }) {
  return <div className="flex items-end justify-between gap-4"><h2 className="text-2xl font-semibold tracking-[-0.035em]">{title}</h2>{action && <Button variant="ghost" size="sm" onClick={onAction}>{action}<ChevronRight className="size-3.5" /></Button>}</div>;
}

function Metric({ label, value, detail, danger = false }: { label: string; value: string; detail: string; danger?: boolean }) {
  return <div className="bg-[var(--panel)] p-5 sm:p-6"><div className="eyebrow">{label}</div><div className={cn("technical mt-4 text-3xl font-semibold", danger && "text-[var(--danger)]")}>{value}</div><div className="mt-1 text-[11px] text-[var(--dim)]">{detail}</div></div>;
}

function TrafficMetric({ icon: Icon, label, value }: { icon: typeof ArrowUpFromLine; label: string; value: number }) {
  return <div className="rounded-2xl border border-[var(--line)] bg-black/15 p-4"><Icon className="size-4 text-[var(--muted)]" /><div className="technical mt-4 text-lg">{formatBytes(value)}</div><div className="mt-1 text-[11px] text-[var(--dim)]">{label}</div></div>;
}

function InstanceRow({ instance }: { instance: Instance }) {
  return <div className="flex items-center gap-4 rounded-2xl border border-transparent px-3 py-3 transition hover:border-[var(--line)] hover:bg-black/10"><span className="status-dot" data-state={instance.online ? "online" : instance.phase} /><div className="min-w-0 flex-1"><div className="truncate text-sm font-medium">{instance.name}</div><div className="technical mt-0.5 truncate text-[10px] text-[var(--dim)]">{instance.id}</div></div><div className="text-right"><div className="technical text-xs">g{instance.applied_generation}/{instance.desired_generation}</div><div className="mt-0.5 text-[10px] text-[var(--dim)]">{instance.phase}</div></div></div>;
}

function InstanceCard({ instance, onDelete }: { instance: Instance; onDelete: () => void }) {
  return (
    <article className="rounded-[1.7rem] border border-[var(--line)] bg-[var(--panel)] p-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex min-w-0 items-center gap-3"><span className="status-dot shrink-0" data-state={instance.online ? "online" : instance.phase} /><div className="min-w-0"><h2 className="truncate font-semibold tracking-[-0.02em]">{instance.name}</h2><div className="technical mt-1 truncate text-[10px] text-[var(--dim)]">{instance.id}</div></div></div>
        <span className="shrink-0 rounded-full border border-[var(--line)] px-2.5 py-1 text-[10px] text-[var(--muted)]">{instance.phase}</span>
      </div>
      <div className="mt-7 grid grid-cols-3 gap-2"><SmallStat label="desired" value={`g${instance.desired_generation}`} /><SmallStat label="applied" value={`g${instance.applied_generation}`} /><SmallStat label="agent" value={instance.agent_version || "-"} /></div>
      {instance.reason && <p className="mt-5 border-t border-[var(--line)] pt-4 text-xs leading-5 text-[var(--warning)]">{instance.reason}</p>}
      <div className="mt-5 flex justify-end border-t border-[var(--line)] pt-5"><Button variant="danger" size="sm" onClick={onDelete} aria-label={`Delete ${instance.name}`}><Trash2 className="size-3.5" /> Delete</Button></div>
    </article>
  );
}

function SmallStat({ label, value }: { label: string; value: string }) {
  return <div className="rounded-xl bg-black/15 p-3"><div className="eyebrow !text-[9px]">{label}</div><div className="technical mt-2 truncate text-xs">{value}</div></div>;
}

function EmptyMini({ icon: Icon, text, action, onAction }: { icon: typeof Server; text: string; action: string; onAction: () => void }) {
  return <div className="flex min-h-44 flex-col items-center justify-center rounded-2xl border border-dashed border-[var(--line)] text-center"><Icon className="size-5 text-[var(--dim)]" /><div className="mt-3 text-sm text-[var(--muted)]">{text}</div><Button variant="ghost" size="sm" className="mt-2" onClick={onAction}>{action}<ChevronRight className="size-3" /></Button></div>;
}

function Field({ label, htmlFor, className, children }: { label: string; htmlFor: string; className?: string; children: React.ReactNode }) {
  return <div className={className}><label className="eyebrow mb-2 block" htmlFor={htmlFor}>{label}</label>{children}</div>;
}

function FormError({ error }: { error: string }) {
  return error ? <p role="alert" className="mt-4 text-sm text-[var(--danger)]">{error}</p> : null;
}
