//! # lastro-core
//!
//! The proof core of Lastro. It takes a proof plan (model + LSL clauses, as
//! derived from human-reviewed intents) and the implementation's input→output
//! table, and decides, per clause:
//!
//! - `proved`: the table covers the entire declared (finite) input space and
//!   the clause holds on every row. This is a proof by exhaustion over the model.
//! - `violated`: a counterexample row.
//! - `vacuous`: no row makes the clause's if-part true.
//! - `weak`: holds, but rejects none of the planted mutants (vibe verification).
//! - `evidenced`: holds on every row, but the table does not cover the space.
//!
//! It also checks that the clauses are jointly satisfiable (spec consistency).
//! The crate builds natively (`lastro-prove` CLI) and to `wasm32-unknown-unknown`
//! with a tiny JSON-in/JSON-out ABI (see [`wasm`]).

pub mod domain;
pub mod lsl;
pub mod prove;
pub mod wasm;

pub use prove::{prove, ProofReport, ProveRequest};

/// JSON in, JSON out. Errors are returned as `{"error": "..."}`.
pub fn prove_json(request: &str) -> String {
    match serde_json::from_str::<ProveRequest>(request) {
        Ok(req) => serde_json::to_string(&prove(&req)).unwrap_or_else(|e| error_json(&e.to_string())),
        Err(e) => error_json(&format!("invalid request: {e}")),
    }
}

fn error_json(msg: &str) -> String {
    serde_json::json!({ "error": msg }).to_string()
}
