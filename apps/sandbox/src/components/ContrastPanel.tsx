import { useMemo, useState } from "react";
import { AlertTriangle, CheckCircle2, CircleSlash, Copy, Cpu, GitCompareArrows, Loader2, Play, ScrollText, ShieldCheck, XCircle } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import { explainRun, renderReceipt, show, toSmtLib, type Env } from "@lastro/sdk";
import { STEPS, type Step } from "@lastro/example-login";
import type { Lastro } from "@/state";
import { ClauseVerdictBadge, IntentVerdictBadge, REASON } from "./verdicts";
import { Markdown } from "./Markdown";

const INPUTS: { key: string; label: string; options: [Env[string], string][] }[] = [
  { key: "sends", label: "The request sends", options: [["email", "Email"], ["username", "Username"], ["both", "Both"], ["neither", "Neither"]] },
  { key: "accountFound", label: "Account on file", options: [[true, "Yes"], [false, "No"]] },
  { key: "password", label: "Password", options: [["right", "Right"], ["wrong", "Wrong"], ["bad-hash", "Bad hash"]] },
  { key: "infra", label: "Infrastructure", options: [["ok", "Works"], ["db-fails", "DB fails"], ["signer-fails", "Signer fails"]] },
];

const TRIES: { label: string; input: Env }[] = [
  { label: "Customer signs in", input: { sends: "email", accountFound: true, password: "right", infra: "ok" } },
  { label: "Admin signs in", input: { sends: "username", accountFound: true, password: "right", infra: "ok" } },
  { label: "Wrong password", input: { sends: "email", accountFound: true, password: "wrong", infra: "ok" } },
  { label: "Both fields sent", input: { sends: "both", accountFound: true, password: "right", infra: "ok" } },
];

function requestLine(input: Env): string {
  const body: Record<string, string> = {};
  if (input.sends === "email" || input.sends === "both") body.email = "ana@example.com";
  if (input.sends === "username" || input.sends === "both") body.username = "root";
  body.password = input.password === "right" ? "hunter2" : input.password === "wrong" ? "hunter3" : "hunter2 (hash unreadable)";
  return `POST /api/auth/login ${JSON.stringify(body)}`;
}

function outcome(out: Env) {
  switch (out.status) {
    case 200:
      return { title: "200 with token", detail: `Token for that account, role "${String(out.role)}", expiring 24 hours after the clock reading.` };
    case 400:
      return { title: "400 Bad Request", detail: "Neither an email nor a username was sent." };
    case 401:
      return { title: "401 Unauthorized", detail: "Credentials refused, no token." };
    case 500:
      return { title: "500 Internal Error", detail: "Infrastructure failure, no token." };
    default:
      return { title: "Invalid response", detail: JSON.stringify(out) };
  }
}

export function ContrastPanel({ l, hover, setHover }: { l: Lastro; hover: string | null; setHover: (id: string | null) => void }) {
  const [tab, setTab] = useState("proof");
  const [input, setInput] = useState<Env>({ sends: "email", accountFound: true, password: "right", infra: "ok" });
  const [copied, setCopied] = useState<string | null>(null);
  const { ts, rust, error, ms } = l.results;
  const plan = l.plan;

  const agree = useMemo(() => {
    if (!ts || !rust) return null;
    return ts.clauses.every((c) => rust.clauses.find((r) => r.clauseId === c.clauseId)?.verdict === c.verdict);
  }, [ts, rust]);

  const run = useMemo(() => {
    if (!plan || !l.compiled.fn) return null;
    const steps: Step[] = [];
    try {
      const out = l.compiled.fn(input, steps);
      return { out, steps, ex: explainRun(plan, input, out), error: null as string | null };
    } catch (e) {
      return { out: null, steps, ex: [], error: (e as Error).message };
    }
  }, [plan, l.compiled, input]);

  const receipt = useMemo(() => (ts && plan ? renderReceipt(ts, plan, rust ?? undefined) : ""), [ts, plan, rust]);

  if (!plan) {
    const deriving = l.ai.busy === "derive";
    const failed = l.ai.error?.step === "derive";
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            {deriving ? <Loader2 className="h-4 w-4 animate-spin text-brass" /> : <GitCompareArrows className="h-4 w-4 text-brass" />}
            <CardTitle className="text-base">Contrast: prose × code</CardTitle>
          </div>
          <CardDescription>
            {!l.frozen
              ? "Finish reviewing the intents: proofs are derived only from approved, frozen intents."
              : deriving
                ? "Claude is deriving proof obligations (LSL clauses) from the reviewed intents. This can take a minute."
                : l.ai.available === false
                  ? "Deriving proofs for edited prose needs Claude (published sandbox or dev server)."
                  : "Waiting for the proof obligations."}
          </CardDescription>
        </CardHeader>
        {failed && (
          <CardContent className="grid gap-2">
            <Alert variant="destructive">
              <AlertTitle>Derivation failed</AlertTitle>
              <AlertDescription className="whitespace-pre-wrap text-xs">{l.ai.error!.message}</AlertDescription>
            </Alert>
            <div>
              <Button size="sm" variant="outline" onClick={l.retryDerive}>
                Try again
              </Button>
            </div>
          </CardContent>
        )}
      </Card>
    );
  }

  const loadCounterexample = (inp: Env) => {
    setInput({ ...inp });
    setTab("run");
  };

  return (
    <Card className="min-w-0">
      <CardHeader className="gap-3 pb-3">
        <div className="flex flex-wrap items-center gap-2">
          <GitCompareArrows className="h-4 w-4 text-brass" />
          <CardTitle className="text-base">Contrast: prose × code</CardTitle>
          <span className="text-xs text-muted-foreground">
            {plan.clauses.length} clauses derived from the approved intents {l.planOrigin === "claude" ? "by Claude" : "(reference derivation)"}, checked against the code above
          </span>
        </div>
        <div className="grid gap-2 sm:grid-cols-3">
          <Stat
            icon={ts?.ok ? <ShieldCheck className="text-proved" /> : <XCircle className="text-fail" />}
            title="TypeScript engine"
            value={error ? "error" : ts ? (ts.ok ? "Accepted" : "Rejected") : "…"}
            tone={error || (ts && !ts.ok) ? "fail" : "proved"}
            detail={ts ? `${ts.summary.proved + ts.summary["proved-under-assumptions"]} proved · ${ts.summary.violated} violated · ${ts.summary.gap} gaps · ${ms.toFixed(0)} ms` : (error ?? "")}
          />
          <Stat
            icon={<Cpu className={rust?.ok ? "text-proved" : rust ? "text-fail" : "text-muted-foreground"} />}
            title="Rust prover (WASM)"
            value={l.rustStatus === "failed" ? "unavailable" : rust ? (rust.ok ? "Proved" : "Not proved") : "loading…"}
            tone={rust ? (rust.ok ? "proved" : "fail") : "muted"}
            detail={rust ? `coverage ${rust.coverage.rows}/${rust.coverage.space ?? "∞"} inputs · ${rust.tableDigest.slice(7, 19)}` : "independent check of the same plan"}
          />
          <Stat
            icon={agree === false ? <AlertTriangle className="text-fail" /> : <CheckCircle2 className="text-proved" />}
            title="Engines agree"
            value={agree === null ? "…" : agree ? "Yes, clause by clause" : "No"}
            tone={agree === false ? "fail" : "proved"}
            detail="Two independent implementations of the same semantics"
          />
        </div>
        <div className="flex items-center gap-2">
          <Switch id="trace" checked={l.withTrace} onCheckedChange={l.setWithTrace} />
          <Label htmlFor="trace" className="text-xs font-normal text-muted-foreground">
            Add a production trace where bcrypt accepted a wrong password (breaks trusted piece T2)
          </Label>
        </div>
      </CardHeader>
      <CardContent>
        {error && (
          <Alert variant="destructive" className="mb-3">
            <AlertTitle>Verification could not run</AlertTitle>
            <AlertDescription className="font-mono text-xs">{error}</AlertDescription>
          </Alert>
        )}
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="proof">
              <ShieldCheck className="h-3.5 w-3.5" /> Proof
            </TabsTrigger>
            <TabsTrigger value="run">
              <Play className="h-3.5 w-3.5" /> Run a login
            </TabsTrigger>
            <TabsTrigger value="receipt">
              <ScrollText className="h-3.5 w-3.5" /> Receipt
            </TabsTrigger>
          </TabsList>

          {/* ---------------- Proof ---------------- */}
          <TabsContent value="proof" className="grid gap-5">
            {ts?.decisions.length ? (
              <Alert variant="warning">
                <AlertTitle>The spec contradicts itself</AlertTitle>
                <AlertDescription>{ts.decisions.map((d) => d.question).join(" ")}</AlertDescription>
              </Alert>
            ) : null}

            <section className="grid gap-2">
              <h3 className="text-sm font-semibold">Clauses: what the code must satisfy for every input</h3>
              <Accordion type="multiple" className="rounded-lg border px-3">
                {plan.clauses.map((c, idx) => {
                  const t = ts?.clauses.find((x) => x.clauseId === c.id);
                  const r = rust?.clauses.find((x) => x.clauseId === c.id);
                  const ce = t?.counterexample ?? (t?.traceViolation ? { input: t.traceViolation.input, output: t.traceViolation.output } : undefined);
                  const lit = c.intentIds.includes(hover ?? "");
                  return (
                    <AccordionItem key={c.id} value={c.id} className={cn(lit && "bg-brass/5")}>
                      <AccordionTrigger className="hover:no-underline">
                        <span className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1">
                          <span className="w-5 text-right font-mono text-xs text-muted-foreground">{idx + 1}</span>
                          <span className="min-w-0 flex-1 basis-64 font-normal">{c.description}</span>
                          <span className="flex flex-wrap items-center gap-1.5">
                            {c.intentIds.map((i) => (
                              <Badge key={i} variant="outline" className="font-mono" onMouseEnter={() => setHover(i)} onMouseLeave={() => setHover(null)}>
                                {i}
                              </Badge>
                            ))}
                            {c.assumes?.map((a) => (
                              <Badge key={a} variant="trusted" className="font-mono" title="relies on a trusted piece">
                                {a}
                              </Badge>
                            ))}
                            <span className="text-[11px] text-muted-foreground">TS</span>
                            {t ? <ClauseVerdictBadge verdict={t.verdict} /> : <Badge variant="secondary">…</Badge>}
                            <span className="text-[11px] text-muted-foreground">Rust</span>
                            {r ? <ClauseVerdictBadge verdict={r.verdict} /> : <Badge variant="secondary">…</Badge>}
                          </span>
                        </span>
                      </AccordionTrigger>
                      <AccordionContent className="grid gap-3 pl-8">
                        <div className="grid gap-1">
                          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">LSL</p>
                          <pre className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">
                            {c.requires ? `requires  ${show(c.requires)}\n` : ""}ensures   {show(c.ensures)}
                          </pre>
                        </div>
                        <p className="text-xs text-muted-foreground">
                          {t?.method === "exhaustive" ? `Checked on ${t.checked} applicable inputs out of 72.` : t ? `Checked on ${t.checked} inputs.` : ""}{" "}
                          {t?.mutants ? `Planted mutants rejected: ${t.mutants.killed}/${t.mutants.total}.` : ""} {t?.note ?? ""}
                        </p>
                        {ce && (
                          <div className="flex flex-wrap items-center gap-2 rounded-md bg-fail-soft p-2 text-xs">
                            <span className="font-semibold text-fail">Counterexample</span>
                            <code className="font-mono">{JSON.stringify(ce.input)}</code>→<code className="font-mono">{JSON.stringify(ce.output)}</code>
                            <Button size="sm" variant="outline" className="ml-auto h-7" onClick={() => loadCounterexample(ce.input)}>
                              <Play /> Run this login
                            </Button>
                          </div>
                        )}
                        <details className="text-xs">
                          <summary className="cursor-pointer text-muted-foreground">SMT-LIB2 (for Z3 / cvc5; unsat means proved)</summary>
                          <pre className="mt-2 max-h-56 overflow-auto rounded-md bg-muted p-3 font-mono">{toSmtLib(plan.model, c)}</pre>
                        </details>
                      </AccordionContent>
                    </AccordionItem>
                  );
                })}
              </Accordion>
            </section>

            {ts && (
              <section className="grid gap-2">
                <h3 className="text-sm font-semibold">Intents: what the prose asked for, and how well it is backed</h3>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Intent</TableHead>
                      <TableHead>Verdict</TableHead>
                      <TableHead>Backed by</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {ts.intents.map((i) => (
                      <TableRow key={i.intentId} onMouseEnter={() => setHover(i.intentId)} onMouseLeave={() => setHover(null)} className={cn(hover === i.intentId && "bg-brass/5")}>
                        <TableCell className="min-w-[220px]">
                          <span className="font-mono font-semibold">{i.intentId}</span> {i.statement}
                          {i.notes.length > 0 && <p className="mt-1 text-xs text-muted-foreground">{i.notes.join(" · ")}</p>}
                        </TableCell>
                        <TableCell>
                          <IntentVerdictBadge verdict={i.verdict} />
                        </TableCell>
                        <TableCell className="font-mono text-xs">{[...i.clauses, ...i.assumptions, ...i.scenarios].join(" ") || "—"}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </section>
            )}

            <div className="grid gap-4 lg:grid-cols-2">
              <section className="grid content-start gap-2">
                <h3 className="text-sm font-semibold">Trusted pieces: the edge of the proof</h3>
                {plan.assumptions.map((a) => {
                  const tests = a.checkedBy.map((id) => ({ id, s: ts?.scenarios.find((x) => x.scenarioId === id)?.status }));
                  return (
                    <div key={a.id} className="grid gap-1 rounded-lg border border-dashed border-trusted/50 p-3 text-xs">
                      <div className="flex items-center gap-2">
                        <Badge variant="trusted" className="font-mono">
                          {a.id}
                        </Badge>
                        <code className="font-mono text-sm">{a.piece}</code>
                      </div>
                      <p>{a.contract}</p>
                      <p>
                        <span className="text-muted-foreground">You accept: </span>
                        {a.youAccept}
                      </p>
                      <p>
                        <span className="text-muted-foreground">If it's wrong: </span>
                        {a.ifWrong}
                      </p>
                      <p className="flex flex-wrap items-center gap-1">
                        <span className="text-muted-foreground">Checked by: </span>
                        {tests.map((t) => (
                          <Badge key={t.id} variant={t.s === "passed" ? "evidence" : "fail"} className="font-mono">
                            {t.id} {t.s === "passed" ? "passed" : "failed"}
                          </Badge>
                        ))}
                      </p>
                    </div>
                  );
                })}
              </section>
              <section className="grid content-start gap-2">
                <h3 className="text-sm font-semibold">Where proof stops</h3>
                {plan.beyondProof.map((b) => (
                  <div key={b.intentId} className="grid gap-1 rounded-lg bg-beyond-soft p-3 text-xs">
                    <div className="flex items-center gap-2">
                      <CircleSlash className="h-3.5 w-3.5 text-beyond" />
                      <span className="font-mono font-semibold">{b.intentId}</span>
                      <Badge variant="beyond">{REASON[b.reason]}</Badge>
                    </div>
                    <p>{b.note}</p>
                  </div>
                ))}
                {l.frozen?.notPromised.map((n) => (
                  <p key={n} className="rounded-lg bg-muted p-3 text-xs">
                    <span className="font-semibold">Not promised: </span>
                    {n}
                  </p>
                ))}
              </section>
            </div>
          </TabsContent>

          {/* ---------------- Run ---------------- */}
          <TabsContent value="run">
            <div className="grid gap-6 lg:grid-cols-[minmax(0,340px)_minmax(0,1fr)]">
              <div className="grid content-start gap-3">
                {INPUTS.map((f) => (
                  <div key={f.key} className="grid gap-1.5">
                    <p className="text-xs font-medium text-muted-foreground">{f.label}</p>
                    <ToggleGroup
                      type="single"
                      variant="outline"
                      size="sm"
                      className="flex-wrap justify-start"
                      value={String(input[f.key])}
                      onValueChange={(v) => {
                        if (!v) return;
                        const opt = f.options.find(([val]) => String(val) === v);
                        if (opt) setInput((p) => ({ ...p, [f.key]: opt[0] }));
                      }}
                    >
                      {f.options.map(([v, lab]) => (
                        <ToggleGroupItem key={String(v)} value={String(v)} className="min-w-[4.5rem] flex-1 data-[state=on]:border-foreground data-[state=on]:bg-background data-[state=on]:font-semibold">
                          {lab}
                        </ToggleGroupItem>
                      ))}
                    </ToggleGroup>
                  </div>
                ))}
                <div className="flex flex-wrap gap-1.5">
                  {TRIES.map((t) => (
                    <Button key={t.label} size="sm" variant="secondary" className="h-7 rounded-full" onClick={() => setInput({ ...t.input })}>
                      {t.label}
                    </Button>
                  ))}
                </div>
                <pre className="whitespace-pre-wrap break-words rounded-md bg-muted p-3 font-mono text-xs">{requestLine(input)}</pre>
                {run && (
                  <ol className="grid">
                    {STEPS.map((s, k) => {
                      const st = run.steps.find((x) => x.name === s.name);
                      const fails = run.ex.some((e) => e.status === "fails");
                      const state = !st ? "skip" : st.status === "stop" ? (fails ? "bad" : "stop") : "ok";
                      return (
                        <li key={s.name} className={cn("relative flex gap-3 pb-3", state === "skip" && "opacity-40")}>
                          {k < STEPS.length - 1 && <span className="absolute bottom-0 left-[9px] top-5 w-px bg-border" />}
                          <span
                            className={cn(
                              "z-10 grid h-5 w-5 shrink-0 place-items-center rounded-full border bg-card text-[10px]",
                              state === "ok" && "border-proved text-proved",
                              state === "stop" && "border-evidence text-evidence",
                              state === "bad" && "border-fail bg-fail text-white",
                            )}
                          >
                            {state === "ok" ? "✓" : state === "bad" ? "✕" : state === "stop" ? "■" : ""}
                          </span>
                          <span className="grid">
                            <span className="text-sm font-medium">{s.name}</span>
                            <code className="font-mono text-[11px] text-muted-foreground">{s.fn}</code>
                          </span>
                        </li>
                      );
                    })}
                  </ol>
                )}
                {run?.out && (
                  <div
                    className={cn(
                      "grid gap-0.5 rounded-lg border p-3",
                      run.ex.some((e) => e.status === "fails") ? "border-fail text-fail" : run.out.status === 200 ? "border-proved text-proved" : "",
                    )}
                  >
                    <span className="text-base font-semibold">{outcome(run.out).title}</span>
                    <span className="text-xs">{run.ex.some((e) => e.status === "fails") ? "The spec rejects this response." : outcome(run.out).detail}</span>
                  </div>
                )}
                {run?.error && (
                  <Alert variant="destructive">
                    <AlertDescription className="font-mono text-xs">{run.error}</AlertDescription>
                  </Alert>
                )}
              </div>
              <div className="grid content-start gap-2">
                <p className="text-xs text-muted-foreground">
                  Every clause is an if-then. It only constrains the logins where its if-part is true. The proof tab checks all 72 logins at once; here you see one.
                </p>
                <div className="flex flex-wrap items-baseline justify-between gap-2">
                  <code className="font-mono text-sm">{plan.model.name} ensures</code>
                  {run && (
                    <span className="text-xs text-muted-foreground">
                      {run.ex.some((e) => e.status === "fails")
                        ? `${run.ex.filter((e) => e.status === "fails").length} of ${run.ex.length} clauses fail on this login`
                        : `${run.ex.filter((e) => e.status !== "if-part-false").length} of ${run.ex.length} clauses apply to this login, and all hold`}
                    </span>
                  )}
                </div>
                <ol className="grid gap-1.5">
                  {run?.ex.map((e, k) => {
                    const c = plan.clauses.find((x) => x.id === e.clauseId)!;
                    return (
                      <li
                        key={e.clauseId}
                        className={cn(
                          "flex items-center gap-3 rounded-md border px-3 py-2",
                          e.status === "if-part-false" && "border-transparent bg-muted text-muted-foreground",
                          (e.status === "fails" || e.status === "error") && "border-fail",
                        )}
                      >
                        <span className="w-5 text-right font-mono text-xs text-muted-foreground">{k + 1}</span>
                        <span className="min-w-0 flex-1">{c.description}</span>
                        {e.status === "if-part-false" ? (
                          <Badge variant="outline" className="font-normal text-muted-foreground">
                            If-part false
                          </Badge>
                        ) : e.status === "holds" ? (
                          <Badge variant="proved">✓ Applies, holds</Badge>
                        ) : (
                          <Badge variant="fail">✕ Fails</Badge>
                        )}
                      </li>
                    );
                  })}
                </ol>
              </div>
            </div>
          </TabsContent>

          {/* ---------------- Receipt ---------------- */}
          <TabsContent value="receipt" className="grid gap-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-xs text-muted-foreground">Generated by the SDK for the code above. This is what goes into the pull request.</p>
              <Button
                size="sm"
                variant="outline"
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(receipt);
                    setCopied("Copied");
                  } catch {
                    const pre = document.getElementById("receipt-md");
                    if (pre) window.getSelection()?.selectAllChildren(pre);
                    setCopied("Selected: press Ctrl+C");
                  }
                }}
              >
                <Copy /> {copied ?? "Copy Markdown"}
              </Button>
            </div>
            <Separator />
            <Markdown text={receipt} />
            <details>
              <summary className="cursor-pointer text-xs text-muted-foreground">Raw Markdown</summary>
              <pre id="receipt-md" className="mt-2 whitespace-pre-wrap rounded-md bg-muted p-3 font-mono text-xs">
                {receipt}
              </pre>
            </details>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}

function Stat({ icon, title, value, detail, tone }: { icon: React.ReactNode; title: string; value: string; detail: string; tone: "proved" | "fail" | "muted" }) {
  return (
    <div className="grid gap-0.5 rounded-lg border p-3 [&_svg]:h-4 [&_svg]:w-4">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        {icon}
        {title}
      </div>
      <div className={cn("text-base font-semibold", tone === "proved" && "text-proved", tone === "fail" && "text-fail")}>{value}</div>
      <div className="truncate font-mono text-[11px] text-muted-foreground" title={detail}>
        {detail}
      </div>
    </div>
  );
}
