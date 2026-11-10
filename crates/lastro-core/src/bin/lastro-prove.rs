//! `lastro-prove < request.json > report.json`
//! Exit code: 0 when every clause is proved and the spec is consistent, 1 otherwise, 2 on bad input.

use std::io::Read;

fn main() {
    let mut input = String::new();
    if std::io::stdin().read_to_string(&mut input).is_err() {
        eprintln!("could not read stdin");
        std::process::exit(2);
    }
    let out = lastro_core::prove_json(&input);
    println!("{out}");
    let v: serde_json::Value = serde_json::from_str(&out).unwrap_or_default();
    std::process::exit(if v.get("error").is_some() { 2 } else if v["ok"] == true { 0 } else { 1 });
}
