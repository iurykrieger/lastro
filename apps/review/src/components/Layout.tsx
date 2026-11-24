import { useState, type ReactNode } from "react";
import { AlertCircle, CheckCircle2, Info, UserRound, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Person } from "@/components/bits";
import { href } from "@/lib/router";
import { knownPeople, useWorkspace } from "@/lib/workspace";
import { cn } from "@/lib/utils";

/** Who is reviewing. Remembered in this browser; the server's default is LASTRO_REVIEWER or `git config user.name`. */
function ReviewerPicker() {
  const { reviewer, setReviewer, data } = useWorkspace();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(reviewer);
  const people = knownPeople(data);
  if (!editing && reviewer)
    return (
      <button type="button" className="flex items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-accent" onClick={() => (setName(reviewer), setEditing(true))} aria-label={`Reviewing as ${reviewer}. Change reviewer`}>
        <Person name={reviewer} />
        <span className="hidden sm:inline">
          <span className="text-muted-foreground">Reviewing as </span>
          <strong data-testid="reviewer">{reviewer}</strong>
        </span>
      </button>
    );
  return (
    <form
      className="flex items-center gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        if (!name.trim()) return;
        setReviewer(name);
        setEditing(false);
      }}
    >
      <UserRound className="hidden size-4 text-muted-foreground sm:block" aria-hidden />
      <input
        aria-label="Your name"
        list="lastro-people"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Your name"
        autoFocus={editing}
        className="h-8 w-32 rounded-md border border-input bg-card px-2 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring sm:w-40"
      />
      <datalist id="lastro-people">
        {people.map((p) => (
          <option key={p} value={p} />
        ))}
      </datalist>
      <Button type="submit" size="sm" disabled={!name.trim()}>
        Review as
      </Button>
    </form>
  );
}

function FlashBar() {
  const { flash, dismissFlash } = useWorkspace();
  if (!flash) return null;
  const Icon = flash.kind === "error" ? AlertCircle : flash.kind === "ok" ? CheckCircle2 : Info;
  return (
    <div
      role={flash.kind === "error" ? "alert" : "status"}
      data-testid="flash"
      className={cn(
        "fixed inset-x-4 bottom-4 z-50 mx-auto flex max-w-xl items-start gap-3 rounded-lg border p-3 text-sm shadow-lg",
        flash.kind === "error" ? "border-fail/50 bg-fail-soft text-fail" : flash.kind === "ok" ? "border-proved/50 bg-proved-soft text-proved" : "bg-card",
      )}
    >
      <Icon className="mt-0.5 size-4 shrink-0" aria-hidden />
      <div className="min-w-0 flex-1">
        <p>{flash.text}</p>
        {flash.detail && <pre className="mt-1 whitespace-pre-wrap break-all font-mono text-[11px] text-muted-foreground">{flash.detail}</pre>}
      </div>
      <button type="button" onClick={dismissFlash} aria-label="Dismiss" className="rounded p-0.5 hover:bg-accent">
        <X className="size-4" />
      </button>
    </div>
  );
}

export function Layout({ section, children }: { section: "proposals" | "usecases"; children: ReactNode }) {
  const nav = [
    { key: "proposals", label: "Proposals" },
    { key: "usecases", label: "Use cases" },
  ] as const;
  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-40 border-b bg-background/90 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-7xl items-center gap-3 px-4">
          <a href={href("proposals")} className="flex items-center gap-2 font-semibold">
            <span className="flex size-6 items-center justify-center rounded bg-brass text-[11px] font-bold text-white" aria-hidden>
              L
            </span>
            <span className="hidden sm:inline">Lastro Review</span>
          </a>
          <nav aria-label="Main" className="flex gap-1">
            {nav.map((n) => (
              <a
                key={n.key}
                href={href(n.key)}
                aria-current={section === n.key ? "page" : undefined}
                className={cn("rounded-md px-2.5 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-foreground", section === n.key && "bg-accent font-medium text-foreground")}
              >
                {n.label}
              </a>
            ))}
          </nav>
          <div className="ml-auto">
            <ReviewerPicker />
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-7xl px-4 py-6">{children}</main>
      <FlashBar />
    </div>
  );
}
