package lastro

import (
	"fmt"
	"sort"
	"strings"
)

// The receipt: what was proved, what was trusted, what is only evidenced, and
// where proof stops. Meant to be pasted into a pull request. Same Markdown as
// the TS SDK's renderReceipt.

var verdictLabel = map[string]string{
	"proved":                   "✅ Proved",
	"proved-under-assumptions": "🟢 Proved under assumptions",
	"evidenced":                "🟡 Evidenced (confidence)",
	"violated":                 "❌ Violated",
	"beyond-proof":             "⚪ Beyond proof",
	"gap":                      "🕳️ Gap",
}

func prefix(s string, n int) string {
	u := utf16Units(s)
	if len(u) <= n {
		return s
	}
	return fromUTF16(u[:n])
}

func showClause(c Clause, arrow string) string {
	if c.Requires != nil {
		return Show(*c.Requires) + arrow + Show(c.Ensures)
	}
	return Show(c.Ensures)
}

// RenderReceipt renders the report as Markdown. plan and rust are optional (nil).
func RenderReceipt(report VerificationReport, plan *ProofPlan, rust *RustProofReport) string {
	var L []string
	add := func(lines ...string) { L = append(L, lines...) }
	s := report.Summary
	add("# Lastro receipt — "+report.Model, "")
	result := "❌ Not accepted"
	if report.OK {
		result = "✅ OK"
	}
	add("**Result:** " + result + "  ")
	add(fmt.Sprintf("Proved: %d · Under assumptions: %d · Evidenced: %d · Beyond proof: %d · Violated: %d · Gaps: %d",
		s["proved"], s["proved-under-assumptions"], s["evidenced"], s["beyond-proof"], s["violated"], s["gap"]), "")
	add(fmt.Sprintf("Intents: `%s…` · Plan: `%s…`", prefix(report.ArtifactHash, 19), prefix(report.PlanHash, 19)))
	if rust != nil {
		agree := "verdicts agree"
		if !rustAgrees(report.Clauses, *rust) {
			agree = "⚠️ verdicts DISAGREE"
		}
		space := "∞"
		if rust.Coverage.Space != nil {
			space = jsNumber(*rust.Coverage.Space)
		}
		add(fmt.Sprintf("Independent check: `%s` · coverage %d/%s inputs · table `%s…` · %s", rust.Engine, rust.Coverage.Rows, space, prefix(rust.TableDigest, 19), agree))
	}
	add("")

	add("## Intents", "", "| Intent | Verdict | Evidence | Confidence | Backed by |", "|---|---|---|---|---|")
	for _, i := range report.Intents {
		refs := strings.Join(append(append(append([]string{}, i.Clauses...), i.Assumptions...), i.Scenarios...), ", ")
		if refs == "" {
			refs = "—"
		}
		conf := "—"
		if i.Verdict == "proved" || i.Verdict == "proved-under-assumptions" {
			conf = "proof"
		} else if i.Confidence > 0 {
			conf = fmt.Sprintf("%.1f%%", i.Confidence*100)
		}
		add(fmt.Sprintf("| **%s** %s | %s | %s | %s | %s |", i.IntentID, strings.ReplaceAll(i.Statement, "|", `\|`), verdictLabel[i.Verdict], strings.Join(i.Evidence, " + "), conf, refs))
	}
	noted := false
	for _, i := range report.Intents {
		if len(i.Notes) > 0 {
			if !noted {
				add("")
				noted = true
			}
			for _, n := range i.Notes {
				add(fmt.Sprintf("- **%s**: %s", i.IntentID, n))
			}
		}
	}

	add("", "## Formal clauses", "")
	for _, r := range report.Clauses {
		var c *Clause
		if plan != nil {
			for k := range plan.Clauses {
				if plan.Clauses[k].ID == r.ClauseID {
					c = &plan.Clauses[k]
				}
			}
		}
		how := "not run"
		switch r.Method {
		case "exhaustive":
			how = fmt.Sprintf("exhaustive over %d applicable inputs", r.Checked)
		case "sampling":
			b := 1.0
			if r.FailureRateUpperBound95 != nil {
				b = *r.FailureRateUpperBound95
			}
			how = fmt.Sprintf("%d applicable samples; failure rate ≤ %.2f%% (95%%)", r.Checked, b*100)
		case "traces":
			how = fmt.Sprintf("%d real traces", r.Checked)
		}
		mut := ""
		if r.Mutants != nil {
			mut = fmt.Sprintf(" · mutants killed %d/%d", r.Mutants.Killed, r.Mutants.Total)
		}
		add(fmt.Sprintf("- **%s** `%s` (%s) — %s%s", r.ClauseID, r.Verdict, r.Evidence, how, mut))
		if c != nil {
			add("  - "+c.Description, "  - `"+showClause(*c, " ⇒ ")+"`")
		}
		if r.Counterexample != nil {
			add(fmt.Sprintf("  - Counterexample: input `%s` → output `%s`", Stringify(r.Counterexample.Input), stringifyOutput(r.Counterexample.Output, plan)))
		}
		if r.TraceViolation != nil {
			ref := ""
			if r.TraceViolation.Ref != "" {
				ref = " (" + r.TraceViolation.Ref + ")"
			}
			add(fmt.Sprintf("  - Violating trace%s: `%s`", ref, Stringify(r.TraceViolation.Input)))
		}
		if r.Note != "" {
			add("  - Note: " + r.Note)
		}
	}

	if len(report.Scenarios) > 0 {
		add("", "## Scenarios (empirical evidence, not proof)", "")
		for _, r := range report.Scenarios {
			mark := "❌"
			if r.Status == "passed" {
				mark = "✅"
			}
			text := ""
			if plan != nil {
				for _, sc := range plan.Scenarios {
					if sc.ID == r.ScenarioID {
						text = fmt.Sprintf("Given %s; when %s; then %s", sc.Given, sc.When, sc.Then)
					}
				}
			}
			errText := ""
			if r.Error != "" {
				errText = " — " + r.Error
			}
			add(fmt.Sprintf("- **%s** %s %s%s", r.ScenarioID, mark, text, errText))
		}
	}

	if len(report.Assumptions) > 0 {
		add("", "## Trusted pieces", "")
		for _, a := range report.Assumptions {
			var tests []string
			for _, id := range a.CheckedBy {
				status := " (planned)"
				for _, r := range report.Scenarios {
					if r.ScenarioID == id {
						status = " ❌"
						if r.Status == "passed" {
							status = " ✅"
						}
					}
				}
				tests = append(tests, id+status)
			}
			checked := strings.Join(tests, ", ")
			if checked == "" {
				checked = "no test"
			}
			add(fmt.Sprintf("- **%s** `%s`: %s", a.ID, a.Piece, a.Contract))
			add("  - You accept: "+a.YouAccept, "  - If it's wrong: "+a.IfWrong, "  - Checked by: "+checked)
		}
	}
	if len(report.BeyondProof) > 0 {
		add("", "## Where proof stops", "")
		for _, b := range report.BeyondProof {
			add(fmt.Sprintf("- **%s** (%s): %s", b.IntentID, b.Reason, b.Note))
		}
	}
	if len(report.NotPromised) > 0 {
		add("", "## Not promised", "")
		for _, n := range report.NotPromised {
			add("- " + n)
		}
	}
	if len(report.Decisions) > 0 {
		add("", "## Pending decisions (spec contradictions)", "")
		for _, d := range report.Decisions {
			add(fmt.Sprintf("- **%s** %s", d.ID, d.Question))
		}
	}
	return strings.Join(L, "\n") + "\n"
}

// stringifyOutput prints an output with its keys in the model's declared order, as a subject
// returning {status, token, role} prints in JavaScript. Unknown keys follow, sorted.
func stringifyOutput(out Env, plan *ProofPlan) string {
	if plan == nil {
		return Stringify(out)
	}
	return stringifyOrdered(out, plan.Model.Outputs.Names())
}

func stringifyOrdered(e map[string]any, order []string) string {
	if e == nil {
		return "null"
	}
	var keys []string
	for _, k := range order {
		if _, ok := e[k]; ok {
			keys = append(keys, k)
		}
	}
	var rest []string
	for k := range e {
		if !contains(keys, k) {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	parts := make([]string, 0, len(e))
	for _, k := range append(keys, rest...) {
		parts = append(parts, jsString(k)+":"+Stringify(e[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}
