export type Status = "failing" | "stale" | "orphaned" | "uncovered" | "unsealed" | "verified";
export const ORDER: Status[] = ["failing", "stale", "orphaned", "uncovered", "unsealed", "verified"];

export interface Op { k: "eq" | "sub" | "del" | "ins"; q?: string; s?: string }
export interface Issue { Code: string; Msg: string }
export interface TestLink { name: string; file: string; line: number; result: string; changed: boolean }
export interface TaskLink { text: string; done: boolean; file: string; line: number }
export interface Change { severity: string; ops: Op[] }
export interface Req {
  id: string; spec: string; local: string; text: string; file: string; line: number; kind: string;
  hash: string; lint?: Issue[]; status: Status; reasons?: string[]; tests: TestLink[]; tasks: TaskLink[];
  sealed?: { seq: number; ts: string; text: string; doc_ver: number; tests: number }; change?: Change;
}
export interface Orphan { req: string; text: string; seq: number; moved_to?: string; match?: number }
export interface Report {
  reqs: Req[]; orphans: Orphan[]; counts: Record<string, number>; warnings: string[] | null;
  run?: { seq: number; ts: string; cmd: string; pass: number; fail: number; skip: number };
  events: number; head: string; key: string; tests: number; untied_tasks?: string[];
}
export interface Info { read_only: boolean; key: string; key_id: string; events: number; head: string }
export interface Rev { ver: number; ts: string; present: boolean; text: string; severity?: string; ops?: Op[] }
export interface Preview { before: Status; after: Status; changed: boolean; change?: Change; reasons?: string[]; lint: Issue[] }
export interface Ev { seq: number; ts: string; type: string; label: string; hash: string; prev: string }
export interface Audit { events: number; chain_ok: boolean; chain_err?: string; head: string; key_id: string; seals: { req: string; seq: number; ok: boolean; err?: string }[] }
export interface Signed { [k: string]: unknown }

async function call<T>(path: string, body?: unknown): Promise<T> {
  const r = await fetch(path, body === undefined ? undefined : {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
  });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error((j as { error?: string }).error ?? `request failed (${r.status})`);
  return j as T;
}

export const api = {
  info: () => call<Info>("/api/info"),
  project: () => call<Report>("/api/project"),
  history: (id: string) => call<Rev[]>(`/api/history?id=${encodeURIComponent(id)}`),
  preview: (id: string, text: string) => call<Preview>("/api/preview", { id, text }),
  rescan: () => call<Report>("/api/rescan", {}),
  seal: (ids?: string[]) => call<{ sealed: unknown[]; skipped: { req: string; why: string }[] }>("/api/seal", { ids: ids ?? [] }),
  attestation: (id: string) => call<Signed>(`/api/attestation?id=${encodeURIComponent(id)}`),
  verify: (s: unknown) => call<{ ok: boolean; signature: string; anchor: string; key?: string }>("/api/attestation/verify", s),
  ledger: () => call<Ev[]>("/api/ledger"),
  audit: () => call<Audit>("/api/audit"),
};
