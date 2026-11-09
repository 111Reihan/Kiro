import { CircleDashed, Ghost, PenLine, ShieldCheck, Unlink, XCircle } from "lucide-react";
import type { ComponentType } from "react";
import { Badge } from "@/components/ui/badge";
import type { Op, Status } from "@/lib/api";
import { cn } from "@/lib/utils";

// Every status has its own icon and word, so colour is never the only signal.
export const META: Record<Status, { label: string; hint: string; icon: ComponentType<{ className?: string }>; dot: string; chip: string }> = {
  failing: { label: "Failing", hint: "A covering test failed in the latest run", icon: XCircle, dot: "bg-red-500", chip: "bg-red-50 text-red-700" },
  stale: { label: "Stale", hint: "Wording or proving tests changed since the seal", icon: Ghost, dot: "bg-amber-500", chip: "bg-amber-50 text-amber-700" },
  orphaned: { label: "Orphaned", hint: "Sealed once, the requirement is gone", icon: Unlink, dot: "bg-fuchsia-500", chip: "bg-fuchsia-50 text-fuchsia-700" },
  uncovered: { label: "Uncovered", hint: "No test says it covers this", icon: CircleDashed, dot: "bg-slate-400", chip: "bg-slate-100 text-slate-600" },
  unsealed: { label: "Unsealed", hint: "Tests exist, nobody has signed off", icon: PenLine, dot: "bg-sky-500", chip: "bg-sky-50 text-sky-700" },
  verified: { label: "Verified", hint: "Sealed, unchanged, tests passing", icon: ShieldCheck, dot: "bg-emerald-500", chip: "bg-emerald-50 text-emerald-700" },
};

export function StatusBadge({ s, className }: { s: Status; className?: string }) {
  const m = META[s];
  const I = m.icon;
  return (
    <Badge variant="outline" className={cn("whitespace-nowrap", className)} title={m.hint}>
      <span aria-hidden="true" className={cn("size-1.5 rounded-full", m.dot)} />
      <I className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
      {m.label}
    </Badge>
  );
}

/** The ghost diff: words that were there (struck) and words that are there now (marked). */
export function GhostDiff({ ops }: { ops: Op[] }) {
  return (
    <p className="text-[15px] leading-8">
      {ops.map((o, i) => {
        switch (o.k) {
          case "eq": return <span key={i}>{o.s} </span>;
          case "sub": return (<span key={i}><del className="gd"><span className="sr-only">removed: </span>{o.q}</del> <ins className="gd"><span className="sr-only">added: </span>{o.s}</ins> </span>);
          case "del": return <span key={i}><del className="gd"><span className="sr-only">removed: </span>{o.q}</del> </span>;
          default: return <span key={i}><ins className="gd"><span className="sr-only">added: </span>{o.s}</ins> </span>;
        }
      })}
    </p>
  );
}

const SEV: Record<string, string> = {
  material: "bg-red-50 text-red-700 border-red-200",
  minor: "bg-amber-50 text-amber-700 border-amber-200",
  cosmetic: "bg-slate-50 text-slate-600 border-slate-200",
};
export function Severity({ s }: { s?: string }) {
  if (!s) return null;
  return <span className={cn("rounded-md border px-2 py-0.5 text-xs font-semibold", SEV[s] ?? SEV.cosmetic)}>{s} change</span>;
}

export const when = (ts: string) => new Date(ts).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
export const short = (h: string, n = 10) => h.slice(0, n);
