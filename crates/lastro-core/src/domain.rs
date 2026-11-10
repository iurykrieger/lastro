//! Value domains. A model whose inputs are all finite can be enumerated, which
//! turns "checked on every row" into a proof over the declared domain.

use crate::lsl::{Env, Value};
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum Domain {
    Bool,
    Enum { values: Vec<Value> },
    Int {
        #[serde(default)]
        min: Option<i64>,
        #[serde(default)]
        max: Option<i64>,
    },
    String {
        #[serde(default)]
        examples: Option<Vec<String>>,
    },
}

impl Domain {
    /// Number of values, or None when the domain is infinite.
    pub fn size(&self) -> Option<u64> {
        match self {
            Domain::Bool => Some(2),
            Domain::Enum { values } => Some(values.len() as u64),
            Domain::Int { min: Some(a), max: Some(b) } => Some(if b >= a { (b - a + 1) as u64 } else { 0 }),
            _ => None,
        }
    }

    pub fn values(&self) -> Option<Vec<Value>> {
        match self {
            Domain::Bool => Some(vec![Value::Bool(false), Value::Bool(true)]),
            Domain::Enum { values } => Some(values.clone()),
            Domain::Int { min: Some(a), max: Some(b) } if b - a <= 1_000_000 => Some((*a..=*b).map(|i| Value::Num(i as f64)).collect()),
            _ => None,
        }
    }

    pub fn contains(&self, v: &Value) -> bool {
        match (self, v) {
            (Domain::Bool, Value::Bool(_)) => true,
            (Domain::Enum { values }, v) => values.contains(v),
            (Domain::Int { min, max }, Value::Num(n)) => {
                n.fract() == 0.0 && min.is_none_or(|m| *n >= m as f64) && max.is_none_or(|m| *n <= m as f64)
            }
            (Domain::String { .. }, Value::Str(_)) => true,
            _ => false,
        }
    }
}

pub type Table = BTreeMap<String, Domain>;

/// Size of the cartesian product, or None if any domain is infinite (or it overflows).
pub fn space_size(t: &Table) -> Option<u64> {
    t.values().try_fold(1u64, |acc, d| d.size().and_then(|s| acc.checked_mul(s)))
}

/// Deterministic enumeration of the whole space (keys in sorted order).
pub fn enumerate(t: &Table) -> Option<Vec<Env>> {
    let keys: Vec<&String> = t.keys().collect();
    let values: Vec<Vec<Value>> = t.values().map(|d| d.values()).collect::<Option<_>>()?;
    let mut out = vec![Env::new()];
    for (k, vs) in keys.iter().zip(values) {
        let mut next = Vec::with_capacity(out.len() * vs.len());
        for env in &out {
            for v in &vs {
                let mut e = env.clone();
                e.insert((*k).clone(), v.clone());
                next.push(e);
            }
        }
        out = next;
    }
    Some(out)
}

/// Canonical key of an environment restricted to the table's fields.
pub fn key_of(env: &Env, t: &Table) -> String {
    t.keys().map(|k| format!("{k}={}", env.get(k).map(|v| v.to_string()).unwrap_or_default())).collect::<Vec<_>>().join("|")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn enumerates_the_full_product() {
        let mut t = Table::new();
        t.insert("a".into(), Domain::Bool);
        t.insert("b".into(), Domain::Enum { values: vec![Value::Str("x".into()), Value::Str("y".into()), Value::Str("z".into())] });
        assert_eq!(space_size(&t), Some(6));
        let all = enumerate(&t).unwrap();
        assert_eq!(all.len(), 6);
        let keys: std::collections::BTreeSet<_> = all.iter().map(|e| key_of(e, &t)).collect();
        assert_eq!(keys.len(), 6);
    }

    #[test]
    fn infinite_domains_are_not_enumerable() {
        let mut t = Table::new();
        t.insert("s".into(), Domain::String { examples: None });
        assert_eq!(space_size(&t), None);
        assert!(enumerate(&t).is_none());
    }

    #[test]
    fn membership() {
        let d = Domain::Int { min: Some(0), max: Some(9) };
        assert!(d.contains(&Value::Num(3.0)));
        assert!(!d.contains(&Value::Num(3.5)));
        assert!(!d.contains(&Value::Num(10.0)));
    }
}
