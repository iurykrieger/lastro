import { useState } from "react";
import { CheckCircle2, MessageSquare, MessageSquarePlus } from "lucide-react";
import type { Proposal, Thread } from "@lastro/sdk";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Person } from "@/components/bits";
import { inline } from "@/components/Markdown";
import { useWorkspace } from "@/lib/workspace";
import { reviewOpen } from "@/lib/review";
import { cn } from "@/lib/utils";

function Composer({ placeholder, submit, onCancel, label }: { placeholder: string; submit: (text: string) => Promise<boolean>; onCancel?: () => void; label: string }) {
  const [text, setText] = useState("");
  const { busy } = useWorkspace();
  return (
    <form
      className="grid gap-2"
      onSubmit={async (e) => {
        e.preventDefault();
        if (text.trim() && (await submit(text))) setText("");
      }}
    >
      <Textarea aria-label={label} placeholder={placeholder} value={text} onChange={(e) => setText(e.target.value)} rows={2} />
      <div className="flex justify-end gap-2">
        {onCancel && (
          <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
            Cancel
          </Button>
        )}
        <Button type="submit" size="sm" disabled={busy || !text.trim()}>
          {label}
        </Button>
      </div>
    </form>
  );
}

export function ThreadView({ p, t, showTarget }: { p: Proposal; t: Thread; showTarget?: boolean }) {
  const { act, reviewer } = useWorkspace();
  const [open, setOpen] = useState(!t.resolved);
  const canResolve = reviewOpen(p) && (reviewer === t.by || reviewer === p.submittedBy);
  return (
    <div className={cn("rounded-lg border bg-card", t.resolved && "bg-muted/40")} data-testid="thread" data-thread={t.id}>
      <button type="button" className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs text-muted-foreground" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        <MessageSquare className="size-3.5" aria-hidden />
        <span className="font-mono">{t.id}</span>
        {showTarget && <span>on {t.on === "prose" ? "the prose" : <span className="font-mono">{t.on}</span>}</span>}
        <span>
          · {t.comments.length} comment{t.comments.length === 1 ? "" : "s"}
        </span>
        {t.resolved && (
          <span className="ml-auto inline-flex items-center gap-1 text-proved">
            <CheckCircle2 className="size-3.5" aria-hidden /> Resolved{t.resolvedBy ? ` by ${t.resolvedBy}` : ""}
          </span>
        )}
      </button>
      {open && (
        <div className="grid gap-3 border-t px-3 py-3">
          {t.comments.map((c, k) => (
            <div key={k} className="flex gap-2.5">
              <Person name={c.by} />
              <div className="min-w-0 flex-1">
                <div className="text-xs font-semibold">{c.by}</div>
                <p className="whitespace-pre-wrap break-words text-sm">{inline(c.text)}</p>
              </div>
            </div>
          ))}
          {reviewOpen(p) && (
            <div className="grid gap-2">
              {!t.resolved && <Composer label="Reply" placeholder="Reply…" submit={async (text) => !!(await act(p.id, { type: "reply", thread: t.id, text }))} />}
              {!t.resolved && canResolve && (
                <div>
                  <Button size="sm" variant="outline" onClick={() => act(p.id, { type: "resolve", thread: t.id })}>
                    <CheckCircle2 aria-hidden /> Resolve thread
                  </Button>
                </div>
              )}
              {!t.resolved && !canResolve && <p className="text-xs text-muted-foreground">Only {t.by === p.submittedBy ? t.by : `${t.by} or ${p.submittedBy}`} can resolve this thread.</p>}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

/** The threads on one thing (an intent, a use case, the prose), and a button to start one. */
export function Threads({ p, on, startLabel = "Comment" }: { p: Proposal; on: string; startLabel?: string }) {
  const { act, reviewer } = useWorkspace();
  const [composing, setComposing] = useState(false);
  const threads = p.threads.filter((t) => t.on === on);
  return (
    <div className="grid gap-2">
      {threads.map((t) => (
        <ThreadView key={t.id} p={p} t={t} />
      ))}
      {reviewOpen(p) &&
        (composing ? (
          <Composer
            label="Start thread"
            placeholder={`Comment on ${on === "prose" ? "the prose" : on}…`}
            onCancel={() => setComposing(false)}
            submit={async (text) => {
              const ok = !!(await act(p.id, { type: "comment", on, text }));
              if (ok) setComposing(false);
              return ok;
            }}
          />
        ) : (
          <div>
            <Button size="sm" variant="ghost" className="text-muted-foreground" onClick={() => setComposing(true)} disabled={!reviewer}>
              <MessageSquarePlus aria-hidden /> {startLabel}
            </Button>
          </div>
        ))}
    </div>
  );
}
