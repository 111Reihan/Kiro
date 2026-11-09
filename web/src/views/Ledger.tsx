import { CheckCircle2, Link2, ShieldAlert } from "lucide-react";
import { useEffect, useState } from "react";
import { short, when } from "@/components/status";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { api, type Audit, type Ev } from "@/lib/api";
import { cn } from "@/lib/utils";

export function Ledger({ events: nEvents }: { events: number }) {
  const [evs, setEvs] = useState<Ev[]>([]);
  const [au, setAu] = useState<Audit | null>(null);
  const [paste, setPaste] = useState("");
  const [res, setRes] = useState<{ ok: boolean; msg: string } | null>(null);
  useEffect(() => { api.ledger().then(setEvs).catch(() => setEvs([])); api.audit().then(setAu).catch(() => setAu(null)); }, [nEvents]);

  const check = async () => {
    let j: unknown;
    try { j = JSON.parse(paste); } catch { setRes({ ok: false, msg: "That is not JSON." }); return; }
    try {
      const r = await api.verify(j);
      setRes({ ok: r.ok, msg: r.ok ? "Signature and log anchor both check out." + (r.key ? ` Note: ${r.key}.` : "") : `Signature: ${r.signature}. Anchor: ${r.anchor}.` });
    } catch (e) { setRes({ ok: false, msg: (e as Error).message }); }
  };
  const bad = au ? au.seals.filter((s) => !s.ok).length + (au.chain_ok ? 0 : 1) : 0;
  return (
    <div className="grid gap-6 lg:grid-cols-3">
      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle>Hash-chained log</CardTitle>
          <CardDescription>Every event carries the hash of the one before it. Editing or deleting any line breaks every hash after it.</CardDescription>
        </CardHeader>
        <CardContent>
          <ol className="relative space-y-0">
            {[...evs].reverse().map((e, i) => (
              <li key={e.seq} className="relative flex gap-3 pb-4">
                {i < evs.length - 1 && <span className="absolute left-[15px] top-8 h-[calc(100%-1.25rem)] w-px bg-border" aria-hidden="true" />}
                <span className={cn("z-10 grid h-8 w-8 shrink-0 place-items-center rounded-full border bg-card text-xs font-semibold tabular-nums", e.type === "spec.seal" && "border-accent text-accent")}>{e.seq}</span>
                <div className="min-w-0">
                  <p className="text-sm font-medium">{e.label}</p>
                  <p className="text-xs text-muted-foreground">{when(e.ts)} · <span className="font-mono">{e.type}</span></p>
                  <p className="mt-0.5 flex items-center gap-1 font-mono text-xs text-muted-foreground"><Link2 className="h-3 w-3" aria-hidden="true" />{short(e.hash, 14)} ← {short(e.prev, 8)}</p>
                </div>
              </li>
            ))}
          </ol>
        </CardContent>
      </Card>

      <div className="space-y-6">
        <Card>
          <CardHeader><CardTitle>Audit</CardTitle><CardDescription>Re-reads the log from disk and checks every seal.</CardDescription></CardHeader>
          <CardContent className="text-sm">
            {!au ? <p className="text-muted-foreground">Loading…</p> : (
              <>
                <p className={cn("flex items-center gap-2 font-medium", bad === 0 ? "text-emerald-700" : "text-red-700")}>
                  {bad === 0 ? <CheckCircle2 className="h-4 w-4" aria-hidden="true" /> : <ShieldAlert className="h-4 w-4" aria-hidden="true" />}
                  {bad === 0 ? "Chain and seals intact" : `${bad} problem${bad > 1 ? "s" : ""} found`}
                </p>
                <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                  <dt className="text-muted-foreground">Events</dt><dd className="tabular-nums">{au.events}</dd>
                  <dt className="text-muted-foreground">Seals checked</dt><dd className="tabular-nums">{au.seals.length}</dd>
                  <dt className="text-muted-foreground">Head</dt><dd className="font-mono text-xs">{short(au.head, 16)}…</dd>
                  <dt className="text-muted-foreground">Signing key</dt><dd className="font-mono text-xs">{au.key_id}</dd>
                </dl>
                {au.chain_err && <p className="mt-2 text-red-700">{au.chain_err}</p>}
              </>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Verify an attestation</CardTitle><CardDescription>Paste one exported from any Specter ledger.</CardDescription></CardHeader>
          <CardContent>
            <label><span className="sr-only">Attestation JSON</span>
              <textarea value={paste} onChange={(e) => { setPaste(e.target.value); setRes(null); }} rows={5} placeholder='{"v":1,"req":"…"}'
                className="w-full resize-y rounded-xl border bg-card p-3 font-mono text-xs" /></label>
            <button onClick={check} disabled={paste.trim() === ""} className="mt-2 rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-white disabled:opacity-40">Verify</button>
            <div aria-live="polite">{res && <p className={cn("mt-3 rounded-lg p-3 text-sm", res.ok ? "bg-emerald-50 text-emerald-800" : "bg-red-50 text-red-800")}>{res.ok ? "✓ " : "✗ "}{res.msg}</p>}</div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
