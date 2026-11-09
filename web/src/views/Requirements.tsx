import { Search } from "lucide-react";
import { useMemo, useState } from "react";
import { CardFrame } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableFooter, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { META, StatusBadge } from "@/components/status";
import { ORDER, type Report, type Req, type Status } from "@/lib/api";
import { cn } from "@/lib/utils";

export function Requirements({ rep, open }: { rep: Report; open: (r: Req) => void }) {
  const [f, setF] = useState<Status | "all">("all");
  const [q, setQ] = useState("");
  const rows = useMemo(() => rep.reqs.filter((r) => (f === "all" || r.status === f) &&
    (q === "" || (r.id + " " + r.text).toLowerCase().includes(q.toLowerCase()))), [rep, f, q]);
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex flex-wrap gap-1.5" role="group" aria-label="Filter by status">
          {(["all", ...ORDER] as const).filter((s) => s === "all" || s === "failing" || s === "stale" || s === "uncovered" || s === "unsealed" || s === "verified").map((s) => {
            const n = s === "all" ? rep.reqs.length : rep.counts[s] ?? 0;
            return (
              <button key={s} onClick={() => setF(s)} aria-pressed={f === s}
                className={cn("rounded-full border px-3 py-1 text-sm font-medium transition-colors", f === s ? "border-accent bg-accent text-white" : "bg-card hover:bg-muted")}>
                {s === "all" ? "All" : META[s].label} <span className="tabular-nums opacity-70">{n}</span>
              </button>
            );
          })}
        </div>
        <label className="relative ml-auto w-full sm:w-64">
          <span className="sr-only">Search requirements</span>
          <Search className="pointer-events-none absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" aria-hidden="true" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search id or words"
            className="h-9 w-full rounded-lg border bg-card pl-9 pr-3 text-sm" />
        </label>
      </div>

      <CardFrame>
        <Table variant="card">
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>Requirement</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="hidden md:table-cell">Pattern</TableHead>
              <TableHead className="text-right">Tests</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={r.id} className="cursor-pointer" onClick={() => open(r)}>
                <TableCell className="max-w-[26rem]">
                  <button className="block w-full text-left" onClick={(e) => { e.stopPropagation(); open(r); }}>
                    <span className="block font-mono text-xs text-muted-foreground">{r.id}</span>
                    <span className="block text-sm">{r.text}</span>
                  </button>
                </TableCell>
                <TableCell><StatusBadge s={r.status} /></TableCell>
                <TableCell className="hidden text-sm capitalize text-muted-foreground md:table-cell">{r.kind}{r.lint && r.lint.length > 0 && <span className="ml-2 rounded bg-amber-50 px-1.5 text-xs text-amber-700" title={r.lint.map((l) => l.Msg).join("; ")}>{r.lint.length} lint</span>}</TableCell>
                <TableCell className="text-right tabular-nums">{r.tests.length}</TableCell>
              </TableRow>
            ))}
            {rows.length === 0 && <TableRow><TableCell colSpan={4} className="py-10 text-center text-muted-foreground">No requirement matches.</TableCell></TableRow>}
          </TableBody>
          <TableFooter>
            <TableRow className="hover:bg-transparent">
              <TableCell colSpan={3}>Showing {rows.length} of {rep.reqs.length}</TableCell>
              <TableCell className="text-right tabular-nums">{rows.reduce((n, r) => n + r.tests.length, 0)}</TableCell>
            </TableRow>
          </TableFooter>
        </Table>
      </CardFrame>
    </div>
  );
}
