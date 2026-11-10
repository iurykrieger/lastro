import { useState } from "react";
import { Check, FileText, Pencil, RotateCcw, Undo2, X } from "lucide-react";
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
import { AI_I5, reviewedArtifact, type Lastro } from "@/state";
import { PROVABILITY, REASON } from "./verdicts";

const KIND: Record<string, string> = { behavior: "behavior", invariant: "invariant", preserve: "preserve", "non-functional": "non-functional" };
const STATUS: Record<string, string> = { proposed: "awaiting review", approved: "approved", rejected: "rejected", amended: "amended by reviewer" };

export function ProsePanel({ l, hover, setHover }: { l: Lastro; hover: string | null; setHover: (id: string | null) => void }) {
  const a = l.artifact;
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState("");

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
        <Tabs defaultValue="intents">
          <TabsList>
            <TabsTrigger value="prose">Text</TabsTrigger>
            <TabsTrigger value="intents">Intents ({a.intents.length})</TabsTrigger>
            <TabsTrigger value="decisions">Decisions ({a.decisions.length})</TabsTrigger>
          </TabsList>

          <TabsContent value="prose" className="space-y-3">
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
          </TabsContent>

          <TabsContent value="intents" className="space-y-3">
            {l.pending.length > 0 ? (
              <Alert variant="warning">
                <AlertTitle>Review not finished</AlertTitle>
                <AlertDescription>Proofs are derived only from approved, frozen intents. Pending: {l.pending.join("; ")}.</AlertDescription>
              </Alert>
            ) : (
              <p className="text-xs text-muted-foreground">
                The AI misread I5 and the reviewer fixed it. Approve the AI's wrong version and watch the contrast below.
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="outline" onClick={() => l.setArtifact(extractLogin())}>
                <RotateCcw /> Start from the AI draft
              </Button>
              <Button size="sm" variant="outline" onClick={() => l.setArtifact(reviewedArtifact())}>
                <Undo2 /> Restore reviewed example
              </Button>
            </div>
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
                    <span className={cn("ml-auto text-xs text-muted-foreground", i.status === "amended" && "text-trusted", i.status === "proposed" && "text-evidence")}>
                      {STATUS[i.status]}
                    </span>
                  </div>
                  {editing === i.id ? (
                    <form
                      className="grid gap-2"
                      onSubmit={(e) => {
                        e.preventDefault();
                        const v = draft.trim();
                        l.updateArtifact((a2) => {
                          const it = a2.intents.find((x) => x.id === i.id)!;
                          if (v && v !== it.statement) {
                            it.originalStatement ??= it.statement;
                            it.statement = v;
                            it.status = "amended";
                            it.reviewNote = "amended in review";
                          }
                        });
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
                  <p className="text-xs italic text-muted-foreground">“{i.source.quote}”</p>
                  <div className="flex flex-wrap gap-1.5">
                    <Button size="sm" variant="outline" disabled={i.status === "approved"} onClick={() => l.updateArtifact((a2) => void (a2.intents.find((x) => x.id === i.id)!.status = "approved"))}>
                      <Check /> Approve
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
                      onClick={() =>
                        l.updateArtifact((a2) => {
                          const it = a2.intents.find((x) => x.id === i.id)!;
                          it.status = "rejected";
                          it.reviewNote = "rejected in review";
                        })
                      }
                    >
                      <X /> Reject
                    </Button>
                    {i.id === "I5" && i.statement !== AI_I5 && (
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
                          })
                        }
                      >
                        Approve the AI's wrong version
                      </Button>
                    )}
                  </div>
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

          <TabsContent value="decisions" className="space-y-3">
            {a.decisions.map((d) => (
              <div key={d.id} className="grid gap-3 rounded-lg border p-3">
                <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  Decision {d.id} · affects {d.intentIds.join(", ")}
                </p>
                <p className="font-medium">{d.question}</p>
                <RadioGroup
                  value={d.chosen === undefined ? "" : String(d.chosen)}
                  onValueChange={(v) => l.updateArtifact((a2) => void (a2.decisions.find((x) => x.id === d.id)!.chosen = Number(v)))}
                >
                  {d.options.map((o, k) => (
                    <Label
                      key={k}
                      htmlFor={`${d.id}-${k}`}
                      className={cn("flex cursor-pointer items-start gap-3 rounded-md border p-3 font-normal", d.chosen === k && "border-brass bg-brass/5")}
                    >
                      <RadioGroupItem id={`${d.id}-${k}`} value={String(k)} className="mt-0.5" />
                      <span className="grid gap-1">
                        <span className="font-medium">
                          {o.label}
                          {k === d.recommended && <span className="ml-1 text-xs text-muted-foreground">(recommended)</span>}
                        </span>
                        <span className="text-xs text-muted-foreground">{o.consequence}</span>
                      </span>
                    </Label>
                  ))}
                </RadioGroup>
              </div>
            ))}
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}
