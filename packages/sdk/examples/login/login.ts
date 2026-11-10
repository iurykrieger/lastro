/**
 * Worked example: a store login (inspired by the PDD Kit write-up, rebuilt for Lastro).
 *
 * Everything an LLM extractor/deriver would produce is written by hand here,
 * so the example runs offline and deterministically. The implementation is
 * kept as source text so tools (like the sandbox) can show, edit and mutate it.
 */
import {
  D,
  L,
  approvedIntents,
  createArtifact,
  type Clause,
  type Env,
  type IntentArtifact,
  type Model,
  type ProofPlan,
} from "../../src/index.js";

export const PROSE = `In the store login, only the right password gets a token, and only administrator accounts get administrator tokens. Whoever sends an email signs in as a customer; the administrator signs in with a username. If both are sent, the email wins. Today's errors stay: a request with neither email nor username gets 400, and a database failure gets 500. Login must respond quickly and the error message must be clear to the user.`;

// ---------------------------------------------------------------------------
// 1. Extraction (what the AI proposed). Note I5: the AI misread the prose.
// ---------------------------------------------------------------------------

export const AI_I5 = "If email and username are sent together, the request is invalid (400).";
export const FIXED_I5 = "If both email and username are sent, the email wins: the account is looked up among customers.";

export function extractLogin(prose = PROSE): IntentArtifact {
  return createArtifact(
    prose,
    [
      { statement: "Only the right password for the named account gets a token.", kind: "invariant", provability: "formal", quote: "only the right password gets a token" },
      { statement: "The right password for the named account always gets a token, unless infrastructure fails (500).", kind: "behavior", provability: "formal", quote: "only the right password gets a token" },
      { statement: "Only administrator accounts (found by username) get a token with the admin role.", kind: "invariant", provability: "formal", quote: "only administrator accounts get administrator tokens" },
      { statement: "An email login always yields a token with the customer role.", kind: "behavior", provability: "formal", quote: "Whoever sends an email signs in as a customer" },
      // The AI got this one wrong: the prose says the email wins, not that the request is invalid.
      { statement: AI_I5, kind: "behavior", provability: "formal", quote: "If both are sent, the email wins." },
      { statement: "A request with neither email nor username gets 400 and no token.", kind: "preserve", provability: "formal", quote: "a request with neither email nor username gets 400" },
      { statement: "A database failure gets 500 and no token.", kind: "preserve", provability: "formal", quote: "a database failure gets 500" },
      { statement: "Login responds quickly.", kind: "non-functional", provability: "beyond-proof", beyondProofReason: "termination-timing", quote: "Login must respond quickly" },
      { statement: "The error message is clear to the user.", kind: "non-functional", provability: "beyond-proof", beyondProofReason: "human-judgment", quote: "the error message must be clear to the user" },
    ],
    [
      {
        intentIds: ["I1", "I2"],
        question: "Must the right password ALWAYS get a token, or is it enough that a wrong one never does?",
        options: [
          { label: "Both ways", consequence: "A login that refuses everyone is rejected by the proof." },
          { label: "Only refuse the wrong one", consequence: "A login that refuses everyone still passes the formal clauses." },
        ],
        recommended: 0,
      },
      {
        intentIds: ["I1", "I7"],
        question: "When bcrypt cannot read the stored hash, does login answer 500 (as today) or 401?",
        options: [
          { label: "500, as today", consequence: "Keeps current behavior and separates corrupt data from a wrong password." },
          { label: "401", consequence: "A corrupt hash becomes indistinguishable from a wrong password." },
        ],
        recommended: 0,
      },
    ],
    ["That login finishes at all (a hung query violates no clause).", "Rate limiting of attempts."],
  );
}

// ---------------------------------------------------------------------------
// 2. Model: the vocabulary of the proof. Trusted pieces enter as facts.
// ---------------------------------------------------------------------------

export const MODEL: Model = {
  name: "login_flow",
  inputs: {
    sends: D.enum("email", "username", "both", "neither"), // request fields
    accountFound: D.bool(), // T1: find_by_email / find_by_username
    password: D.enum("right", "wrong", "bad-hash"), // T2: check_password (bcrypt)
    infra: D.enum("ok", "db-fails", "signer-fails"), // T1 db errors, T3 signer
  },
  outputs: {
    status: D.enum(200, 400, 401, 500),
    token: D.bool(),
    role: D.enum("none", "customer", "admin"),
  },
  defs: {
    named: {
      params: [],
      description: "the request names an account that exists and the database answered",
      body: L.and(L.neq(L.in("sends"), "neither"), L.in("accountFound"), L.neq(L.in("infra"), "db-fails")),
    },
    byEmail: { params: [], description: "the email picks the table (email or both)", body: L.oneOf(L.in("sends"), "email", "both") },
  },
};

// ---------------------------------------------------------------------------
// 3. Derivation. Depends on what the reviewer approved/amended and answered.
// ---------------------------------------------------------------------------

export function deriveLoginPlan(artifact: IntentArtifact): ProofPlan {
  const ok = new Set(approvedIntents(artifact).map((i) => i.id));
  const has = (id: string) => ok.has(id);
  const choice = (id: string) => {
    const d = artifact.decisions.find((x) => x.id === id);
    return d?.chosen ?? d?.recommended ?? 0;
  };
  const i5 = artifact.intents.find((i) => i.id === "I5");
  const i5SaysEmailWins = !!i5 && /email wins|customer/i.test(i5.statement);

  const named = L.call("named");
  const sends = L.in("sends");
  const st = L.out("status");
  const tok = L.out("token");
  const role = L.out("role");
  const noToken = (code: number) => L.and(L.eq(st, code), L.not(tok), L.eq(role, "none"));

  const clauses: Clause[] = [];
  const add = (c: Omit<Clause, "id">, when = true) => {
    if (when && c.intentIds.every(has)) clauses.push({ ...c, id: `C${clauses.length + 1}` });
  };

  add({ intentIds: ["I6"], description: "Neither email nor username: 400 and no token.", requires: L.eq(sends, "neither"), ensures: noToken(400) });
  add({ intentIds: ["I1"], description: "A token only for the named account, with the right password, and status 200.", requires: tok, ensures: L.and(named, L.eq(L.in("password"), "right"), L.eq(L.in("infra"), "ok"), L.eq(st, 200)), assumes: ["T1", "T2"] });
  add(
    { intentIds: ["I2"], description: "The right password for the named account always gets a token, unless signing fails (500).", requires: L.and(named, L.eq(L.in("password"), "right"), L.eq(L.in("infra"), "ok")), ensures: L.and(L.eq(st, 200), tok), assumes: ["T1", "T2", "T3"] },
    choice("D1") === 0,
  );
  add({ intentIds: ["I1"], description: "A login naming an account nobody holds gets 401, unless the database fails (500).", requires: L.and(L.neq(sends, "neither"), L.not(L.in("accountFound")), L.neq(L.in("infra"), "db-fails")), ensures: noToken(401), assumes: ["T1"] });
  add({ intentIds: ["I1"], description: "A wrong password for a readable account gets 401.", requires: L.and(named, L.eq(L.in("password"), "wrong")), ensures: noToken(401), assumes: ["T2"] });
  add({ intentIds: ["I1"], description: choice("D2") === 0 ? "A hash bcrypt cannot read gets 500 and no token." : "A hash bcrypt cannot read gets 401 and no token.", requires: L.and(named, L.eq(L.in("password"), "bad-hash")), ensures: noToken(choice("D2") === 0 ? 500 : 401), assumes: ["T2"] });
  add({ intentIds: ["I7"], description: "A database failure gets 500 and no token.", requires: L.and(L.neq(sends, "neither"), L.eq(L.in("infra"), "db-fails")), ensures: noToken(500) });
  add({ intentIds: ["I3"], description: "The admin role only for an account found by username, with no email sent.", requires: L.eq(role, "admin"), ensures: L.and(tok, L.eq(sends, "username"), named) });
  add({ intentIds: ["I3"], description: "A token issued for a username carries the admin role.", requires: L.and(tok, L.eq(sends, "username")), ensures: L.eq(role, "admin") });
  add({ intentIds: ["I4"], description: "An email login's token always carries the customer role.", requires: L.and(tok, L.call("byEmail")), ensures: L.eq(role, "customer") });
  if (i5SaysEmailWins)
    add({ intentIds: ["I5"], description: "With both email and username, the email wins: any token is a customer token.", requires: L.and(L.eq(sends, "both"), tok), ensures: L.eq(role, "customer") });
  else add({ intentIds: ["I5"], description: "With both email and username, the request is invalid: 400.", requires: L.eq(sends, "both"), ensures: noToken(400) });

  return {
    lastro: "proof-plan/v0",
    artifactHash: artifact.hash!,
    model: MODEL,
    clauses,
    assumptions: [
      { id: "T1", intentIds: ["I1"].filter(has), piece: "find_by_email / find_by_username", contract: "accountFound reflects exactly whether the row exists in the chosen table", youAccept: "the lookups do not mix up tables or columns", ifWrong: "C2 and C4 hold only on paper: someone could sign in to the wrong account", checkedBy: ["S1", "S2"] },
      { id: "T2", intentIds: ["I1"].filter(has), piece: "check_password (bcrypt)", contract: "password=right iff the password matches the hash; bad-hash iff the hash is unreadable", youAccept: "bcrypt compares correctly", ifWrong: "any password may count as right: every password guarantee falls", checkedBy: ["S3"] },
      { id: "T3", intentIds: ["I2"].filter(has), piece: "generate_token (JWT)", contract: "with infra=ok, signing always produces a valid token for the claims", youAccept: "the JWT library signs what it is given", ifWrong: "the token may carry another role or the wrong expiry", checkedBy: ["S4"] },
    ].filter((a) => a.intentIds.length),
    scenarios: [
      { id: "S1", intentIds: ["I4"], given: "a registered customer", when: "they send email and the right password", then: "they get 200 with the customer role", input: { sends: "email", accountFound: true, password: "right", infra: "ok" }, expect: L.and(L.eq(st, 200), L.eq(role, "customer")) },
      { id: "S2", intentIds: ["I1"], given: "an email nobody holds", when: "someone tries to sign in", then: "they get 401", input: { sends: "email", accountFound: false, password: "right", infra: "ok" }, expect: L.eq(st, 401) },
      { id: "S3", intentIds: ["I1"], given: "a registered customer", when: "they type the wrong password", then: "they get 401 and no token", input: { sends: "email", accountFound: true, password: "wrong", infra: "ok" }, expect: L.and(L.eq(st, 401), L.not(tok)) },
      { id: "S4", intentIds: ["I3"], given: "a registered admin", when: "they send username and the right password", then: "they get 200 with the admin role", input: { sends: "username", accountFound: true, password: "right", infra: "ok" }, expect: L.and(L.eq(st, 200), L.eq(role, "admin")) },
    ].filter((s) => s.intentIds.every(has)),
    beyondProof: [
      { intentId: "I8", reason: "termination-timing" as const, note: "No clause says anything about time; a hung query violates nothing in the spec. Measure with a load test." },
      { intentId: "I9", reason: "human-judgment" as const, note: "Message clarity is a human judgment; needs product sign-off." },
    ].filter((b) => has(b.intentId)),
  };
}

// ---------------------------------------------------------------------------
// 4. The program (the "final output"), as source, and bugs to plant.
// ---------------------------------------------------------------------------

export const STEPS = [
  { name: "Pick the table", fn: "lookup_target" },
  { name: "Await the lookup", fn: "find_by_email / find_by_username" },
  { name: "Check the password", fn: "decide_login → check_password" },
  { name: "Sign sub, role, 24h exp", fn: "issue_token → generate_token" },
  { name: "Respond", fn: "login_flow" },
] as const;

export const LOGIN_SOURCE = `function decideLogin({ sends, accountFound, password, infra }, step) {
  // lookup_target: which table does the request name?
  if (sends === "neither") {
    step("Pick the table", "stop");
    return deny(400);
  }
  const table = sends === "username" ? "admins" : "customers"; // the email wins when both are sent
  step("Pick the table");

  // find_by_email / find_by_username  (trusted piece T1)
  if (infra === "db-fails") {
    step("Await the lookup", "stop");
    return deny(500);
  }
  step("Await the lookup");
  if (!accountFound) {
    step("Check the password", "stop");
    return deny(401);
  }

  // decide_login → check_password  (trusted piece T2)
  if (password === "bad-hash") {
    step("Check the password", "stop");
    return deny(500);
  }
  if (password === "wrong") {
    step("Check the password", "stop");
    return deny(401);
  }
  step("Check the password");

  // issue_token → generate_token  (trusted piece T3)
  if (infra === "signer-fails") {
    step("Sign sub, role, 24h exp", "stop");
    return deny(500);
  }
  step("Sign sub, role, 24h exp");
  step("Respond");
  return { status: 200, token: true, role: table === "admins" ? "admin" : "customer" };
}`;

export interface Bug {
  id: string;
  label: string;
  where: string;
  explain: string;
  /** Source edits that plant the bug: [find, replace], each find must occur exactly once. */
  patch: [string, string][];
}

export const BUGS: Bug[] = [
  {
    id: "refuse-everyone",
    label: "Refuse everyone",
    where: "decide_login",
    explain: "A readable password is always refused with 401. It never hands out a wrong token, so a one-way clause alone can't catch it; the both-ways clause can.",
    patch: [['if (password === "wrong") {', 'if (password === "wrong" || true) {']],
  },
  {
    id: "skip-password",
    label: "Skip the password check",
    where: "decide_login",
    explain: "Any password for an existing account gets a token.",
    patch: [
      ['if (password === "bad-hash") {', "if (false) {"],
      ['if (password === "wrong") {', "if (false) {"],
    ],
  },
  {
    id: "username-first",
    label: "Check the username first",
    where: "lookup_target",
    explain: "With both email and username, the admins table is searched.",
    patch: [['const table = sends === "username" ? "admins" : "customers";', 'const table = sends === "email" ? "customers" : "admins";']],
  },
  {
    id: "admin-everywhere",
    label: 'Write "admin" into every token',
    where: "issue_token",
    explain: "Every token leaves with the admin role.",
    patch: [['role: table === "admins" ? "admin" : "customer"', 'role: "admin"']],
  },
  {
    id: "db-as-401",
    label: "Report a database outage as 401",
    where: "login_flow",
    explain: "An infrastructure error becomes 'invalid credentials'.",
    patch: [['step("Await the lookup", "stop");\n    return deny(500);', 'step("Await the lookup", "stop");\n    return deny(401);']],
  },
  {
    id: "bad-hash-as-wrong",
    label: "Treat an unreadable hash as a wrong password",
    where: "decide_login",
    explain: "Corrupt data becomes 401 instead of 500.",
    patch: [['if (password === "bad-hash") {\n    step("Check the password", "stop");\n    return deny(500);', 'if (password === "bad-hash") {\n    step("Check the password", "stop");\n    return deny(401);']],
  },
];

export function plantBug(source: string, bugId?: string): string {
  if (!bugId) return source;
  const bug = BUGS.find((b) => b.id === bugId);
  if (!bug) throw new Error(`unknown bug '${bugId}'`);
  let out = source;
  for (const [find, replace] of bug.patch) {
    if (out.split(find).length !== 2) throw new Error(`bug '${bugId}': patch target not found exactly once`);
    out = out.replace(find, replace);
  }
  return out;
}

export type Step = { name: string; fn: string; status: "ok" | "stop" };
export type LoginFn = (input: Env, steps?: Step[]) => Env;

const deny = (status: number): Env => ({ status, token: false, role: "none" });

/**
 * Compile implementation source into a subject. Only use with source you
 * control (the sandbox runs the viewer's own edits in their own browser).
 */
export function compileLogin(source: string): LoginFn {
  // eslint-disable-next-line @typescript-eslint/no-implied-eval
  const fn = new Function("deny", `"use strict"; return (${source});`)(deny) as (input: Env, step: (name: string, status?: string) => void) => Env;
  if (typeof fn !== "function") throw new Error("source must be a single function expression");
  return (input, steps = []) => {
    const out = fn({ ...input }, (name, status = "ok") => steps.push({ name, fn: STEPS.find((s) => s.name === name)?.fn ?? "", status: status === "stop" ? "stop" : "ok" }));
    return { status: out?.status, token: out?.token, role: out?.role } as Env;
  };
}

/** The reference implementation, optionally with one planted bug. */
export function makeLogin(bug?: string): LoginFn {
  return compileLogin(plantBug(LOGIN_SOURCE, bug));
}
