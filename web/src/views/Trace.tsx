import { useMemo } from "react";
import { META, StatusBadge } from "@/components/status";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import type { Report, Req, Status } from "@/lib/api";

const ROW = 52;
const STROKE: Record<Status, string> = {
  failing: "#ef4444", stale: "#f59e0b", orphaned: "#d946ef", uncovered: "#94a3b8", unsealed: "#0ea5e9", verified: "#10b981",
};
// line style repeats the status in a second channel: solid = verified, dashed = needs attention
const DASH: Record<Status, string | undefined> = { verified: undefined, unsealed: "2 4", stale: "6 4", failing: "1 3", uncovered: "6 6", orphaned: "6 6" };

export function Trace({ rep, open }: { rep: Report; open: (r: Req) => void }) {
  const tests = useMemo(() => {
    const m = new Map<string, { key: string; name: string; file: string }>();
    rep.reqs.forEach((r) => r.tests.forEach((t) => m.set(t.file + "\0" + t.name, { key: t.file + "\0" + t.name, name: t.name, file: t.file })));
    return [...m.values()];
  }, [rep]);
  const ti = new Map(tests.map((t, i) => [t.key, i]));
  const H = Math.max(rep.reqs.length, tests.length) * ROW;
  const W = 120;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Traceability</CardTitle>
        <CardDescription>Each line joins a requirement to a test that says it covers it. Line style repeats the status: solid is verified, patterned lines need attention.</CardDescription>
      </CardHeader>
      <CardContent>
        {/* wide screens: two columns joined by curves */}
        <div className="hidden md:block">
          <div className="grid" style={{ gridTemplateColumns: `minmax(0,1fr) ${W}px minmax(0,1fr)`, height: H }}>
            <ul>
              {rep.reqs.map((r) => (
                <li key={r.id} style={{ height: ROW }} className="flex items-center">
                  <button onClick={() => open(r)} className="flex w-full min-w-0 items-center gap-2 rounded-lg px-2 py-1 text-left hover:bg-muted">
                    <StatusBadge s={r.status} className="shrink-0" />
                    <span className="min-w-0"><span className="block truncate font-mono text-xs text-muted-foreground">{r.id}</span><span className="block truncate text-sm">{r.text}</span></span>
                  </button>
                </li>
              ))}
            </ul>
            <svg width={W} height={H} aria-hidden="true" className="overflow-visible">
              {rep.reqs.flatMap((r, i) => r.tests.map((t) => {
                const j = ti.get(t.file + "\0" + t.name) ?? 0;
                const y1 = i * ROW + ROW / 2, y2 = j * ROW + ROW / 2;
                return <path key={r.id + t.file + t.name} d={`M0 ${y1} C ${W / 2} ${y1}, ${W / 2} ${y2}, ${W} ${y2}`} fill="none" stroke={STROKE[r.status]} strokeWidth="2" strokeDasharray={DASH[r.status]} strokeLinecap="round" />;
              }))}
            </svg>
            <ul>
              {tests.map((t) => (
                <li key={t.key} style={{ height: ROW }} className="flex items-center">
                  <div className="w-full min-w-0 rounded-lg border bg-muted/40 px-3 py-1.5">
                    <span className="block truncate font-mono text-sm">{t.name}</span>
                    <span className="block truncate font-mono text-xs text-muted-foreground">{t.file}</span>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        </div>
        {/* narrow screens: the same links as a grouped list */}
        <ul className="space-y-3 md:hidden">
          {rep.reqs.map((r) => (
            <li key={r.id} className="rounded-xl border p-3">
              <button onClick={() => open(r)} className="block w-full text-left">
                <StatusBadge s={r.status} />
                <span className="mt-1 block font-mono text-xs text-muted-foreground">{r.id}</span>
                <span className="block text-sm">{r.text}</span>
              </button>
              {r.tests.length === 0 ? <p className="mt-2 text-xs text-muted-foreground">No tests linked</p> : (
                <ul className="mt-2 space-y-1 border-l-2 pl-3" style={{ borderColor: STROKE[r.status] }}>
                  {r.tests.map((t) => <li key={t.file + t.name} className="font-mono text-xs">{t.name}</li>)}
                </ul>
              )}
            </li>
          ))}
        </ul>
        <p className="mt-4 text-xs text-muted-foreground">{Object.keys(META).length} statuses · {tests.length} tests · {rep.reqs.reduce((n, r) => n + r.tests.length, 0)} links</p>
      </CardContent>
    </Card>
  );
}
