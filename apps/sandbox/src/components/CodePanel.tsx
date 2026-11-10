import { useMemo } from "react";
import { Bug as BugIcon, Code2, Loader2, RotateCcw, Sparkles } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { cn } from "@/lib/utils";
import { BUGS } from "@lastro/example-login";
import { LOGIN_SOURCE, type Lastro } from "@/state";

/** Lines of `next` that are not part of the longest common subsequence with `base`. */
function changedLines(base: string, next: string): Set<number> {
  const a = base.split("\n");
  const b = next.split("\n");
  const dp = Array.from({ length: a.length + 1 }, () => new Array<number>(b.length + 1).fill(0));
  for (let i = a.length - 1; i >= 0; i--) for (let j = b.length - 1; j >= 0; j--) dp[i]![j] = a[i] === b[j] ? dp[i + 1]![j + 1]! + 1 : Math.max(dp[i + 1]![j]!, dp[i]![j + 1]!);
  const changed = new Set<number>();
  let i = 0;
  let j = 0;
  while (j < b.length) {
    if (i < a.length && a[i] === b[j]) {
      i++;
      j++;
    } else if (i < a.length && dp[i + 1]![j]! >= dp[i]![j + 1]!) i++;
    else changed.add(j++);
  }
  return changed;
}

const TOKEN = /(\/\/.*$)|("(?:[^"\\]|\\.)*")|\b(function|return|const|if|else|true|false)\b|\b(\d+)\b/g;

function highlight(line: string) {
  const out: React.ReactNode[] = [];
  let last = 0;
  for (const m of line.matchAll(TOKEN)) {
    out.push(line.slice(last, m.index));
    const cls = m[1] ? "text-muted-foreground italic" : m[2] ? "text-proved" : m[3] ? "text-brass font-medium" : "text-trusted";
    out.push(
      <span key={m.index} className={cls}>
        {m[0]}
      </span>,
    );
    last = m.index! + m[0].length;
  }
  out.push(line.slice(last));
  return out;
}

function CodeEditor({ value, onChange, changed }: { value: string; onChange: (v: string) => void; changed: Set<number> }) {
  const lines = value.split("\n");
  const widthCh = Math.max(...lines.map((x) => x.length)) + 4;
  return (
    <div className="max-h-[560px] overflow-auto rounded-lg border bg-muted/40 font-mono text-[12.5px] leading-5">
      <div className="flex min-w-max">
        <div aria-hidden className="sticky left-0 z-10 select-none border-r bg-muted px-2 py-3 text-right text-muted-foreground">
          {lines.map((_, k) => (
            <div key={k} className={cn(changed.has(k) && "font-semibold text-fail")}>
              {k + 1}
            </div>
          ))}
        </div>
        <div className="relative">
          <pre aria-hidden className="pointer-events-none m-0 px-3 py-3" style={{ width: `${widthCh}ch` }}>
            {lines.map((ln, k) => (
              <div key={k} className={cn("-mx-3 px-3", changed.has(k) && "bg-fail-soft")}>
                {highlight(ln)}
                {"\n"}
              </div>
            ))}
          </pre>
          <label htmlFor="code-editor" className="sr-only">
            Implementation source
          </label>
          <textarea
            id="code-editor"
            value={value}
            onChange={(e) => onChange(e.target.value)}
            spellCheck={false}
            wrap="off"
            className="absolute inset-0 resize-none overflow-hidden bg-transparent px-3 py-3 text-transparent caret-foreground outline-none selection:bg-brass/30 focus-visible:ring-1 focus-visible:ring-ring"
            style={{ width: `${widthCh}ch`, whiteSpace: "pre" }}
            onKeyDown={(e) => {
              if (e.key === "Tab") {
                e.preventDefault();
                const t = e.currentTarget;
                const s = t.selectionStart;
                const v = t.value.slice(0, s) + "  " + t.value.slice(t.selectionEnd);
                onChange(v);
                requestAnimationFrame(() => t.setSelectionRange(s + 2, s + 2));
              }
            }}
          />
        </div>
      </div>
    </div>
  );
}

export function CodePanel({ l }: { l: Lastro }) {
  const changed = useMemo(() => changedLines(LOGIN_SOURCE, l.source), [l.source]);
  const bug = BUGS.find((b) => b.id === l.bug);
  return (
    <Card className="flex min-w-0 flex-col">
      <CardHeader className="pb-3">
        <div className="flex items-center gap-2">
          <Code2 className="h-4 w-4 text-brass" />
          <CardTitle className="text-base">Code</CardTitle>
          <Badge variant="outline" className="ml-auto font-mono text-[11px]">
            decideLogin · {changed.size ? `${changed.size} line(s) changed` : "reference"}
          </Badge>
        </div>
        <CardDescription>The generated implementation. Edit it, plant a bug, or regenerate it from the intents: the proof re-runs on every change.</CardDescription>
      </CardHeader>
      <CardContent className="grid flex-1 content-start gap-3">
        <div className="grid gap-1.5 rounded-lg border border-dashed p-3">
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" onClick={() => void l.regenerateCode()} disabled={!l.plan || !l.ai.available || !!l.ai.busy}>
              {l.ai.busy === "implement" ? <Loader2 className="animate-spin" /> : <Sparkles />}
              {l.ai.busy === "implement" ? "Claude is writing the code…" : "Regenerate code from the intents"}
            </Button>
            <span className="text-xs text-muted-foreground">
              {!l.plan ? "Needs reviewed intents and their proofs first." : l.ai.available === false ? "Needs Claude (published sandbox or dev server)." : "Claude updates the code to satisfy every clause; the proofs then decide."}
            </span>
          </div>
          {l.ai.codeNote && <p className="text-xs"><span className="font-semibold">Claude: </span>{l.ai.codeNote}</p>}
          {l.ai.error?.step === "implement" && (
            <Alert variant="destructive">
              <AlertTitle>Code generation failed</AlertTitle>
              <AlertDescription className="text-xs">{l.ai.error.message}</AlertDescription>
            </Alert>
          )}
        </div>
        <div className="grid gap-1.5">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Plant one bug</p>
          <div className="flex flex-wrap gap-1.5">
            {BUGS.map((b) => (
              <Button key={b.id} size="sm" variant={l.bug === b.id ? "default" : "outline"} onClick={() => l.plantBug(l.bug === b.id ? null : b.id)} aria-pressed={l.bug === b.id}>
                <BugIcon className={cn(l.bug === b.id ? "" : "text-fail")} />
                {b.label}
              </Button>
            ))}
            <Button size="sm" variant="ghost" onClick={() => l.plantBug(null)} disabled={l.source === LOGIN_SOURCE}>
              <RotateCcw /> Reset code
            </Button>
          </div>
        </div>
        {bug && (
          <p className="rounded-md border border-fail/30 bg-fail-soft/50 px-3 py-2 text-xs">
            <span className="font-semibold">Mutant in </span>
            <code className="font-mono">{bug.where}</code>: {bug.explain}
          </p>
        )}
        <CodeEditor value={l.source} onChange={l.setSource} changed={changed} />
        {l.compiled.error && (
          <Alert variant="destructive">
            <AlertTitle>The code does not run</AlertTitle>
            <AlertDescription className="font-mono text-xs">{l.compiled.error}</AlertDescription>
          </Alert>
        )}
        <p className="text-xs text-muted-foreground">
          Calls to <code className="font-mono">step()</code> only draw the timeline in the run view. <code className="font-mono">deny(code)</code> returns a response without a token.
        </p>
      </CardContent>
    </Card>
  );
}
