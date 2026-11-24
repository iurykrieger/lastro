//! State machines: an independent breadth-first exploration of every interleaving of the actors.
//!
//! The caller passes the machine model, its clauses and the code's step function as a table (each step input the
//! code was asked about, with the next state it returned, or null when the event is not enabled). The prover does not
//! trust the caller's exploration: it walks the state space itself, in the canonical order the TypeScript engine uses
//! (actors ascending, events by name, inputs in domain order), and needs a row for every step it takes. It reports the
//! states visited, the verdict of each clause and its shortest counterexample, so the engines can be compared.

use crate::domain::{enumerate, Domain, Table};
use crate::lsl::{holds, Ctx, Defs, Env, Expr, LslError, Value};
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, HashMap};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MachineModel {
    pub name: String,
    pub state: Table,
    #[serde(default)]
    pub local: Table,
    #[serde(default)]
    pub actors: Option<u32>,
    pub initial: Env,
    pub events: BTreeMap<String, Table>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "lowercase")]
pub enum MachineClause {
    Invariant { id: String, holds: Expr },
    Step {
        id: String,
        #[serde(default)]
        requires: Option<Expr>,
        ensures: Expr,
    },
    Reach { id: String, condition: Expr },
}

impl MachineClause {
    fn id(&self) -> &str {
        match self {
            MachineClause::Invariant { id, .. } | MachineClause::Step { id, .. } | MachineClause::Reach { id, .. } => id,
        }
    }
    fn kind(&self) -> &'static str {
        match self {
            MachineClause::Invariant { .. } => "invariant",
            MachineClause::Step { .. } => "step",
            MachineClause::Reach { .. } => "reach",
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StepRow {
    pub input: Env,
    pub output: Option<Env>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MachineRequest {
    pub machine: MachineModel,
    pub clauses: Vec<MachineClause>,
    pub table: Vec<StepRow>,
    /// Max reachable states visited (default 100,000).
    #[serde(default)]
    pub limit: Option<usize>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct MachineEvent {
    pub actor: u32,
    pub event: String,
    pub input: Env,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MachineClauseResult {
    pub clause_id: String,
    pub kind: String,
    /// `proved`, `violated`, `vacuous` or `not-run` (the exploration did not finish).
    pub verdict: String,
    pub evidence: String,
    pub checked: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub path: Option<Vec<MachineEvent>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub state: Option<Env>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub note: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MachineReport {
    pub engine: String,
    pub model: String,
    pub states: usize,
    pub transitions: usize,
    pub complete: bool,
    /// Steps the exploration needed that the table has no row for.
    pub missing: usize,
    pub clauses: Vec<MachineClauseResult>,
    pub errors: Vec<String>,
}

fn key(e: &Env) -> String {
    serde_json::to_string(e).unwrap_or_default()
}

fn show(e: &MachineEvent) -> String {
    let input = if e.input.is_empty() { String::new() } else { key(&e.input) };
    format!("{}{} by actor {}", e.event, input, e.actor)
}

struct Node {
    state: Env,
    parent: Option<usize>,
    via: Option<MachineEvent>,
}

fn path_to(nodes: &[Node], i: usize) -> Vec<MachineEvent> {
    let mut path = vec![];
    let mut k = Some(i);
    while let Some(j) = k {
        match &nodes[j].via {
            Some(e) => path.push(e.clone()),
            None => break,
        }
        k = nodes[j].parent;
    }
    path.reverse();
    path
}

/// Explore the machine over the step table, breadth first, and decide each clause.
pub fn explore(req: &MachineRequest) -> MachineReport {
    let m = &req.machine;
    let actors = m.actors.unwrap_or(1).max(1);
    let limit = req.limit.unwrap_or(100_000);
    let mut errors: Vec<String> = vec![];
    let finite = |t: &Table, at: &str, errors: &mut Vec<String>| {
        for (k, d) in t {
            if d.size().is_none() {
                errors.push(format!("{at}.{k}: the domain must be finite"));
            }
        }
    };
    finite(&m.state, "state", &mut errors);
    finite(&m.local, "local", &mut errors);
    for (e, t) in &m.events {
        finite(t, &format!("events.{e}"), &mut errors);
    }
    if m.events.is_empty() {
        errors.push("events: a machine needs at least one event".into());
    }
    let mut results: Vec<MachineClauseResult> = req
        .clauses
        .iter()
        .map(|c| MachineClauseResult { clause_id: c.id().into(), kind: c.kind().into(), verdict: "not-run".into(), evidence: "none".into(), checked: 0, path: None, state: None, note: None })
        .collect();
    if !errors.is_empty() {
        for r in &mut results {
            r.note = Some("the model or the clause is invalid".into());
        }
        return MachineReport { engine: "lastro-core".into(), model: m.name.clone(), states: 0, transitions: 0, complete: false, missing: 0, clauses: results, errors };
    }

    // Every event any actor can raise, in the canonical order.
    let mut events: Vec<MachineEvent> = vec![];
    for actor in 1..=actors {
        for (name, inputs) in &m.events {
            for input in enumerate(inputs).unwrap_or_default() {
                events.push(MachineEvent { actor, event: name.clone(), input });
            }
        }
    }
    let table: HashMap<String, Option<Env>> = req.table.iter().map(|r| (key(&r.input), r.output.clone())).collect();
    let step_input = |s: &Env, e: &MachineEvent| -> Env {
        let mut x = e.input.clone();
        x.insert("actor".into(), Value::Num(e.actor as f64));
        x.insert("event".into(), Value::Str(e.event.clone()));
        for k in m.state.keys() {
            x.insert(k.clone(), s[k].clone());
        }
        for v in m.local.keys() {
            x.insert(v.clone(), s[&format!("{v}_{}", e.actor)].clone());
        }
        x
    };
    let next_state = |s: &Env, e: &MachineEvent, out: &Env| -> Result<Env, String> {
        let mut n = s.clone();
        let check = |k: &str, d: &Domain, v: &Value| if d.contains(v) { Ok(()) } else { Err(format!("{k}={} is outside its domain after {}", serde_json::to_string(v).unwrap_or_default(), show(e))) };
        for (k, d) in &m.state {
            let v = out.get(k).unwrap_or(&s[k]).clone();
            check(k, d, &v)?;
            n.insert(k.clone(), v);
        }
        for (k, d) in &m.local {
            let own = format!("{k}_{}", e.actor);
            let v = out.get(k).unwrap_or(&s[&own]).clone();
            check(k, d, &v)?;
            n.insert(own, v);
        }
        Ok(n)
    };
    let step_output = |n: &Env, actor: u32| -> Env {
        let mut o = Env::new();
        for k in m.state.keys() {
            o.insert(k.clone(), n[k].clone());
        }
        for k in m.local.keys() {
            o.insert(k.clone(), n[&format!("{k}_{actor}")].clone());
        }
        o
    };
    let defs = Defs::new();
    let empty = Env::new();
    let mut applied: HashMap<String, usize> = HashMap::new();
    let mut missing = 0usize;

    let mut initial = Env::new();
    for k in m.state.keys() {
        initial.insert(k.clone(), m.initial.get(k).cloned().unwrap_or(Value::Bool(false)));
    }
    for a in 1..=actors {
        for v in m.local.keys() {
            initial.insert(format!("{v}_{a}"), m.initial.get(v).cloned().unwrap_or(Value::Bool(false)));
        }
    }
    let mut nodes = vec![Node { state: initial, parent: None, via: None }];
    let mut seen: HashMap<String, usize> = HashMap::new();
    seen.insert(key(&nodes[0].state), 0);

    fn settle(r: &mut MachineClauseResult, f: Result<bool, LslError>) -> bool {
        match f {
            Ok(b) => b,
            Err(e) => {
                if r.verdict == "not-run" {
                    r.verdict = "violated".into();
                    r.evidence = "none".into();
                    r.note = Some(format!("evaluation error: {}", e.0));
                }
                false
            }
        }
    }
    let on_state = |nodes: &[Node], i: usize, results: &mut Vec<MachineClauseResult>| {
        let s = &nodes[i].state;
        let ctx = Ctx { input: s, output: &empty, defs: &defs };
        for (c, r) in req.clauses.iter().zip(results.iter_mut()) {
            if r.verdict != "not-run" {
                continue;
            }
            match c {
                MachineClause::Invariant { holds: h, .. } => {
                    let bad = holds(h, &ctx).map(|b| !b);
                    if settle(r, bad) {
                        r.verdict = "violated".into();
                        r.evidence = "counterexample".into();
                        r.path = Some(path_to(nodes, i));
                        r.state = Some(s.clone());
                    }
                }
                MachineClause::Reach { condition, .. } => {
                    if settle(r, holds(condition, &ctx)) {
                        r.verdict = "proved".into();
                        r.evidence = "formal".into();
                        r.path = Some(path_to(nodes, i));
                        r.state = Some(s.clone());
                        r.note = Some("reached: the path is a witness".into());
                    }
                }
                MachineClause::Step { .. } => {}
            }
        }
    };
    let on_step = |nodes: &[Node], from: usize, e: &MachineEvent, next: &Env, results: &mut Vec<MachineClauseResult>, applied: &mut HashMap<String, usize>| {
        let input = step_input(&nodes[from].state, e);
        let output = step_output(next, e.actor);
        let ctx = Ctx { input: &input, output: &output, defs: &defs };
        for (c, r) in req.clauses.iter().zip(results.iter_mut()) {
            let MachineClause::Step { id, requires, ensures } = c else { continue };
            if r.verdict != "not-run" {
                continue;
            }
            let applies = match requires {
                None => true,
                Some(q) => settle(r, holds(q, &ctx)),
            };
            if !applies {
                continue;
            }
            *applied.entry(id.clone()).or_default() += 1;
            if settle(r, holds(ensures, &ctx).map(|b| !b)) {
                r.verdict = "violated".into();
                r.evidence = "counterexample".into();
                let mut p = path_to(nodes, from);
                p.push(e.clone());
                r.path = Some(p);
                r.state = Some(next.clone());
            }
        }
    };

    on_state(&nodes, 0, &mut results);
    let mut transitions = 0usize;
    let mut complete = true;
    let mut i = 0;
    'bfs: while i < nodes.len() {
        for e in &events {
            let x = step_input(&nodes[i].state, e);
            let out = match table.get(&key(&x)) {
                None => {
                    missing += 1;
                    let msg = format!("the table has no row for {} in state {}", show(e), key(&nodes[i].state));
                    if !errors.contains(&msg) && errors.len() < 10 {
                        errors.push(msg);
                    }
                    continue;
                }
                Some(None) => continue,
                Some(Some(out)) => out,
            };
            let n = match next_state(&nodes[i].state, e, out) {
                Ok(n) => n,
                Err(msg) => {
                    if !errors.contains(&msg) {
                        errors.push(msg);
                    }
                    continue;
                }
            };
            transitions += 1;
            on_step(&nodes, i, e, &n, &mut results, &mut applied);
            let k = key(&n);
            if seen.contains_key(&k) {
                continue;
            }
            if nodes.len() >= limit {
                complete = false;
                break 'bfs;
            }
            seen.insert(k, nodes.len());
            nodes.push(Node { state: n, parent: Some(i), via: Some(e.clone()) });
            on_state(&nodes, nodes.len() - 1, &mut results);
        }
        i += 1;
    }

    if complete {
        for (c, r) in req.clauses.iter().zip(results.iter_mut()) {
            r.checked = if matches!(c, MachineClause::Step { .. }) { applied.get(c.id()).copied().unwrap_or(0) } else { nodes.len() };
            if r.verdict != "not-run" {
                continue;
            }
            match c {
                MachineClause::Reach { .. } => {
                    r.verdict = "violated".into();
                    r.evidence = "formal".into();
                    r.note = Some(format!("no reachable state satisfies the condition ({} states visited)", nodes.len()));
                }
                MachineClause::Step { .. } if r.checked == 0 => {
                    r.verdict = "vacuous".into();
                    r.note = Some("no reachable step makes the if-part true: the clause proves nothing".into());
                }
                _ => {
                    r.verdict = "proved".into();
                    r.evidence = "formal".into();
                }
            }
        }
        if let Some(first) = errors.first().cloned() {
            for r in &mut results {
                if r.verdict == "proved" {
                    r.verdict = "violated".into();
                    r.evidence = "none".into();
                    r.note = Some(first.clone());
                }
            }
        }
    }
    MachineReport { engine: "lastro-core".into(), model: m.name.clone(), states: nodes.len(), transitions, complete, missing, clauses: results, errors }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn counter(lost_update: bool) -> MachineRequest {
        // Two actors increment a shared counter; with a lost update each reads, then writes the value it read plus one.
        let n = Domain::Int { min: Some(0), max: Some(2) };
        let pc = Domain::Enum { values: vec![Value::Str("idle".into()), Value::Str("read".into()), Value::Str("done".into())] };
        let t = Domain::Int { min: Some(0), max: Some(2) };
        let machine = MachineModel {
            name: "counter".into(),
            state: [("n".to_string(), n)].into_iter().collect(),
            local: [("pc".to_string(), pc), ("t".to_string(), t)].into_iter().collect(),
            actors: Some(2),
            initial: [("n".to_string(), Value::Num(0.0)), ("pc".to_string(), Value::Str("idle".into())), ("t".to_string(), Value::Num(0.0))].into_iter().collect(),
            events: [("read".to_string(), Table::new()), ("write".to_string(), Table::new())].into_iter().collect(),
        };
        let inv: Expr = serde_json::from_str(
            r#"{"op":"eq","args":[{"var":"in.n"},{"op":"add","args":[{"op":"ite","args":[{"op":"eq","args":[{"var":"in.pc_1"},{"const":"done"}]},{"const":1},{"const":0}]},{"op":"ite","args":[{"op":"eq","args":[{"var":"in.pc_2"},{"const":"done"}]},{"const":1},{"const":0}]}]}]}"#,
        )
        .unwrap();
        // The step function, tabulated over every state the exploration can reach (and more).
        let mut table = vec![];
        for nv in 0..=2 {
            for pc in ["idle", "read", "done"] {
                for tv in 0..=2 {
                    for ev in ["read", "write"] {
                        for actor in 1..=2 {
                            let mut input = Env::new();
                            input.insert("n".into(), Value::Num(nv as f64));
                            input.insert("pc".into(), Value::Str(pc.into()));
                            input.insert("t".into(), Value::Num(tv as f64));
                            input.insert("event".into(), Value::Str(ev.into()));
                            input.insert("actor".into(), Value::Num(actor as f64));
                            let mut out = Env::new();
                            let output = match (ev, pc) {
                                ("read", "idle") if !lost_update => {
                                    out.insert("n".into(), Value::Num(((nv + 1) as f64).min(2.0)));
                                    out.insert("pc".into(), Value::Str("done".into()));
                                    Some(out)
                                }
                                ("read", "idle") => {
                                    out.insert("t".into(), Value::Num(nv as f64));
                                    out.insert("pc".into(), Value::Str("read".into()));
                                    Some(out)
                                }
                                ("write", "read") if lost_update => {
                                    out.insert("n".into(), Value::Num(((tv + 1) as f64).min(2.0)));
                                    out.insert("pc".into(), Value::Str("done".into()));
                                    Some(out)
                                }
                                _ => None,
                            };
                            table.push(StepRow { input, output });
                        }
                    }
                }
            }
        }
        MachineRequest { machine, clauses: vec![MachineClause::Invariant { id: "C1".into(), holds: inv }], table, limit: None }
    }

    #[test]
    fn an_atomic_counter_is_proved_over_every_interleaving() {
        let r = explore(&counter(false));
        assert!(r.complete);
        assert_eq!(r.missing, 0);
        assert_eq!(r.clauses[0].verdict, "proved");
    }

    #[test]
    fn a_lost_update_has_a_shortest_counterexample() {
        let r = explore(&counter(true));
        assert_eq!(r.clauses[0].verdict, "violated");
        let path: Vec<String> = r.clauses[0].path.as_ref().unwrap().iter().map(|e| format!("{}@{}", e.event, e.actor)).collect();
        assert_eq!(path, ["read@1", "read@2", "write@1", "write@2"]);
    }

    #[test]
    fn a_step_the_table_does_not_have_is_reported() {
        let mut req = counter(false);
        req.table.truncate(10);
        let r = explore(&req);
        assert!(r.missing > 0);
        assert_ne!(r.clauses[0].verdict, "proved");
    }
}
