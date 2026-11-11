import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { Proposal } from "@lastro/sdk";
import type { ReviewAction, ReviewResult, WorkspaceSnapshot } from "./api-types";

const REVIEWER_KEY = "lastro-review:reviewer";

export interface Flash {
  kind: "ok" | "error" | "info";
  text: string;
  detail?: string;
}

interface WorkspaceState {
  data?: WorkspaceSnapshot;
  error?: string;
  reviewer: string;
  setReviewer: (name: string) => void;
  /** Apply a review action as the current reviewer. Resolves to the updated proposal, or undefined on failure. */
  act: (proposal: string, action: ReviewAction) => Promise<Proposal | undefined>;
  busy: boolean;
  flash?: Flash;
  dismissFlash: () => void;
  reload: () => Promise<void>;
}

const Ctx = createContext<WorkspaceState | null>(null);

function readStoredReviewer(): string {
  try {
    return localStorage.getItem(REVIEWER_KEY) ?? "";
  } catch {
    return "";
  }
}

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const [data, setData] = useState<WorkspaceSnapshot>();
  const [error, setError] = useState<string>();
  const [stored, setStored] = useState(readStoredReviewer);
  const [busy, setBusy] = useState(false);
  const [flash, setFlash] = useState<Flash>();
  const loading = useRef<Promise<void>>();

  const reload = useCallback(async () => {
    // Coalesce bursts of change events into one request at a time.
    if (loading.current) return loading.current;
    loading.current = (async () => {
      try {
        const r = await fetch("api/workspace", { cache: "no-store" });
        const body = await r.json();
        if (!r.ok) throw new Error(body.error ?? r.statusText);
        setData(body as WorkspaceSnapshot);
        setError(undefined);
      } catch (e) {
        setError((e as Error).message);
      } finally {
        loading.current = undefined;
      }
    })();
    return loading.current;
  }, []);

  useEffect(() => {
    void reload();
    // Live updates: another reviewer on this server, an agent running the CLI, or a git pull.
    const es = typeof EventSource !== "undefined" ? new EventSource("api/events") : undefined;
    es?.addEventListener("change", () => void reload());
    return () => es?.close();
  }, [reload]);

  const reviewer = stored || data?.reviewer || "";

  const setReviewer = useCallback((name: string) => {
    const v = name.trim();
    setStored(v);
    try {
      if (v) localStorage.setItem(REVIEWER_KEY, v);
      else localStorage.removeItem(REVIEWER_KEY);
    } catch {
      // Private windows may refuse storage; the name still holds for this visit.
    }
  }, []);

  const act = useCallback(
    async (proposal: string, action: ReviewAction) => {
      setBusy(true);
      try {
        const r = await fetch(`api/proposals/${encodeURIComponent(proposal)}/review`, {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ as: reviewer, action }),
        });
        const body = await r.json();
        if (r.status === 202) {
          setFlash({ kind: "info", text: body.error, detail: body.request ? `Request: ${body.request}\nAnswer: ${body.answer}` : undefined });
          return undefined;
        }
        if (!r.ok) {
          setFlash({ kind: "error", text: body.error ?? r.statusText });
          return undefined;
        }
        const res = body as ReviewResult;
        setData((d) => (d ? { ...d, proposals: d.proposals.map((p) => (p.id === res.proposal.id ? res.proposal : p)) } : d));
        setFlash(res.note ? { kind: "info", text: res.note } : res.proposal.hash && res.proposal.status === "approved" ? { kind: "ok", text: `${res.proposal.id} is approved: every intent is reviewed, and its proofs can be derived.` } : undefined);
        return res.proposal;
      } catch (e) {
        setFlash({ kind: "error", text: (e as Error).message });
        return undefined;
      } finally {
        setBusy(false);
      }
    },
    [reviewer],
  );

  const value = useMemo<WorkspaceState>(
    () => ({ data, error, reviewer, setReviewer, act, busy, flash, dismissFlash: () => setFlash(undefined), reload }),
    [data, error, reviewer, setReviewer, act, busy, flash, reload],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useWorkspace(): WorkspaceState {
  const v = useContext(Ctx);
  if (!v) throw new Error("useWorkspace outside WorkspaceProvider");
  return v;
}

/** Everyone who appears in the workspace's reviews, for the reviewer picker. */
export function knownPeople(data: WorkspaceSnapshot | undefined): string[] {
  const s = new Set<string>();
  if (data?.reviewer) s.add(data.reviewer);
  for (const p of data?.proposals ?? []) {
    if (p.submittedBy) s.add(p.submittedBy);
    for (const i of p.intents) i.approvals.forEach((a) => s.add(a));
    for (const d of p.decisions) if (d.answeredBy) s.add(d.answeredBy);
    for (const t of p.threads) t.comments.forEach((c) => s.add(c.by));
  }
  return [...s].filter(Boolean).sort((a, b) => a.localeCompare(b));
}
