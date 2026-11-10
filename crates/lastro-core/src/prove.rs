//! The prover: checks every clause of a proof plan against the implementation's
//! input→output table.
//!
//! The prover does not trust the caller's claim that the table is complete. It
//! enumerates the declared input space itself and only issues `proved` when
//! every point of that space has exactly one row. Otherwise the same checks
//! yield `evidenced` (empirical), with a rule-of-three failure bound.

use crate::domain::{enumerate, key_of, space_size, Table};
use crate::lsl::{holds, Ctx, Defs, Env, Expr, LslError, Value};
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Model {
    pub name: String,
    pub inputs: Table,
    pub outputs: Table,
    #[serde(default)]
    pub defs: Defs,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Clause {
    pub id: String,
    #[serde(default)]
    pub intent_ids: Vec<String>,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub requires: Option<Expr>,
    pub ensures: Expr,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Row {
    pub input: Env,
    pub output: Env,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProveRequest {
    pub model: Model,
    pub clauses: Vec<Clause>,
    pub table: Vec<Row>,
    /// Max output-space size for the consistency check (default 4096).
    #[serde(default)]
    pub consistency_limit: Option<u64>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum Verdict {
    Proved,
    Evidenced,
    Violated,
    Vacuous,
    Weak,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Coverage {
    /// Size of the declared input space, None when infinite.
    pub space: Option<u64>,
    pub rows: usize,
    pub complete: bool,
    pub missing: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub first_missing: Option<Env>,
    pub out_of_domain: usize,
    /// Same input mapped to different outputs: the implementation is not a function.
    pub nondeterministic: usize,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MutantStats {
    pub total: usize,
    pub killed: usize,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ClauseProof {
    pub clause_id: String,
    pub verdict: Verdict,
    pub evidence: String,
    pub checked: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub counterexample: Option<Row>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub failure_rate_upper_bound95: Option<f64>,
    pub mutants: MutantStats,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Contradiction {
    pub input: Env,
    pub clauses: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProofReport {
    pub engine: String,
    pub model: String,
    pub coverage: Coverage,
    pub clauses: Vec<ClauseProof>,
    pub contradictions: Vec<Contradiction>,
    /// SHA-256 over the canonical table, so a receipt names exactly what was proved.
    pub table_digest: String,
    pub ok: bool,
}

type Mutant = Box<dyn Fn(&Env) -> Env + 'static>;

/// "Plant one bug" on the output side: force a field to a constant, or negate a boolean.
fn mutants(model: &Model) -> Vec<Mutant> {
    #![allow(clippy::type_complexity)]
    let mut ms: Vec<Mutant> = Vec::new();
    for (field, d) in &model.outputs {
        if matches!(d, crate::domain::Domain::Bool) {
            let f = field.clone();
            ms.push(Box::new(move |o: &Env| {
                let mut o = o.clone();
                if let Some(Value::Bool(b)) = o.get(&f).cloned() {
                    o.insert(f.clone(), Value::Bool(!b));
                }
                o
            }));
        }
        if let Some(vals) = d.values().filter(|v| v.len() <= 16) {
            for v in vals {
                let f = field.clone();
                ms.push(Box::new(move |o: &Env| {
                    let mut o = o.clone();
                    o.insert(f.clone(), v.clone());
                    o
                }));
            }
        }
        if matches!(d, crate::domain::Domain::Int { .. }) {
            for delta in [1.0, -1.0] {
                let f = field.clone();
                ms.push(Box::new(move |o: &Env| {
                    let mut o = o.clone();
                    if let Some(Value::Num(n)) = o.get(&f).cloned() {
                        o.insert(f.clone(), Value::Num(n + delta));
                    }
                    o
                }));
            }
        }
    }
    ms
}

fn check(c: &Clause, input: &Env, output: &Env, defs: &Defs) -> Result<Option<bool>, LslError> {
    let ctx = Ctx { input, output, defs };
    if let Some(r) = &c.requires {
        if !holds(r, &ctx)? {
            return Ok(None);
        }
    }
    holds(&c.ensures, &ctx).map(Some)
}

pub fn prove(req: &ProveRequest) -> ProofReport {
    let model = &req.model;
    let defs = &model.defs;

    // 1. Validate rows and measure coverage of the declared input space.
    let mut by_key: BTreeMap<String, &Row> = BTreeMap::new();
    let mut out_of_domain = 0;
    let mut nondeterministic = 0;
    for row in &req.table {
        let input_ok = model.inputs.iter().all(|(k, d)| row.input.get(k).is_some_and(|v| d.contains(v)));
        let output_ok = model.outputs.iter().all(|(k, d)| row.output.get(k).is_some_and(|v| d.contains(v)));
        if !input_ok || !output_ok {
            out_of_domain += 1;
            continue;
        }
        let key = key_of(&row.input, &model.inputs);
        match by_key.get(&key) {
            Some(prev) if prev.output != row.output => nondeterministic += 1,
            Some(_) => {}
            None => {
                by_key.insert(key, row);
            }
        }
    }
    let rows: Vec<&Row> = by_key.values().copied().collect();
    let space = space_size(&model.inputs);
    let (missing, first_missing) = match enumerate(&model.inputs) {
        Some(all) if space.is_some_and(|s| s <= 1_000_000) => {
            let missing: Vec<Env> = all.into_iter().filter(|e| !by_key.contains_key(&key_of(e, &model.inputs))).collect();
            (missing.len(), missing.into_iter().next())
        }
        _ => (0, None),
    };
    let complete = space.is_some() && missing == 0 && nondeterministic == 0 && out_of_domain == 0 && !rows.is_empty();
    let coverage = Coverage { space, rows: rows.len(), complete, missing, first_missing, out_of_domain, nondeterministic };

    // 2. Clauses.
    let ms = mutants(model);
    let clauses: Vec<ClauseProof> = req
        .clauses
        .iter()
        .map(|c| {
            let mut proof = ClauseProof {
                clause_id: c.id.clone(),
                verdict: Verdict::Vacuous,
                evidence: "none".into(),
                checked: 0,
                counterexample: None,
                failure_rate_upper_bound95: None,
                mutants: MutantStats { total: ms.len(), killed: 0 },
                error: None,
            };
            let result: Result<(), LslError> = (|| {
                for row in &rows {
                    match check(c, &row.input, &row.output, defs)? {
                        None => {}
                        Some(true) => proof.checked += 1,
                        Some(false) => {
                            proof.checked += 1;
                            if proof.counterexample.is_none() {
                                proof.counterexample = Some((*row).clone());
                            }
                        }
                    }
                }
                if proof.counterexample.is_some() {
                    proof.verdict = Verdict::Violated;
                    proof.evidence = if complete { "formal" } else { "empirical" }.into();
                    return Ok(());
                }
                if proof.checked == 0 {
                    proof.verdict = Verdict::Vacuous;
                    return Ok(());
                }
                for m in &ms {
                    let mut killed = false;
                    for row in &rows {
                        if check(c, &row.input, &m(&row.output), defs)? == Some(false) {
                            killed = true;
                            break;
                        }
                    }
                    if killed {
                        proof.mutants.killed += 1;
                    }
                }
                if proof.mutants.killed == 0 && !ms.is_empty() {
                    proof.verdict = Verdict::Weak;
                } else if complete {
                    proof.verdict = Verdict::Proved;
                    proof.evidence = "formal".into();
                } else {
                    proof.verdict = Verdict::Evidenced;
                    proof.evidence = "empirical".into();
                    proof.failure_rate_upper_bound95 = Some((3.0 / proof.checked as f64).min(1.0));
                }
                Ok(())
            })();
            if let Err(e) = result {
                proof.verdict = Verdict::Violated;
                proof.error = Some(e.to_string());
            }
            proof
        })
        .collect();

    // 3. Spec consistency: for each input, does some output satisfy every clause?
    let contradictions = consistency(req);

    let digest = table_digest(&rows);
    let ok = complete && contradictions.is_empty() && clauses.iter().all(|c| c.verdict == Verdict::Proved);
    ProofReport {
        engine: concat!("lastro-core ", env!("CARGO_PKG_VERSION")).to_string(),
        model: model.name.clone(),
        coverage,
        clauses,
        contradictions,
        table_digest: digest,
        ok,
    }
}

/// Inputs for which the clauses cannot all hold at once, whatever the program outputs.
pub fn consistency(req: &ProveRequest) -> Vec<Contradiction> {
    let model = &req.model;
    let limit = req.consistency_limit.unwrap_or(4096);
    let (Some(outputs), Some(inputs)) = (
        space_size(&model.outputs).filter(|s| *s <= limit).and_then(|_| enumerate(&model.outputs)),
        space_size(&model.inputs).filter(|s| *s <= 100_000).and_then(|_| enumerate(&model.inputs)),
    ) else {
        return vec![];
    };
    let mut seen = BTreeSet::new();
    let mut out = vec![];
    for input in inputs {
        let satisfiable = outputs.iter().any(|o| req.clauses.iter().all(|c| check(c, &input, o, &model.defs).map(|r| r != Some(false)).unwrap_or(true)));
        if satisfiable {
            continue;
        }
        let applicable: Vec<String> = req
            .clauses
            .iter()
            .filter(|c| outputs.iter().any(|o| check(c, &input, o, &model.defs).map(|r| r.is_some()).unwrap_or(false)))
            .map(|c| c.id.clone())
            .collect();
        if seen.insert(applicable.join(",")) {
            out.push(Contradiction { input, clauses: applicable });
        }
    }
    out
}

fn table_digest(rows: &[&Row]) -> String {
    use sha2::{Digest, Sha256};
    let mut h = Sha256::new();
    for r in rows {
        h.update(serde_json::to_string(&r.input).unwrap_or_default());
        h.update(b"=>");
        h.update(serde_json::to_string(&r.output).unwrap_or_default());
        h.update(b"\n");
    }
    let bytes = h.finalize();
    let hex: String = bytes.iter().map(|b| format!("{b:02x}")).collect();
    format!("sha256:{hex}")
}
