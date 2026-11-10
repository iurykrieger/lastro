use lastro_core::prove::{prove, ProveRequest, Verdict};
use serde_json::{json, Value};

/// Model: x in 0..=9 -> y in 0..=40. Spec: y = 2x, and y >= x.
fn request(table: Vec<(i64, i64)>, extra_clauses: Vec<Value>) -> ProveRequest {
    let mut clauses = vec![
        json!({"id":"C1","intentIds":["I1"],"description":"y doubles x","ensures":{"op":"eq","args":[{"var":"out.y"},{"op":"mul","args":[{"const":2},{"var":"in.x"}]}]}}),
        json!({"id":"C2","intentIds":["I1"],"description":"y never below x","ensures":{"op":"ge","args":[{"var":"out.y"},{"var":"in.x"}]}}),
    ];
    clauses.extend(extra_clauses);
    serde_json::from_value(json!({
        "model": {
            "name": "double",
            "inputs": {"x": {"type":"int","min":0,"max":9}},
            "outputs": {"y": {"type":"int","min":0,"max":40}}
        },
        "clauses": clauses,
        "table": table.into_iter().map(|(x, y)| json!({"input":{"x":x},"output":{"y":y}})).collect::<Vec<_>>()
    }))
    .unwrap()
}

fn verdict(r: &lastro_core::ProofReport, id: &str) -> Verdict {
    r.clauses.iter().find(|c| c.clause_id == id).unwrap().verdict
}

#[test]
fn complete_correct_table_is_proved() {
    let r = prove(&request((0..=9).map(|x| (x, 2 * x)).collect(), vec![]));
    assert!(r.coverage.complete);
    assert_eq!(verdict(&r, "C1"), Verdict::Proved);
    assert_eq!(verdict(&r, "C2"), Verdict::Proved);
    assert!(r.ok);
    assert!(r.table_digest.starts_with("sha256:"));
}

#[test]
fn a_bug_yields_a_counterexample() {
    let r = prove(&request((0..=9).map(|x| (x, if x == 7 { 15 } else { 2 * x })).collect(), vec![]));
    let c1 = r.clauses.iter().find(|c| c.clause_id == "C1").unwrap();
    assert_eq!(c1.verdict, Verdict::Violated);
    assert_eq!(c1.evidence, "formal");
    assert_eq!(c1.counterexample.as_ref().unwrap().input["x"], lastro_core::lsl::Value::Num(7.0));
    assert!(!r.ok);
}

#[test]
fn the_prover_does_not_trust_an_incomplete_table() {
    let r = prove(&request((0..=5).map(|x| (x, 2 * x)).collect(), vec![]));
    assert!(!r.coverage.complete);
    assert_eq!(r.coverage.missing, 4);
    assert_eq!(verdict(&r, "C1"), Verdict::Evidenced);
    assert!(!r.ok);
}

#[test]
fn nondeterministic_and_out_of_domain_rows_break_completeness() {
    let mut t: Vec<(i64, i64)> = (0..=9).map(|x| (x, 2 * x)).collect();
    t.push((3, 7));
    t.push((4, 99));
    let r = prove(&request(t, vec![]));
    assert_eq!(r.coverage.nondeterministic, 1);
    assert_eq!(r.coverage.out_of_domain, 1);
    assert!(!r.coverage.complete);
}

#[test]
fn tautologies_are_weak_and_unreachable_clauses_are_vacuous() {
    let r = prove(&request(
        (0..=9).map(|x| (x, 2 * x)).collect(),
        vec![
            json!({"id":"C3","ensures":{"op":"or","args":[{"op":"ge","args":[{"var":"out.y"},{"const":0}]},{"op":"lt","args":[{"var":"out.y"},{"const":0}]}]}}),
            json!({"id":"C4","requires":{"op":"gt","args":[{"var":"in.x"},{"const":100}]},"ensures":{"op":"eq","args":[{"var":"out.y"},{"const":0}]}}),
        ],
    ));
    assert_eq!(verdict(&r, "C3"), Verdict::Weak);
    assert_eq!(verdict(&r, "C4"), Verdict::Vacuous);
}

#[test]
fn contradictory_clauses_are_found_without_any_implementation() {
    let r = prove(&request(
        vec![],
        vec![json!({"id":"C3","requires":{"op":"eq","args":[{"var":"in.x"},{"const":3}]},"ensures":{"op":"eq","args":[{"var":"out.y"},{"const":5}]}})],
    ));
    assert_eq!(r.contradictions.len(), 1);
    assert_eq!(r.contradictions[0].clauses, vec!["C1", "C2", "C3"]);
}

#[test]
fn json_abi_reports_errors() {
    let out = lastro_core::prove_json("{not json");
    assert!(out.contains("\"error\""));
}
