//! LSL (Lastro Spec Language): a JSON expression AST shared with the TypeScript SDK.
//!
//! Variables are namespaced `in.<field>` / `out.<field>`; inside a def body,
//! parameters are referenced by bare name.

use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
use std::fmt;

/// A runtime value. Numbers are f64 to match JSON / JavaScript semantics.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(untagged)]
pub enum Value {
    Bool(bool),
    Num(f64),
    Str(String),
}

impl fmt::Display for Value {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Value::Bool(b) => write!(f, "{b}"),
            Value::Num(n) => write!(f, "{n}"),
            Value::Str(s) => write!(f, "{s:?}"),
        }
    }
}

pub type Env = BTreeMap<String, Value>;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Op {
    And,
    Or,
    Not,
    Implies,
    Iff,
    Eq,
    Neq,
    Lt,
    Le,
    Gt,
    Ge,
    Add,
    Sub,
    Mul,
    Ite,
    In,
}

impl Op {
    fn arity(self) -> (usize, usize) {
        use Op::*;
        match self {
            And | Or | Add | Mul | In => (if self == In { 2 } else { 1 }, usize::MAX),
            Not => (1, 1),
            Ite => (3, 3),
            _ => (2, 2),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(untagged)]
pub enum Expr {
    Var {
        var: String,
    },
    Const {
        #[serde(rename = "const")]
        value: Value,
    },
    Op {
        op: Op,
        args: Vec<Expr>,
    },
    Call {
        call: String,
        args: Vec<Expr>,
    },
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Def {
    pub params: Vec<String>,
    pub body: Expr,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
}

pub type Defs = BTreeMap<String, Def>;

#[derive(Debug, Clone, PartialEq)]
pub struct LslError(pub String);

impl fmt::Display for LslError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "LSL: {}", self.0)
    }
}

impl std::error::Error for LslError {}

pub struct Ctx<'a> {
    pub input: &'a Env,
    pub output: &'a Env,
    pub defs: &'a Defs,
}

fn err<T>(msg: impl Into<String>) -> Result<T, LslError> {
    Err(LslError(msg.into()))
}

fn as_bool(v: Value) -> Result<bool, LslError> {
    match v {
        Value::Bool(b) => Ok(b),
        other => err(format!("expected boolean, got {other}")),
    }
}

fn as_num(v: Value) -> Result<f64, LslError> {
    match v {
        Value::Num(n) => Ok(n),
        other => err(format!("expected number, got {other}")),
    }
}

pub fn eval(e: &Expr, ctx: &Ctx, locals: &Env, depth: usize) -> Result<Value, LslError> {
    if depth > 64 {
        return err("max recursion depth exceeded (recursive def?)");
    }
    match e {
        Expr::Const { value } => Ok(value.clone()),
        Expr::Var { var } => lookup(var, ctx, locals),
        Expr::Call { call, args } => {
            let def = ctx.defs.get(call).ok_or_else(|| LslError(format!("unknown def '{call}'")))?;
            if def.params.len() != args.len() {
                return err(format!("def '{call}' expects {} args, got {}", def.params.len(), args.len()));
            }
            let mut bound = Env::new();
            for (p, a) in def.params.iter().zip(args) {
                bound.insert(p.clone(), eval(a, ctx, locals, depth + 1)?);
            }
            eval(&def.body, ctx, &bound, depth + 1)
        }
        Expr::Op { op, args } => {
            let (min, max) = op.arity();
            if args.len() < min || args.len() > max {
                return err(format!("op {op:?} with {} args", args.len()));
            }
            let ev = |x: &Expr| eval(x, ctx, locals, depth + 1);
            Ok(match op {
                Op::And => {
                    for a in args {
                        if !as_bool(ev(a)?)? {
                            return Ok(Value::Bool(false));
                        }
                    }
                    Value::Bool(true)
                }
                Op::Or => {
                    for a in args {
                        if as_bool(ev(a)?)? {
                            return Ok(Value::Bool(true));
                        }
                    }
                    Value::Bool(false)
                }
                Op::Not => Value::Bool(!as_bool(ev(&args[0])?)?),
                Op::Implies => Value::Bool(!as_bool(ev(&args[0])?)? || as_bool(ev(&args[1])?)?),
                Op::Iff => Value::Bool(as_bool(ev(&args[0])?)? == as_bool(ev(&args[1])?)?),
                Op::Eq => Value::Bool(ev(&args[0])? == ev(&args[1])?),
                Op::Neq => Value::Bool(ev(&args[0])? != ev(&args[1])?),
                Op::Lt => Value::Bool(as_num(ev(&args[0])?)? < as_num(ev(&args[1])?)?),
                Op::Le => Value::Bool(as_num(ev(&args[0])?)? <= as_num(ev(&args[1])?)?),
                Op::Gt => Value::Bool(as_num(ev(&args[0])?)? > as_num(ev(&args[1])?)?),
                Op::Ge => Value::Bool(as_num(ev(&args[0])?)? >= as_num(ev(&args[1])?)?),
                Op::Add => {
                    let mut s = 0.0;
                    for a in args {
                        s += as_num(ev(a)?)?;
                    }
                    Value::Num(s)
                }
                Op::Mul => {
                    let mut s = 1.0;
                    for a in args {
                        s *= as_num(ev(a)?)?;
                    }
                    Value::Num(s)
                }
                Op::Sub => Value::Num(as_num(ev(&args[0])?)? - as_num(ev(&args[1])?)?),
                Op::Ite => {
                    if as_bool(ev(&args[0])?)? {
                        ev(&args[1])?
                    } else {
                        ev(&args[2])?
                    }
                }
                Op::In => {
                    let v = ev(&args[0])?;
                    let mut found = false;
                    for a in &args[1..] {
                        if ev(a)? == v {
                            found = true;
                            break;
                        }
                    }
                    Value::Bool(found)
                }
            })
        }
    }
}

pub fn holds(e: &Expr, ctx: &Ctx) -> Result<bool, LslError> {
    as_bool(eval(e, ctx, &Env::new(), 0)?)
}

fn lookup(name: &str, ctx: &Ctx, locals: &Env) -> Result<Value, LslError> {
    if let Some(v) = locals.get(name) {
        return Ok(v.clone());
    }
    let (ns, field) = name.split_once('.').ok_or_else(|| LslError(format!("unbound variable '{name}'")))?;
    let src = match ns {
        "in" => ctx.input,
        "out" => ctx.output,
        _ => return err(format!("unbound variable '{name}'")),
    };
    src.get(field).cloned().ok_or_else(|| LslError(format!("unbound variable '{name}'")))
}

/// Human-readable rendering, identical to the TypeScript `show()`.
pub fn show(e: &Expr) -> String {
    match e {
        Expr::Const { value } => match value {
            Value::Str(s) => format!("{s:?}"),
            v => v.to_string(),
        },
        Expr::Var { var } => var.clone(),
        Expr::Call { call, args } => format!("{call}({})", args.iter().map(show).collect::<Vec<_>>().join(", ")),
        Expr::Op { op, args } => {
            let wrap = |x: &Expr| match x {
                Expr::Op { op: Op::Not, .. } => show(x),
                Expr::Op { .. } => format!("({})", show(x)),
                _ => show(x),
            };
            let sym = match op {
                Op::And => "∧",
                Op::Or => "∨",
                Op::Implies => "⇒",
                Op::Iff => "⇔",
                Op::Eq => "=",
                Op::Neq => "≠",
                Op::Lt => "<",
                Op::Le => "≤",
                Op::Gt => ">",
                Op::Ge => "≥",
                Op::Add => "+",
                Op::Sub => "-",
                Op::Mul => "·",
                Op::Not => return format!("¬{}", wrap(&args[0])),
                Op::Ite => return format!("if {} then {} else {}", show(&args[0]), show(&args[1]), show(&args[2])),
                Op::In => {
                    return format!("{} ∈ {{{}}}", show(&args[0]), args[1..].iter().map(show).collect::<Vec<_>>().join(", "))
                }
            };
            args.iter().map(wrap).collect::<Vec<_>>().join(&format!(" {sym} "))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse(s: &str) -> Expr {
        serde_json::from_str(s).unwrap()
    }

    #[test]
    fn parses_and_evaluates_the_ts_wire_format() {
        let e = parse(r#"{"op":"implies","args":[{"var":"in.x"},{"op":"eq","args":[{"var":"out.y"},{"const":2}]}]}"#);
        let input: Env = [("x".into(), Value::Bool(true))].into();
        let output: Env = [("y".into(), Value::Num(2.0))].into();
        let defs = Defs::new();
        assert!(holds(&e, &Ctx { input: &input, output: &output, defs: &defs }).unwrap());
        assert_eq!(show(&e), "in.x ⇒ (out.y = 2)");
    }

    #[test]
    fn defs_bind_parameters() {
        let mut defs = Defs::new();
        defs.insert("both".into(), Def { params: vec!["a".into(), "b".into()], body: parse(r#"{"op":"and","args":[{"var":"a"},{"var":"b"}]}"#), description: None });
        let e = parse(r#"{"call":"both","args":[{"const":true},{"var":"in.x"}]}"#);
        let input: Env = [("x".into(), Value::Bool(false))].into();
        let out = Env::new();
        assert!(!holds(&e, &Ctx { input: &input, output: &out, defs: &defs }).unwrap());
    }

    #[test]
    fn type_errors_and_unbound_vars_are_reported() {
        let defs = Defs::new();
        let env = Env::new();
        let ctx = Ctx { input: &env, output: &env, defs: &defs };
        assert!(holds(&parse(r#"{"var":"in.nope"}"#), &ctx).is_err());
        assert!(holds(&parse(r#"{"op":"not","args":[{"const":1}]}"#), &ctx).is_err());
    }
}
