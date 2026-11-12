using System.Globalization;
using System.Text.Json;
using System.Text.Json.Nodes;

namespace Lastro;

// Derivation, proving, applying, feedback and the receipt of proposal/v1 (see Proposals.cs).
public static partial class Proposals
{
    private static string Declared(IEnumerable<string> names) => names.Any() ? string.Join(", ", names) : "none";

    /// <summary>The JSON an expression field holds: the LLM writes expressions as JSON strings.</summary>
    private static (string Text, JsonNode? Node) ExprText(JsonNode? n)
    {
        if (n is JsonValue v && v.GetValueKind() == JsonValueKind.String)
        {
            var text = v.GetValue<string>();
            try { return (text, JsonNode.Parse(text)); }
            catch (JsonException) { return (text, null); }
        }
        return (n?.ToJsonString() ?? "", n);
    }

    private static bool Blank(JsonNode? n) => n is null || (n is JsonValue v && v.GetValueKind() == JsonValueKind.String && string.IsNullOrWhiteSpace(v.GetValue<string>()))
        || (n is JsonValue nv && nv.GetValueKind() == JsonValueKind.Null) || (n is JsonValue sv && sv.GetValueKind() == JsonValueKind.String && sv.GetValue<string>() == "null");

    private sealed record Raised(ProposalDecision Card, string? Breaks);

    /// <summary>The check of one derived proof (clause or scenario), parsed and validated against its model; problems are added.</summary>
    internal static ProofCheck? CheckFromAnswer(string pid, JsonNode? d, Model model, List<string> problems)
    {
        var kind = S(d?["kind"]);
        Expr? Parse(string where, JsonNode? raw)
        {
            var (text, node) = ExprText(raw);
            if (node is null)
            {
                problems.Add($"{pid}.{where}: not valid JSON: {text}");
                return null;
            }
            var errs = Lsl.Validate(node, model, null, $"{pid}.{where}");
            if (errs.Count > 0)
            {
                problems.AddRange(errs);
                return null;
            }
            return Lsl.Parse(node.ToJsonString());
        }
        ProofCheck? check = null;
        switch (kind)
        {
            case "clause":
                var requires = Blank(d?["requires"]) ? null : Parse("requires", d?["requires"]);
                if (Parse("ensures", d?["ensures"]) is { } ensures)
                    check = new ProofCheck { Kind = "clause", Requires = requires, Ensures = ensures, Assumptions = A(d?["assumptions"]).Select(S).ToList() };
                break;
            case "scenario":
                var (_, inputNode) = ExprText(d?["inputJson"]);
                Env? input = null;
                if (inputNode is not JsonObject inputObj) problems.Add($"{pid}.input: not valid JSON");
                else
                {
                    input = LastroJson.FromNode<Env>(inputObj);
                    foreach (var k in model.Inputs.Keys)
                        if (!inputObj.ContainsKey(k)) problems.Add($"{pid}.input: missing model input {k}");
                }
                if (Parse("expect", d?["expect"]) is { } expect && input is not null) check = new ProofCheck { Kind = "scenario", Input = input, Expect = expect };
                break;
            default:
                problems.Add($"{pid}: unknown kind \"{kind}\"");
                break;
        }
        return check;
    }

    /// <summary>
    /// Checks the derive step's answer and attaches the pending proofs. When derivation returns decisions
    /// (or the new clauses contradict each other), the proposal goes back to review with no proof kept.
    /// </summary>
    public static Proposal FromDerivation(Proposal p, IReadOnlyList<UseCase> useCases, ProofSetup setup, JsonNode? answer, VerifyOptions? opts = null)
    {
        if (!IsApproved(p)) throw new ProposalException([$"{p.Id} is not approved"]);
        var problems = new List<string>();
        var forward = ForwardIntents(p);
        var byId = forward.ToDictionary(i => i.Id, StringComparer.Ordinal);
        var slugs = new HashSet<string>(StringComparer.Ordinal);
        var proofs = new List<ProofEntry>();
        var n = 0;
        foreach (var d in A(answer?["proofs"]))
        {
            var pid = $"{p.Id}.PF-{Pad(++n, 2)}";
            var (intentId, slug, modelName, subject) = (S(d?["intent"]), S(d?["slug"]), S(d?["model"]), S(d?["subject"]));
            var hasIt = byId.TryGetValue(intentId, out var it);
            if (!hasIt) problems.Add($"{pid}: intent {intentId} is not an approved or amended intent of {p.Id}");
            if (!Slug.IsMatch(slug)) problems.Add($"{pid}: slug \"{slug}\" is not kebab-case");
            if (!slugs.Add(slug)) problems.Add($"{pid}: duplicate proof slug \"{slug}\"");
            var hasModel = setup.Models.TryGetValue(modelName, out var model);
            if (!hasModel) problems.Add($"{pid}: unknown model \"{modelName}\" (declared: {Declared(setup.Models.Keys)})");
            if (!setup.Subjects.Contains(subject)) problems.Add($"{pid}: unknown subject \"{subject}\" (declared: {Declared(setup.Subjects)})");
            var replaces = A(d?["replaces"]).Select(S).ToList();
            foreach (var r in replaces)
                if (!(hasIt && it!.Replaces.Any(x => x.Proof == r))) problems.Add($"{pid}: replaces {r}, which its intent does not list");
            if (!hasModel) continue;
            var check = CheckFromAnswer(pid, d, model!, problems);
            if (check is null || !hasIt) continue;
            proofs.Add(new ProofEntry
            {
                Id = pid, Slug = slug, Name = S(d?["name"]), Description = S(d?["description"]), Intent = it!.Id, UseCase = it.UseCase, Model = modelName, Subject = subject,
                Check = check, Replaces = replaces.Count > 0 ? replaces : null, Status = "pending",
            });
        }

        var decided = new List<Raised>();
        foreach (var d in A(answer?["decisions"]))
        {
            var dslug = S(d?["slug"]);
            var kept = new List<string>();
            foreach (var x in A(d?["intents"]).Select(S))
            {
                if (byId.ContainsKey(x)) kept.Add(x);
                else problems.Add($"decision \"{dslug}\": unknown or rejected intent {x}");
            }
            var options = Options(d?["options"]);
            if (options.Count < 2) problems.Add($"decision \"{dslug}\" needs at least two options");
            var breaks = S(d?["breaks"]);
            if (breaks != "" && !useCases.Any(u => u.Proofs.Any(x => x.Id == breaks))) problems.Add($"decision \"{dslug}\": breaks unknown proof {breaks}");
            decided.Add(new Raised(new ProposalDecision
            {
                Slug = dslug, Name = S(d?["name"]), Origin = "derivation", Question = S(d?["question"]), Options = options, Recommended = I(d?["recommended"]), Intents = kept,
            }, breaks == "" ? null : breaks));
        }
        // Coverage: every forward intent has a proof, unless it is beyond proof or a decision covers it.
        foreach (var it in forward)
        {
            if (proofs.Any(x => x.Intent == it.Id) || decided.Any(d => d.Card.Intents.Contains(it.Id))) continue;
            if (it.Change == "remove") problems.Add($"{it.Id}: a remove needs a proof that the old behavior is gone, or a decision");
            else if (it.Provability != "beyond-proof") problems.Add($"{it.Id}: no proof derived");
        }
        if (problems.Count > 0) throw new ProposalException(problems);

        // Contradictions between the new clauses of one model come back as decisions.
        var contradictions = new List<Raised>();
        foreach (var (key, plan) in PlansByGroup(proofs, setup.Models, x => x.Model))
            foreach (var c in Engine.CheckConsistency(plan, opts))
                contradictions.Add(new Raised(new ProposalDecision
                {
                    Slug = $"contradiction-{key}-{contradictions.Count + 1}", Name = $"Contradiction in {key}", Origin = "consistency-check",
                    Question = c.Question, Options = c.Options, Recommended = c.Recommended, Intents = [.. c.IntentIds],
                }, null));

        var next = LastroJson.Clone(p);
        var raised = decided.Concat(contradictions).ToList();
        if (raised.Count == 0)
        {
            next.Proofs = proofs;
            next.LastRun = null;
            return WithStatus(next);
        }
        // Back to review: the approval is withdrawn and no proof is kept.
        foreach (var r in raised)
        {
            if (next.Decisions.Any(x => x.Question == r.Card.Question)) continue;
            r.Card.Id = $"D{next.Decisions.Count + 1}";
            next.Decisions.Add(r.Card);
            if (r.Breaks is null) continue;
            foreach (var iid in r.Card.Intents)
            {
                var it = next.Intents.First(x => x.Id == iid);
                if (!it.Replaces.Any(x => x.Proof == r.Breaks)) it.Replaces.Add(new ProofReplacement { Proof = r.Breaks, Reason = r.Card.Question });
                it.Status = "proposed";
                it.Approvals = [];
            }
        }
        next.Hash = null;
        next.Proofs = [];
        next.LastRun = null;
        next.Supersedes = SupersedesOf(next);
        return WithStatus(next);
    }

    // -----------------------------------------------------------------------
    // Proving: the proposal's proofs plus every use case's (regression)
    // -----------------------------------------------------------------------

    private static List<(string Key, ProofPlan Plan)> PlansByGroup(IEnumerable<ProofEntry> proofs, IReadOnlyDictionary<string, Model> models, Func<ProofEntry, string> key)
    {
        var groups = new List<(string Key, ProofPlan Plan)>();
        foreach (var pr in proofs)
        {
            if (!models.TryGetValue(pr.Model, out var model)) continue;
            var k = key(pr);
            var i = groups.FindIndex(g => g.Key == k);
            if (i < 0)
            {
                groups.Add((k, new ProofPlan { Model = model }));
                i = groups.Count - 1;
            }
            var plan = groups[i].Plan;
            if (pr.Check.Kind == "clause")
                plan.Clauses.Add(new Clause { Id = pr.Id, IntentIds = [pr.Id], Description = pr.Description, Requires = pr.Check.Requires, Ensures = pr.Check.Ensures! });
            else
                plan.Scenarios.Add(new Scenario { Id = pr.Id, IntentIds = [pr.Id], Given = pr.Description, When = "the subject runs", Then = Lsl.Show(pr.Check.Expect!), Input = pr.Check.Input ?? new Env(), Expect = pr.Check.Expect! });
        }
        return groups;
    }

    /// <summary>A synthetic approved artifact, so the engine can run proofs that are not tied to a v0 artifact.</summary>
    private static IntentArtifact ArtifactFor(ProofPlan plan)
    {
        var a = new IntentArtifact();
        foreach (var id in plan.Clauses.Select(c => c.Id).Concat(plan.Scenarios.Select(s => s.Id)))
            a.Intents.Add(new Intent { Id = id, Statement = id, Kind = IntentKind.Behavior, Provability = Provability.Formal, Status = IntentStatus.Approved });
        a.Hash = Canonical.HashOf(a);
        return a;
    }

    /// <summary>An Env with the given keys first (in order), then the rest sorted.</summary>
    private static Env Ordered(Env e, IEnumerable<string> order)
    {
        var o = new Env();
        foreach (var k in order)
            if (e.TryGetValue(k, out var v)) o[k] = v;
        foreach (var k in e.Keys.Where(k => !o.ContainsKey(k)).OrderBy(k => k, StringComparer.Ordinal)) o[k] = e[k];
        return o;
    }

    private static string VerdictOf(ClauseVerdict v) => Wire.Of(v);

    /// <summary>Runs proofs grouped by (model, subject) on the engine, and on the Rust prover when given.</summary>
    public static async Task<(List<ProofOutcome> Outcomes, bool? EnginesAgree)> RunProofsAsync(IReadOnlyList<(ProofEntry Proof, string Role)> proofs, RunOptions o)
    {
        var outcomes = new List<ProofOutcome>();
        bool? agree = null;
        var role = new Dictionary<string, string>(StringComparer.Ordinal);
        foreach (var (pr, r) in proofs) role[pr.Id] = r;
        var runnable = new List<ProofEntry>();
        foreach (var (pr, _) in proofs)
        {
            if (!o.Models.ContainsKey(pr.Model))
                outcomes.Add(new ProofOutcome { Proof = pr.Id, UseCase = pr.UseCase, Intent = pr.Intent, Role = role[pr.Id], Verdict = "not-run", Note = $"model {pr.Model} is not declared" });
            else if (!o.Subjects.ContainsKey(pr.Subject))
                outcomes.Add(new ProofOutcome { Proof = pr.Id, UseCase = pr.UseCase, Intent = pr.Intent, Role = role[pr.Id], Verdict = "not-run", Note = $"subject {pr.Subject} is not available" });
            else runnable.Add(pr);
        }
        foreach (var (key, plan) in PlansByGroup(runnable, o.Models, x => x.Model + "\0" + x.Subject))
        {
            var subject = o.Subjects[key[(key.IndexOf('\0') + 1)..]];
            var artifact = ArtifactFor(plan);
            plan.ArtifactHash = artifact.Hash!;
            var report = await Engine.VerifyAsync(artifact, plan, (o.Verify ?? new VerifyOptions()).With(subject, null)).ConfigureAwait(false);
            if (o.Rust is { } rust && plan.Clauses.Count > 0)
            {
                var table = await RustProver.TabulateAsync(plan.Model, subject).ConfigureAwait(false);
                var rr = rust.Prove(plan, table);
                var same = report.Clauses.All(c => rr.Clauses.FirstOrDefault(r => r.ClauseId == c.ClauseId)?.Verdict == c.Verdict);
                agree = (agree ?? true) && same;
            }
            var outputs = plan.Model.Outputs.Keys.ToList();
            foreach (var r in report.Clauses)
            {
                var pr = runnable.First(x => x.Id == r.ClauseId);
                outcomes.Add(new ProofOutcome
                {
                    Proof = pr.Id, UseCase = pr.UseCase, Intent = pr.Intent, Role = role[pr.Id], Verdict = VerdictOf(r.Verdict),
                    Passed = r.Verdict is ClauseVerdict.Proved or ClauseVerdict.Evidenced, Note = r.Note,
                    Counterexample = r.Counterexample is { } cx ? new ProofCounterexample { Input = Ordered(cx.Input, []), Output = Ordered(cx.Output, outputs) } : null,
                });
            }
            foreach (var r in report.Scenarios)
            {
                var pr = runnable.First(x => x.Id == r.ScenarioId);
                var passed = r.Status == ScenarioStatus.Passed;
                outcomes.Add(new ProofOutcome
                {
                    Proof = pr.Id, UseCase = pr.UseCase, Intent = pr.Intent, Role = role[pr.Id], Verdict = passed ? "evidenced" : "violated", Passed = passed, Note = r.Error,
                    Counterexample = passed ? null : new ProofCounterexample { Input = pr.Check.Input ?? new Env(), Output = r.Output is { } ro ? Ordered(ro, outputs) : null },
                });
            }
        }
        var order = proofs.Select((x, i) => (x.Proof.Id, i)).GroupBy(x => x.Id).ToDictionary(g => g.Key, g => g.First().i, StringComparer.Ordinal);
        return (outcomes.OrderBy(x => order[x.Proof]).ToList(), agree);
    }

    private static List<(string Id, int Proofs)> ResultingUseCases(Proposal p, IReadOnlyList<UseCase> useCases)
    {
        var sup = p.Supersedes.ToHashSet(StringComparer.Ordinal);
        var changes = LiveChanges(p);
        int Own(string id) => p.Proofs.Count(x => x.UseCase == id);
        var o = new List<(string, int)>();
        foreach (var u in useCases)
        {
            if (changes.Any(c => c.UseCase == u.Id && c.Change == "remove")) continue;
            o.Add((u.Id, Own(u.Id) + PassedProofs(u).Count(x => !sup.Contains(x.Id))));
        }
        foreach (var c in changes.Where(c => c.Change == "add")) o.Add((c.UseCase, Own(c.UseCase)));
        return o;
    }

    /// <summary>
    /// Runs the proposal's proofs and every use case's proofs except the superseded ones (a failing one is
    /// a regression). Superseded proofs run too, only to note the ones that still pass.
    /// </summary>
    public static async Task<(Proposal Proposal, ProposalRun Run)> ProveAsync(Proposal p, IReadOnlyList<UseCase> useCases, RunOptions o)
    {
        if (!IsApproved(p)) throw new ProposalException([$"{p.Id} is not approved"]);
        if (p.Proofs.Count == 0 && ForwardIntents(p).Any(i => i.Provability != "beyond-proof")) throw new ProposalException([$"{p.Id} has no proofs: run derive first"]);
        var sup = p.Supersedes.ToHashSet(StringComparer.Ordinal);
        var removed = LiveChanges(p).Where(c => c.Change == "remove").Select(c => c.UseCase).ToHashSet(StringComparer.Ordinal);
        var set = new List<(ProofEntry, string)>();
        set.AddRange(p.Proofs.Select(x => (x, "proposal")));
        set.AddRange(useCases.SelectMany(PassedProofs).Where(x => !sup.Contains(x.Id) && !removed.Contains(x.UseCase)).Select(x => (x, "regression")));
        set.AddRange(useCases.SelectMany(PassedProofs).Where(x => sup.Contains(x.Id)).Select(x => (x, "superseded")));
        var (outcomes, agree) = await RunProofsAsync(set, o).ConfigureAwait(false);
        var run = new ProposalRun { Outcomes = outcomes, EnginesAgree = agree };
        var ok = true;
        foreach (var x in outcomes)
        {
            if (x.Role == "regression" && !x.Passed) run.Regressions.Add(x.Proof);
            else if (x.Role == "superseded" && x.Passed) run.ReplacedStillPass.Add(x.Proof);
            else if (x.Role == "proposal" && !x.Passed) ok = false;
        }
        run.Unproved = ResultingUseCases(p, useCases).Where(u => u.Proofs == 0).Select(u => u.Id).ToList();
        run.Ok = ok && run.Regressions.Count == 0 && agree != false;
        var next = LastroJson.Clone(p);
        foreach (var pr in next.Proofs)
        {
            var r = outcomes.FirstOrDefault(x => x.Role == "proposal" && x.Proof == pr.Id);
            if (r is null) continue;
            pr.Status = r.Passed ? "passed" : "failed";
            pr.Verdict = r.Verdict;
            pr.Evidence = r.Verdict switch { "proved" => "formal", "not-run" => "none", _ => "empirical" };
        }
        next.LastRun = run;
        return (WithStatus(next), run);
    }

    /// <summary>Runs every use case's passed proofs alone (<c>regress</c>).</summary>
    public static async Task<ProposalRun> RegressAsync(IReadOnlyList<UseCase> useCases, RunOptions o)
    {
        var (outcomes, agree) = await RunProofsAsync(useCases.SelectMany(PassedProofs).Select(x => (x, "regression")).ToList(), o).ConfigureAwait(false);
        var run = new ProposalRun
        {
            Outcomes = outcomes, EnginesAgree = agree,
            Regressions = outcomes.Where(x => !x.Passed).Select(x => x.Proof).ToList(),
            Unproved = useCases.Where(u => PassedProofs(u).Count == 0).Select(u => u.Id).ToList(),
        };
        run.Ok = run.Regressions.Count == 0 && agree != false;
        return run;
    }

    // -----------------------------------------------------------------------
    // Apply
    // -----------------------------------------------------------------------

    /// <summary>
    /// Attaches the proofs and syncs the use cases. Refuses unless the proposal is approved, its baseline
    /// still matches and its last run passed every proof, new and regression.
    /// </summary>
    public static Applied Apply(Proposal p, IReadOnlyList<UseCase> useCases)
    {
        var problems = new List<string>();
        if (!IsApproved(p)) problems.Add($"{p.Id} is not approved");
        if (BaselineOf(useCases) != p.Baseline) problems.Add($"the use cases changed since {p.Id} was extracted: run propose again");
        if (p.LastRun is null) problems.Add($"{p.Id} has not been proved: run prove first");
        else if (!p.LastRun.Ok)
            problems.Add($"{p.Id}'s last run failed: {string.Join(", ", p.LastRun.Outcomes.Where(x => x.Role == "proposal" && !x.Passed).Select(x => x.Proof).Concat(p.LastRun.Regressions))}");
        foreach (var pr in p.Proofs)
            if (pr.Status != "passed") problems.Add($"{pr.Id} is {pr.Status}");
        if (problems.Count > 0) throw new ProposalException(problems);
        var sup = p.Supersedes.ToHashSet(StringComparer.Ordinal);
        var o = LastroJson.Clone(useCases.ToList());
        var written = new List<string>();
        var removed = new List<string>();
        void Attach(UseCase u)
        {
            u.Proofs = u.Proofs.Where(x => !sup.Contains(x.Id)).Concat(p.Proofs.Where(x => x.UseCase == u.Id).Select(LastroJson.Clone)).ToList();
            if (!u.History.Contains(p.Id)) u.History.Add(p.Id);
            written.Add(u.Id);
        }
        foreach (var c in LiveChanges(p))
        {
            var u = o.FirstOrDefault(x => x.Id == c.UseCase);
            switch (c.Change)
            {
                case "remove":
                    if (u is not null) o.Remove(u);
                    removed.Add(c.UseCase);
                    break;
                case "add":
                    var nu = new UseCase { Id = c.UseCase, Slug = c.Slug, Name = c.Name, Description = c.After ?? "" };
                    Attach(nu);
                    o.Add(nu);
                    break;
                default:
                    u!.Name = c.Name;
                    u.Description = c.After ?? "";
                    Attach(u);
                    break;
            }
        }
        o = o.OrderBy(x => x.Id, StringComparer.Ordinal).ToList();
        var next = LastroJson.Clone(p);
        next.Status = "applied";
        return new Applied(next, o, written, removed, BaselineOf(o));
    }

    // -----------------------------------------------------------------------
    // Feedback and receipt
    // -----------------------------------------------------------------------

    private static string Js(Env? e) => e is null ? "null" : e.ToString();

    private static string FormalOf(ProofEntry pr) => pr.Check.Kind == "clause"
        ? (pr.Check.Requires is { } r ? $"requires {Lsl.Show(r)}; " : "") + $"ensures {Lsl.Show(pr.Check.Ensures!)}"
        : $"input {Js(pr.Check.Input)}; expect {Lsl.Show(pr.Check.Expect!)}";

    private static List<ProofEntry> AllProofs(Proposal p, IReadOnlyList<UseCase> useCases) => [.. p.Proofs, .. useCases.SelectMany(u => u.Proofs)];

    /// <summary>The last run's failures as a brief for whoever writes the code.</summary>
    public static string Feedback(Proposal p, IReadOnlyList<UseCase> useCases)
    {
        if (p.LastRun is not { } run) return $"{p.Id} has not been proved yet.";
        var all = AllProofs(p, useCases);
        var failing = run.Outcomes.Where(x => x.Role != "superseded" && !x.Passed).ToList();
        if (failing.Count == 0)
            return "Every proof passes." + (run.Unproved.Count > 0 ? $" Use cases with no proofs yet: {string.Join(", ", run.Unproved)}." : "");
        var lines = new List<string> { "The implementation breaks these proofs. Fix the code so each holds, without breaking the others:" };
        foreach (var f in failing)
        {
            var pr = all.FirstOrDefault(x => x.Id == f.Proof);
            var what = f.Role == "regression" ? $"regression in {f.UseCase}" : f.Intent;
            var line = $"- {f.Proof} ({what}): {pr?.Description ?? ""}\n  {(pr is null ? "" : FormalOf(pr))}";
            if (f.Counterexample is { } cx) line += $"\n  counterexample: {Js(cx.Input)} returned {Js(cx.Output)}";
            else if (!string.IsNullOrEmpty(f.Note)) line += $"\n  {f.Note}";
            lines.Add(line);
        }
        return string.Join("\n", lines);
    }

    private static readonly Dictionary<string, string> VerdictMark = new(StringComparer.Ordinal)
    {
        ["proved"] = "✅ proved", ["evidenced"] = "🟡 evidenced", ["violated"] = "❌ violated", ["vacuous"] = "⚠️ vacuous", ["weak"] = "⚠️ weak", ["not-run"] = "⏸ not run",
    };

    /// <summary>The Markdown receipt: the use-case diff, proofs gained and replaced, regressions. Same text as the TS SDK's.</summary>
    public static string RenderReceipt(Proposal p, IReadOnlyList<UseCase> useCases)
    {
        var L = new List<string>
        {
            $"# Receipt: {p.Id} {p.Name}", "",
            $"Status: **{p.Status}** · submitted by {p.SubmittedBy} · baseline `{p.Baseline}`{(p.Hash is not null ? $" · approval `{p.Hash}`" : "")}", "",
            p.Description, "",
        };
        var run = p.LastRun;
        var all = AllProofs(p, useCases);
        L.AddRange(["## Use cases", ""]);
        foreach (var c in p.UseCases)
        {
            var its = p.Intents.Where(i => c.Intents.Contains(i.Id)).ToList();
            var live = its.Count(i => i.Status != "rejected");
            var mark = c.Change == "add" ? "➕" : c.Change == "remove" ? "➖" : "✏️";
            L.AddRange([$"### {mark} {c.UseCase} {c.Name} ({c.Change}{(live == 0 ? ", every intent rejected" : "")})", ""]);
            if (!string.IsNullOrEmpty(c.Before)) L.Add($"- Before: {c.Before}");
            if (!string.IsNullOrEmpty(c.After) && c.Change != "remove") L.Add($"- After: {c.After}");
            foreach (var i in its)
            {
                L.Add($"- **{i.Id}** `{i.Status}` {i.Description}");
                foreach (var r in i.Replaces)
                {
                    var old = all.FirstOrDefault(x => x.Id == r.Proof);
                    var by = p.Proofs.Where(x => x.Replaces?.Contains(r.Proof) == true).Select(x => x.Id).ToList();
                    L.Add($"  - replaces {r.Proof}{(old is not null ? $" ({old.Description})" : "")}: {r.Reason}{(by.Count > 0 ? $"; now guarded by {string.Join(", ", by)}" : "")}");
                }
            }
            var gained = p.Proofs.Where(x => x.UseCase == c.UseCase).Select(x => x.Id).ToList();
            if (gained.Count > 0) L.Add($"- Gains: {string.Join(", ", gained)}");
            L.Add("");
        }
        ProofOutcome? Outcome(string id) => run?.Outcomes.FirstOrDefault(x => x.Proof == id);
        if (p.Proofs.Count > 0)
        {
            L.AddRange(["## Proofs", ""]);
            foreach (var pr in p.Proofs)
            {
                var r = Outcome(pr.Id);
                L.Add($"- **{pr.Id}** {(r is not null ? VerdictMark[r.Verdict] : pr.Status)} — {pr.Description} ({pr.Intent}, {pr.Model}/{pr.Subject})");
                L.Add($"  - `{FormalOf(pr)}`");
                if (r?.Counterexample is { } cx) L.Add($"  - Counterexample: `{Js(cx.Input)}` → `{Js(cx.Output)}`");
                if (!string.IsNullOrEmpty(r?.Note)) L.Add($"  - Note: {r.Note}");
            }
            L.Add("");
        }
        if (run is not null)
        {
            var engines = run.EnginesAgree is { } ea ? ea ? " Engines agree: yes." : " Engines agree: NO." : "";
            L.AddRange(["## Regression", "", $"{run.Outcomes.Count(x => x.Role == "regression")} use-case proof(s) ran; {run.Regressions.Count} failed.{engines}", ""]);
            foreach (var id in run.Regressions)
            {
                var r = Outcome(id)!;
                var detail = r.Counterexample is { } cx ? $": `{Js(cx.Input)}` → `{Js(cx.Output)}`" : !string.IsNullOrEmpty(r.Note) ? $": {r.Note}" : "";
                L.Add($"- ❌ **{id}** ({r.UseCase}){detail}");
            }
            foreach (var id in run.ReplacedStillPass) L.Add($"- ⚠️ **{id}** is superseded but still passes: the behavior it guarded may not have changed.");
            if (run.Unproved.Count > 0) L.Add($"- Unproved use cases (no proofs yet): {string.Join(", ", run.Unproved)}");
            L.Add("");
        }
        if (p.Decisions.Count > 0)
        {
            L.AddRange(["## Decisions", ""]);
            foreach (var d in p.Decisions)
            {
                var answer = d.Chosen is { } k ? d.Options[k].Label + (d.AnsweredBy is not null ? $" ({d.AnsweredBy})" : "") : "unanswered";
                L.Add($"- **{d.Id}** {d.Question} → {answer}");
            }
            L.Add("");
        }
        if (p.NotPromised.Count > 0)
        {
            L.AddRange(["## Not promised", ""]);
            L.AddRange(p.NotPromised.Select(x => $"- {x}"));
            L.Add("");
        }
        return string.Join("\n", L);
    }

    // -----------------------------------------------------------------------
    // LLM steps: extract (propose, recombine) and derive
    // -----------------------------------------------------------------------

    private static JsonArray ProofView(UseCase? u) => new([.. (u?.Proofs ?? []).Select(p => (JsonNode)new JsonObject
    {
        ["id"] = p.Id, ["name"] = p.Name, ["description"] = p.Description,
        ["formal"] = p.Check.Kind == "clause"
            ? (p.Check.Requires is { } r ? Lsl.Show(r) + " => " : "") + Lsl.Show(p.Check.Ensures!)
            : Js(p.Check.Input) + " => " + Lsl.Show(p.Check.Expect!),
    })]);

    /// <summary>The user turn of the propose step: the use cases and the prose.</summary>
    public static string ProposeUserPrompt(string prose, IReadOnlyList<UseCase> useCases)
    {
        var cases = new JsonArray();
        foreach (var u in useCases)
        {
            var o = new JsonObject { ["slug"] = u.Slug, ["name"] = u.Name, ["description"] = u.Description };
            if (u.Proofs.Count > 0) o["proofs"] = ProofView(u);
            cases.Add(o);
        }
        return $"<use-cases>\n{JsJson.Stringify(cases, 2)}\n</use-cases>\n\n<prose>\n{prose}\n</prose>";
    }

    public static LlmRequest ProposeRequest(string prose, IReadOnlyList<UseCase> useCases) =>
        new("extract", LlmPrompts.ProposeSystem, ProposeUserPrompt(prose, useCases), (JsonObject)JsonNode.Parse(LlmPrompts.ProposeSchemaJson)!);

    /// <summary>Asks the LLM for a proposal and checks its answer.</summary>
    public static async Task<Proposal> ExtractAsync(AskJson ask, ProposeInput input, CancellationToken cancellationToken = default) =>
        FromExtraction(input, await ask(ProposeRequest(input.Prose, input.UseCases), cancellationToken).ConfigureAwait(false));

    /// <summary>The user turn of a recombination: the use case, its remaining intents and the reviewer's instructions.</summary>
    public static string RecombineUserPrompt(Proposal p, string useCase, string? instructions = null)
    {
        var c = p.UseCases.FirstOrDefault(x => x.UseCase == useCase || x.Slug == useCase) ?? throw new InvalidOperationException($"{useCase} is not touched by {p.Id}");
        var intents = new JsonArray([.. p.Intents.Where(i => c.Intents.Contains(i.Id) && i.Status != "rejected")
            .Select(i => (JsonNode)new JsonObject { ["id"] = i.Id, ["change"] = i.Change, ["description"] = i.Description })]);
        var o = new JsonObject { ["useCase"] = c.UseCase, ["name"] = c.Name, ["before"] = c.Before ?? "", ["intents"] = intents };
        if (!string.IsNullOrEmpty(instructions)) o["instructions"] = instructions;
        return JsJson.Stringify(o, 2);
    }

    public static LlmRequest RecombineRequest(Proposal p, string useCase, string? instructions = null) =>
        new("extract", LlmPrompts.RecombineSystem, RecombineUserPrompt(p, useCase, instructions), (JsonObject)JsonNode.Parse(LlmPrompts.RecombineSchemaJson)!);

    /// <summary>Asks the LLM to rewrite a use case's description from its remaining intents.</summary>
    public static async Task<(string Name, string After)> RecombineAsync(AskJson ask, Proposal p, string useCase, string? instructions = null, CancellationToken cancellationToken = default)
    {
        var a = await ask(RecombineRequest(p, useCase, instructions), cancellationToken).ConfigureAwait(false);
        return (S(a?["name"]), S(a?["after"]));
    }

    /// <summary>
    /// The user turn of the derive step: models, subjects, the approved intents, the use cases they touch with
    /// their proofs, the decisions and what is not promised.
    /// </summary>
    public static string DeriveProofsUserPrompt(Proposal p, IReadOnlyList<UseCase> useCases, ProofSetup setup)
    {
        var forward = ForwardIntents(p);
        var touched = forward.Select(i => i.UseCase).ToHashSet(StringComparer.Ordinal);
        var approved = new JsonArray();
        foreach (var i in forward)
        {
            var o = new JsonObject { ["id"] = i.Id, ["change"] = i.Change, ["useCase"] = i.UseCase, ["description"] = i.Description, ["provability"] = i.Provability };
            if (i.BeyondProofReason is not null) o["beyondProofReason"] = i.BeyondProofReason;
            o["replaces"] = LastroJson.ToNode(i.Replaces);
            approved.Add(o);
        }
        var cases = new JsonArray([.. p.UseCases.Where(c => touched.Contains(c.UseCase)).Select(c => (JsonNode)new JsonObject
        {
            ["id"] = c.UseCase, ["change"] = c.Change, ["name"] = c.Name, ["before"] = c.Before ?? "", ["after"] = c.After ?? "",
            ["proofs"] = ProofView(useCases.FirstOrDefault(u => u.Id == c.UseCase)),
        })]);
        var decisions = new JsonArray();
        foreach (var d in p.Decisions)
        {
            var o = new JsonObject { ["question"] = d.Question };
            var k = d.Chosen ?? d.Recommended;
            if (k >= 0 && k < d.Options.Count) o["answer"] = d.Options[k].Label;
            decisions.Add(o);
        }
        var models = new JsonObject();
        foreach (var (name, model) in setup.Models) models[name] = LastroJson.ToNode(model);
        return JsJson.Stringify(new JsonObject
        {
            ["models"] = models, ["subjects"] = new JsonArray([.. setup.Subjects.Select(s => (JsonNode)JsonValue.Create(s)!)]), ["approvedIntents"] = approved,
            ["useCases"] = cases, ["decisions"] = decisions, ["notPromised"] = new JsonArray([.. p.NotPromised.Select(s => (JsonNode)JsonValue.Create(s)!)]),
        }, 2);
    }

    public static LlmRequest DeriveProofsRequest(Proposal p, IReadOnlyList<UseCase> useCases, ProofSetup setup) =>
        new("derive", LlmPrompts.DeriveProofsSystem, DeriveProofsUserPrompt(p, useCases, setup), (JsonObject)JsonNode.Parse(LlmPrompts.DeriveProofsSchemaJson)!);

    /// <summary>Asks the LLM for the proofs of the approved intents and checks its answer.</summary>
    public static async Task<Proposal> DeriveAsync(AskJson ask, Proposal p, IReadOnlyList<UseCase> useCases, ProofSetup setup, VerifyOptions? opts = null, CancellationToken cancellationToken = default) =>
        FromDerivation(p, useCases, setup, await ask(DeriveProofsRequest(p, useCases, setup), cancellationToken).ConfigureAwait(false), opts);
}
