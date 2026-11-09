import { Check, Copy, FileCheck2, FlaskConical, ListChecks } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { GhostDiff, Severity, StatusBadge, when } from "@/components/status";
import { Sheet } from "@/components/ui/sheet";
import { api, type Preview, type Req, type Rev, type Status } from "@/lib/api";
import { cn } from "@/lib/utils";

const RES: Record<string, string> = { pass: "text-emerald-700", fail: "text-red-700", skip: "text-slate-500", none: "text-slate-500" };

function Section({ title, icon, children }: { title: string; icon?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="mt-7 first:mt-0">
      <h3 className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">{icon}{title}</h3>
      {children}
    </section>
  );
}

export function Detail({ req, onClose }: { req: Req | null; onClose: () => void }) {
  const [hist, setHist] = useState<Rev[] | null>(null);
  const [draft, setDraft] = useState("");
  const [pv, setPv] = useState<Preview | null>(null);
  const [err, setErr] = useState("");
  const [att, setAtt] = useState("");
  const [copied, setCopied] = useState(false);
  const seq = useRef(0);

  useEffect(() => {
    setHist(null); setPv(null); setErr(""); setAtt(""); setCopied(false);
    if (!req) return;
    setDraft(req.text);
    api.history(req.id).then(setHist).catch(() => setHist([]));
  }, [req?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // what-if: ask the server what would go stale if the words read like this
  useEffect(() => {
    if (!req || draft.trim() === "" || draft === req.text) { setPv(null); setErr(""); return; }
    const n = ++seq.current;
    const t = setTimeout(() => {
      api.preview(req.id, draft).then((p) => { if (n === seq.current) { setPv(p); setErr(""); } })
        .catch((e: Error) => { if (n === seq.current) { setPv(null); setErr(e.message); } });
    }, 250);
    return () => clearTimeout(t);
  }, [draft, req]);

  const showAtt = async () => {
    if (!req) return;
    try { setAtt(JSON.stringify(await api.attestation(req.id), null, 2)); } catch (e) { setAtt((e as Error).message); }
  };
  const copy = async () => {
    try { await navigator.clipboard.writeText(att); setCopied(true); setTimeout(() => setCopied(false), 1500); } catch { /* clipboard blocked */ }
  };

  return (
    <Sheet open={!!req} onClose={onClose} title={req?.id ?? ""} description={req ? `${req.file}:${req.line}` : undefined}>
      {req && (
        <>
          <div className="mb-4 flex flex-wrap items-center gap-2">
            <StatusBadge s={req.status} />
            <span className="rounded-md bg-muted px-2 py-0.5 text-xs font-medium capitalize text-muted-foreground">{req.kind} pattern</span>
            {req.sealed && <span className="text-xs text-muted-foreground">sealed in event {req.sealed.seq}</span>}
          </div>

          {req.change ? (
            <Section title="Ghost diff: sealed wording against now" icon={<Severity s={req.change.severity} />}>
              <div className="rounded-xl border bg-muted/40 p-4"><GhostDiff ops={req.change.ops} /></div>
            </Section>
          ) : (
            <Section title="Wording"><p className="rounded-xl border bg-muted/40 p-4 text-[15px] leading-7">{req.text}</p></Section>
          )}
          {req.reasons && req.reasons.length > 0 && (
            <ul className="mt-3 list-disc space-y-1 pl-5 text-sm text-muted-foreground">{req.reasons.map((r) => <li key={r}>{r}</li>)}</ul>
          )}
          {req.lint && req.lint.length > 0 && (
            <ul className="mt-3 space-y-1 text-sm">{req.lint.map((l) => <li key={l.Code} className="rounded-md bg-amber-50 px-3 py-1.5 text-amber-800"><b>{l.Code}</b>: {l.Msg}</li>)}</ul>
          )}

          <Section title="What if the words changed?" icon={<FlaskConical className="h-3.5 w-3.5" aria-hidden="true" />}>
            <label className="block">
              <span className="sr-only">Edit the requirement to preview its effect</span>
              <textarea value={draft} onChange={(e) => setDraft(e.target.value)} rows={3}
                className="w-full resize-y rounded-xl border bg-card p-3 text-sm leading-6" />
            </label>
            <p className="mt-1 text-xs text-muted-foreground">Nothing is saved. Specter checks the draft against the sealed wording and the linked tests.</p>
            <div aria-live="polite">
              {err && <p className="mt-2 text-sm text-red-700">{err}</p>}
              {pv && (
                <div className="mt-3 rounded-xl border p-4">
                  <div className="flex flex-wrap items-center gap-2 text-sm">
                    <StatusBadge s={pv.before as Status} /><span aria-label="would become">→</span><StatusBadge s={pv.after as Status} />
                    <Severity s={pv.change?.severity} />
                  </div>
                  {pv.change && <div className="mt-3"><GhostDiff ops={pv.change.ops} /></div>}
                  {!pv.change && <p className="mt-2 text-sm text-muted-foreground">No meaningful change against the sealed wording.</p>}
                  {pv.lint.length > 0 && <p className="mt-2 text-sm text-amber-800">Lint: {pv.lint.map((l) => l.Code).join(", ")}</p>}
                </div>
              )}
            </div>
          </Section>

          <Section title={`Proving tests (${req.tests.length})`}>
            {req.tests.length === 0 ? <p className="text-sm text-muted-foreground">No test says it covers this. Add <code className="font-mono">// covers: {req.id}</code> above one.</p> : (
              <ul className="divide-y rounded-xl border">
                {req.tests.map((t) => (
                  <li key={t.file + t.name} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2.5 text-sm">
                    <span><span className="font-mono">{t.name}</span><span className="block font-mono text-xs text-muted-foreground">{t.file}:{t.line}</span></span>
                    <span className="flex items-center gap-2">
                      {t.changed && <span className="rounded bg-amber-50 px-1.5 text-xs font-medium text-amber-700">edited since seal</span>}
                      <span className={cn("font-medium", RES[t.result])}>{t.result === "none" ? "not in run" : t.result}</span>
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Section>

          {req.tasks.length > 0 && (
            <Section title="Kiro tasks" icon={<ListChecks className="h-3.5 w-3.5" aria-hidden="true" />}>
              <ul className="space-y-1 text-sm">{req.tasks.map((t) => <li key={t.file + t.line}><span aria-hidden="true">{t.done ? "☑" : "☐"}</span><span className="sr-only">{t.done ? "done:" : "open:"}</span> {t.text}</li>)}</ul>
            </Section>
          )}

          <Section title="History of this wording">
            {hist === null ? <p className="text-sm text-muted-foreground">Loading…</p> : hist.length === 0 ? <p className="text-sm text-muted-foreground">Not recorded yet. Run a rescan to start its history.</p> : (
              <ol className="space-y-3 border-l pl-4">
                {hist.map((h) => (
                  <li key={h.ver} className="relative">
                    <span className="absolute -left-[21px] top-1.5 h-2.5 w-2.5 rounded-full border-2 border-accent bg-card" aria-hidden="true" />
                    <p className="text-xs text-muted-foreground">version {h.ver} · {when(h.ts)} <Severity s={h.severity} /></p>
                    {h.ops ? <GhostDiff ops={h.ops} /> : <p className="text-sm">{h.present ? h.text : "removed from the file"}</p>}
                  </li>
                ))}
              </ol>
            )}
          </Section>

          <Section title="Signed attestation" icon={<FileCheck2 className="h-3.5 w-3.5" aria-hidden="true" />}>
            {!req.sealed ? <p className="text-sm text-muted-foreground">Not sealed yet.</p> : att === "" ? (
              <button onClick={showAtt} className="rounded-lg border bg-card px-3 py-1.5 text-sm font-medium hover:bg-muted">Show attestation JSON</button>
            ) : (
              <div className="relative">
                <pre className="max-h-64 overflow-auto rounded-xl border bg-muted/50 p-3 text-xs leading-5">{att}</pre>
                <button onClick={copy} className="absolute right-2 top-2 flex items-center gap-1 rounded-md border bg-card px-2 py-1 text-xs font-medium hover:bg-muted">
                  {copied ? <Check className="h-3.5 w-3.5" aria-hidden="true" /> : <Copy className="h-3.5 w-3.5" aria-hidden="true" />}{copied ? "Copied" : "Copy"}
                </button>
              </div>
            )}
            <p className="mt-2 text-xs text-muted-foreground">Check it anywhere with <code className="font-mono">specter verify FILE -key PUBKEY -log specter.log</code>.</p>
          </Section>
        </>
      )}
    </Sheet>
  );
}
