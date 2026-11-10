import { useState } from "react";
import { Check, CheckCheck, FileText, Loader2, Pencil, RotateCcw, Sparkles, Undo2, X } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { extractLogin } from "@lastro/example-login";
import { AI_I5, PROSE, REVIEWERS, type Lastro } from "@/state";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { approvalCount, type DecisionCard } from "@lastro/sdk";
import { PROVABILITY, REASON } from "./verdicts";

const KIND: Record<string, string> = { behavior: "behavior", invariant: "invariant", preserve: "preserve", "non-functional": "non-functional" };
const STATUS: Record<string, string> = { proposed: "awaiting review", approved: "approved", rejected: "rejected", amended: "amended by reviewer" };

export function ProsePanel({ l, hover, setHover }: { l: Lastro; hover: string | null; setHover: (id: string | null) => void }) {
  const a = l.artifact;
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [tab, setTab] = useState("intents");
  const [proseDraft, setProseDraft] = useState<string | null>(null);
  const extracting = l.ai.busy === "extract";
  // Each decision sits under the first intent it affects that is still waiting for approval
  // (or its first intent, once they are all approved).
  const isApproved = (id: string) => {
    const it = a.intents.find((x) => x.id === id);
    return !!it && (it.status === "rejected" || approvalCount(it) >= l.requiredApprovals);
  };
  const hosted = new Map<string, DecisionCard[]>();
  const orphans: DecisionCard[] = [];
  for (const d of a.decisions) {
    const host = d.intentIds.find((id) => a.intents.some((x) => x.id === id) && !isApproved(id)) ?? d.intentIds.find((id) => a.intents.some((x) => x.id === id));
    if (!host) orphans.push(d);
    else hosted.set(host, [...(hosted.get(host) ?? []), d]);
  }
  const original = a.prose === PROSE;

  const applyProse = async () => {
    if (proseDraft === null) return;
    if (proseDraft.trim() === a.prose) {
      setProseDraft(null);
      return;
    }
    if (await l.applyProse(proseDraft)) {
      setProseDraft(null);
      setTab("intents");
    }
  };

  // Highlighted prose: cut at every quote boundary.
  const marks = a.intents.filter((i) => i.status !== "rejected").map((i) => ({ ...i.source, id: i.id }));
  const cuts = [...new Set([0, a.prose.length, ...marks.flatMap((m) => [m.start, m.end])])].sort((x, y) => x - y);
  const pieces = cuts.slice(0, -1).map((s, k) => {
    const e = cuts[k + 1]!;
    const ids = marks.filter((m) => m.start <= s && m.end >= e).map((m) => m.id);
    return { text: a.prose.slice(s, e), ids };
  });

  return (
    <Card className="flex min-w-0 flex-col">
      <CardHeader className="pb-3">
        <div className="flex items-center gap-2">
          <FileText className="h-4 w-4 text-brass" />
          <CardTitle className="text-base">Prose</CardTitle>
          <Badge variant="outline" className="ml-auto font-mono text-[11px]">
            {l.frozen ? `intents frozen · ${l.frozen.hash!.slice(7, 19)}` : `${l.pending.length} pending`}
          </Badge>
        </div>
        <CardDescription>The business rule as written, the intents the AI extracted, and your review.</CardDescription>
      </CardHeader>
      <CardContent className="flex-1">
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="prose">Text</TabsTrigger>
            <TabsTrigger value="intents">
              Intents ({a.intents.length}){a.decisions.length ? ` · ${a.decisions.length} decisions` : ""}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="prose" className="space-y-3">
            {proseDraft === null ? (
              <>
                <p className="max-w-[65ch] text-[15px] leading-8">
                  {pieces.map((p, k) =>
                    p.ids.length ? (
                      <mark
                        key={k}
                        className={cn(
                          "rounded-sm border-b-2 border-brass/60 bg-brass/15 px-0.5 text-foreground",
                          p.ids.includes(hover ?? "") && "bg-brass/35",
                        )}
                        onMouseEnter={() => setHover(p.ids[0]!)}
                        onMouseLeave={() => setHover(null)}
                      >
                        {p.text}
                        <sup className="ml-0.5 font-mono text-[10px] text-brass">{p.ids.join("·")}</sup>
                      </mark>
                    ) : (
                      <span key={k}>{p.text}</span>
                    ),
                  )}
                </p>
                <p className="text-xs text-muted-foreground">Each highlight is the exact quote an intent came from (the intent map).</p>
                <div className="flex flex-wrap gap-2">
                  <Button size="sm" onClick={() => setProseDraft(a.prose)}>
                    <Pencil /> Edit the business rule
                  </Button>
                  {!original && (
                    <Button size="sm" variant="outline" onClick={l.restoreExample}>
                      <Undo2 /> Restore original prose
                    </Button>
                  )}
                </div>
              </>
            ) : (
              <div className="grid gap-2">
                <Label htmlFor="prose-editor">Business rule</Label>
                <Textarea id="prose-editor" rows={9} value={proseDraft} onChange={(e) => setProseDraft(e.target.value)} disabled={extracting} className="text-[15px] leading-7" />
                <div className="flex flex-wrap items-center gap-2">
                  <Button size="sm" onClick={applyProse} disabled={extracting || l.ai.available === false || !proseDraft.trim()}>
                    {extracting ? <Loader2 className="animate-spin" /> : <Sparkles />}
                    {extracting ? "Claude is extracting intents…" : "Re-extract intents"}
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => setProseDraft(null)} disabled={extracting}>
                    Cancel
                  </Button>
                  {proseDraft.trim() !== PROSE && (
                    <Button size="sm" variant="ghost" onClick={() => setProseDraft(PROSE)} disabled={extracting}>
                      <Undo2 /> Original text
                    </Button>
                  )}
                </div>
                <p className="text-xs text-muted-foreground">
                  Claude reads the new text and proposes intents. Intents you already reviewed keep your verdict; only what the edit changed comes back for review.
                  Then Claude derives new proof obligations, and they run against the current code. The model's inputs and outputs stay fixed: anything
                  outside them lands in "Where proof stops".
                </p>
                {l.ai.available === false && (
                  <Alert variant="warning">
                    <AlertTitle>Claude is not available here</AlertTitle>
                    <AlertDescription>
                      Open the sandbox published on claude.ai, or run <code className="font-mono">npm run sandbox</code> with <code className="font-mono">ANTHROPIC_API_KEY</code> set.
                      Restoring the original text works without Claude.
                    </AlertDescription>
                  </Alert>
                )}
                {l.ai.error?.step === "extract" && (
                  <Alert variant="destructive">
                    <AlertTitle>Extraction failed</AlertTitle>
                    <AlertDescription className="whitespace-pre-wrap text-xs">{l.ai.error.message}</AlertDescription>
                  </Alert>
                )}
              </div>
            )}
          </TabsContent>

          <TabsContent value="intents" className="space-y-3">
            {l.pending.length > 0 ? (
              <Alert variant="warning">
                <AlertTitle>Review not finished</AlertTitle>
                <AlertDescription className="grid gap-2">
                  <span>Proofs are derived only from approved, frozen intents. {l.pending.length} item(s) pending: see the top of the page.</span>
                  <span>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        l.reviewAs((r) => {
                          for (const d of a.decisions) if (d.chosen === undefined) r.answer(d.id, d.recommended);
                          r.approveAll();
                        })
                      }
                    >
                      <CheckCheck /> Approve everything as {l.reviewer} (recommended answers)
                    </Button>
                  </span>
                </AlertDescription>
              </Alert>
            ) : original ? (
              <p className="text-xs text-muted-foreground">
                The AI misread I5 and the reviewer fixed it. Approve the AI's wrong version and watch the contrast below.
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">Intents extracted by Claude from your edited text. Review them like any AI draft.</p>
            )}
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg bg-muted/60 p-2 text-xs">
              <span className="flex items-center gap-2">
                <span className="text-muted-foreground">Reviewing as</span>
                <ToggleGroup type="single" size="sm" variant="outline" value={l.reviewer} onValueChange={(v) => v && l.setReviewer(v)}>
                  {REVIEWERS.map((r) => (
                    <ToggleGroupItem key={r} value={r} className="h-7 px-2.5 data-[state=on]:border-foreground data-[state=on]:bg-background data-[state=on]:font-semibold">
                      {r}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              </span>
              <span className="flex items-center gap-2">
                <span className="text-muted-foreground">Team policy: approvals per intent</span>
                <ToggleGroup type="single" size="sm" variant="outline" value={String(l.requiredApprovals)} onValueChange={(v) => v && l.setRequiredApprovals(Number(v))}>
                  {[1, 2].map((n) => (
                    <ToggleGroupItem key={n} value={String(n)} className="h-7 w-8 data-[state=on]:border-foreground data-[state=on]:bg-background data-[state=on]:font-semibold">
                      {n}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              </span>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="outline" onClick={() => l.setArtifact({ ...extractLogin(), policy: { requiredApprovals: l.requiredApprovals } })} disabled={!original}>
                <RotateCcw /> Start from the AI draft
              </Button>
              <Button size="sm" variant="outline" onClick={l.restoreExample}>
                <Undo2 /> Restore reviewed example
              </Button>
            </div>
            {orphans.map((d) => (
              <DecisionBlock key={d.id} d={d} l={l} open />
            ))}
            <ul className="grid gap-2">
              {a.intents.map((i) => (
                <li
                  key={i.id}
                  onMouseEnter={() => setHover(i.id)}
                  onMouseLeave={() => setHover(null)}
                  className={cn(
                    "grid gap-2 rounded-lg border p-3 transition-colors",
                    hover === i.id && "border-brass/60",
                    i.status === "proposed" && "border-dashed border-evidence/60",
                    i.status === "amended" && "border-trusted/40",
                    i.status === "rejected" && "opacity-60",
                  )}
                >
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="font-mono text-sm font-semibold">{i.id}</span>
                    <Badge variant="secondary">{KIND[i.kind]}</Badge>
                    <Badge variant={PROVABILITY[i.provability]!.variant}>
                      {PROVABILITY[i.provability]!.label}
                      {i.beyondProofReason ? ` · ${REASON[i.beyondProofReason]}` : ""}
                    </Badge>
                    {i.decisionId && <Badge variant="trusted">from decision {i.decisionId}</Badge>}
                    <span className={cn("ml-auto text-xs text-muted-foreground", i.status === "amended" && "text-trusted", i.status === "proposed" && "text-evidence")}>
                      {STATUS[i.status]}
                      {i.status !== "rejected" && ` · ${approvalCount(i)}/${l.requiredApprovals}`}
                      {i.approvals?.length ? ` (${i.approvals.join(", ")})` : ""}
                    </span>
                  </div>
                  {editing === i.id ? (
                    <form
                      className="grid gap-2"
                      onSubmit={(e) => {
                        e.preventDefault();
                        const v = draft.trim();
                        if (v && v !== i.statement) l.reviewAs((r) => r.amend(i.id, v, {}, `amended by ${l.reviewer}`));
                        setEditing(null);
                      }}
                    >
                      <Label htmlFor={`edit-${i.id}`} className="sr-only">
                        New statement for {i.id}
                      </Label>
                      <Textarea id={`edit-${i.id}`} rows={2} value={draft} onChange={(e) => setDraft(e.target.value)} autoFocus />
                      <div className="flex gap-2">
                        <Button size="sm" type="submit">
                          Save amendment
                        </Button>
                        <Button size="sm" variant="ghost" type="button" onClick={() => setEditing(null)}>
                          Cancel
                        </Button>
                      </div>
                    </form>
                  ) : (
                    <p className={cn("font-medium", i.status === "rejected" && "line-through")}>{i.statement}</p>
                  )}
                  {i.originalStatement && i.originalStatement !== i.statement && (
                    <p className="text-xs text-muted-foreground">
                      The AI understood: <s className="text-fail">{i.originalStatement}</s>
                    </p>
                  )}
                  {i.source.quote && <p className="text-xs italic text-muted-foreground">“{i.source.quote}”</p>}
                  <div className="flex flex-wrap gap-1.5">
                    <Button size="sm" variant="outline" disabled={i.status !== "proposed" && i.status !== "rejected" && (i.approvals ?? []).includes(l.reviewer)} onClick={() => l.reviewAs((r) => r.approve(i.id))}>
                      <Check /> Approve as {l.reviewer}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setDraft(i.statement);
                        setEditing(i.id);
                      }}
                    >
                      <Pencil /> Amend
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={i.status === "rejected"}
                      onClick={() => l.reviewAs((r) => r.reject(i.id, "rejected in review"))}
                    >
                      <X /> Reject
                    </Button>
                    {original && i.id === "I5" && i.statement !== AI_I5 && (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-fail"
                        onClick={() =>
                          l.updateArtifact((a2) => {
                            const it = a2.intents.find((x) => x.id === "I5")!;
                            it.statement = AI_I5;
                            delete it.originalStatement;
                            it.status = "approved";
                            it.approvals = [...REVIEWERS];
                          })
                        }
                      >
                        Approve the AI's wrong version
                      </Button>
                    )}
                  </div>
                  {hosted.get(i.id)?.map((d) => (
                    <DecisionBlock key={d.id} d={d} l={l} open={d.chosen === undefined || !isApproved(i.id)} />
                  ))}
                </li>
              ))}
            </ul>
            {a.notPromised.length > 0 && (
              <div className="rounded-lg bg-muted p-3 text-xs">
                <p className="mb-1 font-semibold uppercase tracking-wide text-muted-foreground">Not promised</p>
                <ul className="list-disc space-y-0.5 pl-4">
                  {a.notPromised.map((n) => (
                    <li key={n}>{n}</li>
                  ))}
                </ul>
              </div>
            )}
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}

/** A decision card inside the intent it affects. Collapsed to its answer once settled. */
function DecisionBlock({ d, l, open }: { d: DecisionCard; l: Lastro; open: boolean }) {
  const [expanded, setExpanded] = useState(open);
  const answered = d.chosen !== undefined;
  const show = expanded || open;
  return (
    <div className={cn("grid gap-2 rounded-md border p-3", answered ? "border-border bg-muted/40" : "border-evidence/60 bg-evidence-soft/40")}>
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <Badge variant={answered ? "secondary" : "evidence"}>Decision {d.id}</Badge>
        <span className="text-muted-foreground">
          affects {d.intentIds.join(", ")}
          {d.origin !== "extraction" && " · from the spec: your answer becomes an intent to approve"}
        </span>
        {answered && !open && (
          <Button size="sm" variant="ghost" className="ml-auto h-6 px-2 text-xs" onClick={() => setExpanded((x) => !x)}>
            {show ? "Hide" : "Change"}
          </Button>
        )}
      </div>
      <p className="text-sm font-medium">{d.question}</p>
      {!show && answered ? (
        <p className="text-xs">
          <span className="text-muted-foreground">Decided: </span>
          {d.options[d.chosen!]!.label}
        </p>
      ) : (
        <RadioGroup value={answered ? String(d.chosen) : ""} onValueChange={(v) => l.reviewAs((r) => r.answer(d.id, Number(v)))}>
          {d.options.map((o, k) => (
            <Label
              key={k}
              htmlFor={`${d.id}-${k}`}
              className={cn("flex cursor-pointer items-start gap-3 rounded-md border bg-background p-2.5 font-normal", d.chosen === k && "border-brass bg-brass/5")}
            >
              <RadioGroupItem id={`${d.id}-${k}`} value={String(k)} className="mt-0.5" />
              <span className="grid gap-0.5">
                <span className="text-sm font-medium">
                  {o.label}
                  {k === d.recommended && <span className="ml-1 text-xs text-muted-foreground">(recommended)</span>}
                </span>
                <span className="text-xs text-muted-foreground">{o.consequence}</span>
              </span>
            </Label>
          ))}
        </RadioGroup>
      )}
    </div>
  );
}
