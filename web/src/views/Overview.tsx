import { Ghost, ShieldCheck, Unlink, XCircle } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { StatCard } from "@/components/ui/stat-card";
import { GhostDiff, META, Severity, StatusBadge, when } from "@/components/status";
import { ORDER, type Report, type Req } from "@/lib/api";

export function Overview({ rep, open }: { rep: Report; open: (r: Req) => void }) {
  const total = rep.reqs.length;
  const c = rep.counts;
  const pct = total ? Math.round(((c.verified ?? 0) / total) * 100) : 0;
  const drift = rep.reqs.filter((r) => r.status === "stale" || r.status === "failing").sort((a, b) => (a.status === b.status ? 0 : a.status === "failing" ? -1 : 1));
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard title="Verified" value={pct} suffix="%" caption={`${c.verified ?? 0} of ${total} requirements`} icon={<ShieldCheck className="h-4 w-4" />} tone="bg-emerald-50 text-emerald-700" />
        <StatCard title="Stale" value={c.stale ?? 0} caption="Words or tests moved after the seal" icon={<Ghost className="h-4 w-4" />} tone="bg-amber-50 text-amber-700" />
        <StatCard title="Failing" value={c.failing ?? 0} caption="Covering test failed last run" icon={<XCircle className="h-4 w-4" />} tone="bg-red-50 text-red-700" />
        <StatCard title="Orphaned" value={c.orphaned ?? 0} caption="Sealed, then the requirement vanished" icon={<Unlink className="h-4 w-4" />} tone="bg-fuchsia-50 text-fuchsia-700" />
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Drift feed</CardTitle>
            <CardDescription>Requirements whose proof no longer matches. Open one to see the ghost diff.</CardDescription>
          </CardHeader>
          <CardContent>
            {drift.length === 0 ? (
              <p className="rounded-lg bg-emerald-50 p-4 text-sm text-emerald-800">Nothing has drifted. Every sealed requirement still reads the way it did when it was proved.</p>
            ) : (
              <ul className="divide-y">
                {drift.map((r) => (
                  <li key={r.id}>
                    <button onClick={() => open(r)} className="block w-full rounded-lg px-2 py-3 text-left hover:bg-muted/60">
                      <div className="mb-1 flex flex-wrap items-center gap-2">
                        <StatusBadge s={r.status} />
                        <span className="font-mono text-xs text-muted-foreground">{r.id}</span>
                        <Severity s={r.change?.severity} />
                      </div>
                      {r.change ? <GhostDiff ops={r.change.ops} /> : <p className="text-sm">{r.text}</p>}
                      {r.reasons && r.reasons.length > 0 && <p className="mt-1 text-sm text-muted-foreground">{r.reasons.join(" · ")}</p>}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Status mix</CardTitle>
              <CardDescription>{total} requirements, {rep.tests} annotated tests</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex h-3 overflow-hidden rounded-full bg-muted" role="img"
                aria-label={ORDER.map((s) => `${c[s] ?? 0} ${s}`).join(", ")}>
                {ORDER.map((s) => (c[s] ?? 0) > 0 && s !== "orphaned" ? <div key={s} className={META[s].dot} style={{ width: `${((c[s] ?? 0) / Math.max(total, 1)) * 100}%` }} /> : null)}
              </div>
              <ul className="mt-4 space-y-2 text-sm">
                {ORDER.map((s) => (
                  <li key={s} className="flex items-center justify-between">
                    <StatusBadge s={s} />
                    <span className="tabular-nums text-muted-foreground">{c[s] ?? 0}</span>
                  </li>
                ))}
              </ul>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Latest test run</CardTitle>
            </CardHeader>
            <CardContent className="text-sm">
              {rep.run ? (
                <>
                  <p className="font-mono text-xs text-muted-foreground">{rep.run.cmd}</p>
                  <p className="mt-2"><b>{rep.run.pass}</b> passed · <b>{rep.run.fail}</b> failed · <b>{rep.run.skip}</b> skipped</p>
                  <p className="mt-1 text-muted-foreground">{when(rep.run.ts)}</p>
                </>
              ) : <p className="text-muted-foreground">No run stored. Use <code className="font-mono">specter run -- go test -json ./...</code></p>}
            </CardContent>
          </Card>
        </div>
      </div>

      {rep.orphans.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Orphans</CardTitle>
            <CardDescription>These were sealed, then removed from the spec. A moved requirement is found by its words.</CardDescription>
          </CardHeader>
          <CardContent>
            <ul className="divide-y">
              {rep.orphans.map((o) => (
                <li key={o.req} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-3 text-sm">
                  <StatusBadge s="orphaned" />
                  <span className="font-mono text-xs text-muted-foreground">{o.req}</span>
                  <span className="min-w-0 basis-full sm:flex-1 sm:basis-0">{o.text}</span>
                  {o.moved_to ? <span className="rounded-md bg-sky-50 px-2 py-0.5 text-xs font-medium text-sky-700">moved to {o.moved_to} · {o.match}% alike</span>
                    : <span className="rounded-md bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600">deleted</span>}
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
