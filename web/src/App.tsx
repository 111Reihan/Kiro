import { GitCompareArrows, LayoutDashboard, ListChecks, ScrollText, RefreshCw, Stamp } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { api, type Info, type Report, type Req } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Detail } from "@/views/Detail";
import { Ledger } from "@/views/Ledger";
import { Overview } from "@/views/Overview";
import { Requirements } from "@/views/Requirements";
import { Trace } from "@/views/Trace";

const TABS = [
  { id: "overview", label: "Overview", icon: LayoutDashboard },
  { id: "requirements", label: "Requirements", icon: ListChecks },
  { id: "trace", label: "Trace", icon: GitCompareArrows },
  { id: "ledger", label: "Ledger", icon: ScrollText },
] as const;
type Tab = (typeof TABS)[number]["id"];

const Logo = () => (
  <svg width="28" height="28" viewBox="0 0 32 32" aria-hidden="true">
    <rect width="32" height="32" rx="8" fill="#4338ca" />
    <path d="M9 24V14a7 7 0 0 1 14 0v10l-3.5-2.5L16 24l-3.5-2.5z" fill="#fff" />
    <circle cx="13.5" cy="14" r="1.4" fill="#4338ca" /><circle cx="18.5" cy="14" r="1.4" fill="#4338ca" />
  </svg>
);

export default function App() {
  const [tab, setTab] = useState<Tab>(() => (TABS.find((t) => t.id === location.hash.slice(1))?.id ?? "overview"));
  const [rep, setRep] = useState<Report | null>(null);
  const [info, setInfo] = useState<Info | null>(null);
  const [err, setErr] = useState("");
  const [sel, setSel] = useState<Req | null>(null);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");

  const load = useCallback(() => Promise.all([api.project(), api.info()]).then(([r, i]) => { setRep(r); setInfo(i); setErr(""); }).catch((e: Error) => setErr(e.message)), []);
  useEffect(() => { load(); }, [load]);
  useEffect(() => { location.hash = tab; }, [tab]);
  useEffect(() => { const h = () => { const t = TABS.find((x) => x.id === location.hash.slice(1)); if (t) setTab(t.id); }; addEventListener("hashchange", h); return () => removeEventListener("hashchange", h); }, []);

  const act = async (fn: () => Promise<string>) => {
    setBusy(true); setNote("");
    try { setNote(await fn()); await load(); } catch (e) { setNote((e as Error).message); } finally { setBusy(false); }
  };
  const ro = info?.read_only ?? true;

  return (
    <div className="mx-auto min-h-screen max-w-6xl px-4 pb-16 sm:px-6">
      <header className="flex flex-wrap items-center gap-x-6 gap-y-3 py-5">
        <div className="flex items-center gap-2.5"><Logo /><span className="text-lg font-semibold tracking-tight">Specter</span></div>
        <nav aria-label="Views" className="order-last flex w-full gap-1 overflow-x-auto sm:order-none sm:w-auto">
          {TABS.map((t) => (
            <button key={t.id} onClick={() => setTab(t.id)} aria-current={tab === t.id ? "page" : undefined}
              className={cn("flex flex-1 items-center justify-center gap-1.5 whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition-colors sm:flex-none", tab === t.id ? "bg-accent-soft text-accent" : "text-muted-foreground hover:bg-muted")}>
              <t.icon className="h-4 w-4" aria-hidden="true" />{t.label}
            </button>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-2">
          {info && <span className="hidden rounded-full border px-2.5 py-1 font-mono text-xs text-muted-foreground sm:inline" title="Signing key id">key {info.key_id}</span>}
          <button disabled={ro || busy} onClick={() => act(async () => { await api.rescan(); return "Rescanned and recorded."; })}
            title={ro ? "Read-only instance" : "Scan the project and record requirement versions"}
            className="flex items-center gap-1.5 rounded-lg border bg-card px-3 py-2 text-sm font-medium hover:bg-muted disabled:opacity-40">
            <RefreshCw className={cn("h-4 w-4", busy && "animate-spin")} aria-hidden="true" />Rescan
          </button>
          <button disabled={ro || busy} onClick={() => act(async () => { const r = await api.seal(); return `Sealed ${r.sealed.length}, skipped ${r.skipped.length}.`; })}
            title={ro ? "Read-only instance" : "Sign every requirement whose tests passed"}
            className="flex items-center gap-1.5 rounded-lg bg-accent px-3 py-2 text-sm font-semibold text-white hover:opacity-90 disabled:opacity-40">
            <Stamp className="h-4 w-4" aria-hidden="true" />Seal
          </button>
        </div>
      </header>

      {ro && info && <p className="mb-4 rounded-lg border border-sky-200 bg-sky-50 px-4 py-2.5 text-sm text-sky-900">Read-only instance. Everything here is real, including the what-if editor; rescan and seal are disabled. Run <code className="font-mono">specter serve</code> on your own project to use them.</p>}
      <div aria-live="polite">{note && <p className="mb-4 rounded-lg bg-muted px-4 py-2.5 text-sm">{note}</p>}</div>
      {err && <p role="alert" className="rounded-lg bg-red-50 p-4 text-sm text-red-800">Could not load: {err}</p>}
      {!rep && !err && <p className="py-20 text-center text-muted-foreground">Loading…</p>}

      {rep && (
        <main id="main">
          {rep.reqs.length === 0 && rep.orphans.length === 0 ? (
            <div className="rounded-2xl border border-dashed p-10 text-center">
              <h1 className="text-lg font-semibold">No requirements found</h1>
              <p className="mx-auto mt-2 max-w-md text-sm text-muted-foreground">Specter reads Kiro <code className="font-mono">requirements.md</code> files, and lines such as <code className="font-mono">**R1** …</code> or <code className="font-mono">REQ-1: …</code>. Add a <code className="font-mono">// covers: R1</code> comment above a test to link it.</p>
            </div>
          ) : (
            <>
              {tab === "overview" && <Overview rep={rep} open={setSel} />}
              {tab === "requirements" && <Requirements rep={rep} open={setSel} />}
              {tab === "trace" && <Trace rep={rep} open={setSel} />}
              {tab === "ledger" && <Ledger events={rep.events} />}
            </>
          )}
          {rep.warnings && rep.warnings.length > 0 && (
            <details className="mt-6 rounded-xl border bg-card p-4 text-sm"><summary className="cursor-pointer font-medium">{rep.warnings.length} scan warning{rep.warnings.length > 1 ? "s" : ""}</summary>
              <ul className="mt-2 list-disc space-y-1 pl-5 text-muted-foreground">{rep.warnings.map((w) => <li key={w}>{w}</li>)}</ul></details>
          )}
        </main>
      )}
      <Detail req={sel} onClose={() => setSel(null)} />
    </div>
  );
}
