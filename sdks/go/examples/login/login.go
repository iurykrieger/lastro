// Package login is the worked example of the TS SDK (sdks/typescript/examples/login), ported to Go:
// a store login, from prose to proof. Everything an LLM extractor or deriver would produce is
// written by hand, so it runs offline and deterministically, and the same planted bugs are
// available as Go implementations.
package login

import (
	"fmt"
	"regexp"

	lastro "github.com/iurykrieger/lastro/sdks/go"
)

// Prose is the business rule.
const Prose = `In the store login, only the right password gets a token, and only administrator accounts get administrator tokens. Whoever sends an email signs in as a customer; the administrator signs in with a username. If both are sent, the email wins. Today's errors stay: a request with neither email nor username gets 400, and a database failure gets 500. Login must respond quickly and the error message must be clear to the user.`

// AII5 is the AI's misreading of the prose; FixedI5 is the reviewer's amendment.
const (
	AII5    = "If email and username are sent together, the request is invalid (400)."
	FixedI5 = "If both email and username are sent, the email wins: the account is looked up among customers."
)

// Extract is what the AI proposed (note I5: the AI misread the prose).
func Extract(prose string) (lastro.IntentArtifact, error) {
	d := func(statement, kind, provability, quote string) lastro.IntentDraft {
		return lastro.IntentDraft{Statement: statement, Kind: kind, Provability: provability, Quote: quote}
	}
	i8 := d("Login responds quickly.", "non-functional", "beyond-proof", "Login must respond quickly")
	i8.BeyondProofReason = lastro.TerminationTiming
	i9 := d("The error message is clear to the user.", "non-functional", "beyond-proof", "the error message must be clear to the user")
	i9.BeyondProofReason = lastro.HumanJudgment
	return lastro.CreateArtifact(prose,
		[]lastro.IntentDraft{
			d("Only the right password for the named account gets a token.", "invariant", "formal", "only the right password gets a token"),
			d("The right password for the named account always gets a token, unless infrastructure fails (500).", "behavior", "formal", "only the right password gets a token"),
			d("Only administrator accounts (found by username) get a token with the admin role.", "invariant", "formal", "only administrator accounts get administrator tokens"),
			d("An email login always yields a token with the customer role.", "behavior", "formal", "Whoever sends an email signs in as a customer"),
			d(AII5, "behavior", "formal", "If both are sent, the email wins."),
			d("A request with neither email nor username gets 400 and no token.", "preserve", "formal", "a request with neither email nor username gets 400"),
			d("A database failure gets 500 and no token.", "preserve", "formal", "a database failure gets 500"),
			i8, i9,
		},
		[]lastro.DecisionDraft{
			{IntentIDs: []string{"I1", "I2"}, Question: "Must the right password ALWAYS get a token, or is it enough that a wrong one never does?", Options: []lastro.DecisionOption{
				{Label: "Both ways", Consequence: "A login that refuses everyone is rejected by the proof."},
				{Label: "Only refuse the wrong one", Consequence: "A login that refuses everyone still passes the formal clauses."},
			}},
			{IntentIDs: []string{"I1", "I7"}, Question: "When bcrypt cannot read the stored hash, does login answer 500 (as today) or 401?", Options: []lastro.DecisionOption{
				{Label: "500, as today", Consequence: "Keeps current behavior and separates corrupt data from a wrong password."},
				{Label: "401", Consequence: "A corrupt hash becomes indistinguishable from a wrong password."},
			}},
		},
		[]string{"That login finishes at all (a hung query violates no clause).", "Rate limiting of attempts."},
	)
}

// Model is the vocabulary of the proof. Trusted pieces enter as facts.
var Model = lastro.Model{
	Name: "login_flow",
	Inputs: lastro.Fields{
		{Name: "sends", Domain: lastro.Enum("email", "username", "both", "neither")}, // request fields
		{Name: "accountFound", Domain: lastro.Bool()},                                // T1: find_by_email / find_by_username
		{Name: "password", Domain: lastro.Enum("right", "wrong", "bad-hash")},        // T2: check_password (bcrypt)
		{Name: "infra", Domain: lastro.Enum("ok", "db-fails", "signer-fails")},       // T1 db errors, T3 signer
	},
	Outputs: lastro.Fields{
		{Name: "status", Domain: lastro.Enum(200, 400, 401, 500)},
		{Name: "token", Domain: lastro.Bool()},
		{Name: "role", Domain: lastro.Enum("none", "customer", "admin")},
	},
	Defs: lastro.Defs{
		{Name: "named", Def: lastro.Def{Params: []string{}, Description: "the request names an account that exists and the database answered",
			Body: lastro.And(lastro.Neq(lastro.In("sends"), "neither"), lastro.In("accountFound"), lastro.Neq(lastro.In("infra"), "db-fails"))}},
		{Name: "byEmail", Def: lastro.Def{Params: []string{}, Description: "the email picks the table (email or both)",
			Body: lastro.OneOf(lastro.In("sends"), "email", "both")}},
	},
}

var emailWins = regexp.MustCompile(`(?i)email wins|customer`)

// DerivePlan derives the proof plan; it depends on what the reviewer approved, amended and answered.
func DerivePlan(a lastro.IntentArtifact) lastro.ProofPlan {
	ok := map[string]bool{}
	for _, i := range lastro.ApprovedIntents(a) {
		ok[i.ID] = true
	}
	has := func(ids ...string) bool {
		for _, id := range ids {
			if !ok[id] {
				return false
			}
		}
		return true
	}
	choice := func(id string) int {
		for _, d := range a.Decisions {
			if d.ID == id {
				if d.Chosen != nil {
					return *d.Chosen
				}
				return d.Recommended
			}
		}
		return 0
	}
	i5SaysEmailWins := false
	for _, i := range a.Intents {
		if i.ID == "I5" {
			i5SaysEmailWins = emailWins.MatchString(i.Statement)
		}
	}

	L := struct{ named, sends, st, tok, role lastro.Expr }{lastro.Call("named"), lastro.In("sends"), lastro.Out("status"), lastro.Out("token"), lastro.Out("role")}
	noToken := func(code int) lastro.Expr {
		return lastro.And(lastro.Eq(L.st, code), lastro.Not(L.tok), lastro.Eq(L.role, "none"))
	}
	ptr := func(e lastro.Expr) *lastro.Expr { return &e }
	var clauses []lastro.Clause
	add := func(c lastro.Clause, when bool) {
		if when && has(c.IntentIDs...) {
			c.ID = fmt.Sprintf("C%d", len(clauses)+1)
			clauses = append(clauses, c)
		}
	}
	pw := lastro.In("password")
	infra := lastro.In("infra")
	add(lastro.Clause{IntentIDs: []string{"I6"}, Description: "Neither email nor username: 400 and no token.", Requires: ptr(lastro.Eq(L.sends, "neither")), Ensures: noToken(400)}, true)
	add(lastro.Clause{IntentIDs: []string{"I1"}, Description: "A token only for the named account, with the right password, and status 200.", Requires: ptr(L.tok), Ensures: lastro.And(L.named, lastro.Eq(pw, "right"), lastro.Eq(infra, "ok"), lastro.Eq(L.st, 200)), Assumes: []string{"T1", "T2"}}, true)
	add(lastro.Clause{IntentIDs: []string{"I2"}, Description: "The right password for the named account always gets a token, unless signing fails (500).", Requires: ptr(lastro.And(L.named, lastro.Eq(pw, "right"), lastro.Eq(infra, "ok"))), Ensures: lastro.And(lastro.Eq(L.st, 200), L.tok), Assumes: []string{"T1", "T2", "T3"}}, choice("D1") == 0)
	add(lastro.Clause{IntentIDs: []string{"I1"}, Description: "A login naming an account nobody holds gets 401, unless the database fails (500).", Requires: ptr(lastro.And(lastro.Neq(L.sends, "neither"), lastro.Not(lastro.In("accountFound")), lastro.Neq(infra, "db-fails"))), Ensures: noToken(401), Assumes: []string{"T1"}}, true)
	add(lastro.Clause{IntentIDs: []string{"I1"}, Description: "A wrong password for a readable account gets 401.", Requires: ptr(lastro.And(L.named, lastro.Eq(pw, "wrong"))), Ensures: noToken(401), Assumes: []string{"T2"}}, true)
	badHash, badHashText := 500, "A hash bcrypt cannot read gets 500 and no token."
	if choice("D2") != 0 {
		badHash, badHashText = 401, "A hash bcrypt cannot read gets 401 and no token."
	}
	add(lastro.Clause{IntentIDs: []string{"I1"}, Description: badHashText, Requires: ptr(lastro.And(L.named, lastro.Eq(pw, "bad-hash"))), Ensures: noToken(badHash), Assumes: []string{"T2"}}, true)
	add(lastro.Clause{IntentIDs: []string{"I7"}, Description: "A database failure gets 500 and no token.", Requires: ptr(lastro.And(lastro.Neq(L.sends, "neither"), lastro.Eq(infra, "db-fails"))), Ensures: noToken(500)}, true)
	add(lastro.Clause{IntentIDs: []string{"I3"}, Description: "The admin role only for an account found by username, with no email sent.", Requires: ptr(lastro.Eq(L.role, "admin")), Ensures: lastro.And(L.tok, lastro.Eq(L.sends, "username"), L.named)}, true)
	add(lastro.Clause{IntentIDs: []string{"I3"}, Description: "A token issued for a username carries the admin role.", Requires: ptr(lastro.And(L.tok, lastro.Eq(L.sends, "username"))), Ensures: lastro.Eq(L.role, "admin")}, true)
	add(lastro.Clause{IntentIDs: []string{"I4"}, Description: "An email login's token always carries the customer role.", Requires: ptr(lastro.And(L.tok, lastro.Call("byEmail"))), Ensures: lastro.Eq(L.role, "customer")}, true)
	if i5SaysEmailWins {
		add(lastro.Clause{IntentIDs: []string{"I5"}, Description: "With both email and username, the email wins: any token is a customer token.", Requires: ptr(lastro.And(lastro.Eq(L.sends, "both"), L.tok)), Ensures: lastro.Eq(L.role, "customer")}, true)
	} else {
		add(lastro.Clause{IntentIDs: []string{"I5"}, Description: "With both email and username, the request is invalid: 400.", Requires: ptr(lastro.Eq(L.sends, "both")), Ensures: noToken(400)}, true)
	}

	var assumptions []lastro.Assumption
	for _, t := range []lastro.Assumption{
		{ID: "T1", IntentIDs: []string{"I1"}, Piece: "find_by_email / find_by_username", Contract: "accountFound reflects exactly whether the row exists in the chosen table", YouAccept: "the lookups do not mix up tables or columns", IfWrong: "C2 and C4 hold only on paper: someone could sign in to the wrong account", CheckedBy: []string{"S1", "S2"}},
		{ID: "T2", IntentIDs: []string{"I1"}, Piece: "check_password (bcrypt)", Contract: "password=right iff the password matches the hash; bad-hash iff the hash is unreadable", YouAccept: "bcrypt compares correctly", IfWrong: "any password may count as right: every password guarantee falls", CheckedBy: []string{"S3"}},
		{ID: "T3", IntentIDs: []string{"I2"}, Piece: "generate_token (JWT)", Contract: "with infra=ok, signing always produces a valid token for the claims", YouAccept: "the JWT library signs what it is given", IfWrong: "the token may carry another role or the wrong expiry", CheckedBy: []string{"S4"}},
	} {
		if has(t.IntentIDs...) {
			assumptions = append(assumptions, t)
		}
	}
	in := func(sends string, found bool, password, infra string) map[string]any {
		return map[string]any{"sends": sends, "accountFound": found, "password": password, "infra": infra}
	}
	var scenarios []lastro.Scenario
	for _, s := range []lastro.Scenario{
		{ID: "S1", IntentIDs: []string{"I4"}, Given: "a registered customer", When: "they send email and the right password", Then: "they get 200 with the customer role", Input: in("email", true, "right", "ok"), Expect: lastro.And(lastro.Eq(L.st, 200), lastro.Eq(L.role, "customer"))},
		{ID: "S2", IntentIDs: []string{"I1"}, Given: "an email nobody holds", When: "someone tries to sign in", Then: "they get 401", Input: in("email", false, "right", "ok"), Expect: lastro.Eq(L.st, 401)},
		{ID: "S3", IntentIDs: []string{"I1"}, Given: "a registered customer", When: "they type the wrong password", Then: "they get 401 and no token", Input: in("email", true, "wrong", "ok"), Expect: lastro.And(lastro.Eq(L.st, 401), lastro.Not(L.tok))},
		{ID: "S4", IntentIDs: []string{"I3"}, Given: "a registered admin", When: "they send username and the right password", Then: "they get 200 with the admin role", Input: in("username", true, "right", "ok"), Expect: lastro.And(lastro.Eq(L.st, 200), lastro.Eq(L.role, "admin"))},
	} {
		if has(s.IntentIDs...) {
			for k, v := range s.Input {
				s.Input[k] = lastro.Normalize(v)
			}
			scenarios = append(scenarios, s)
		}
	}
	var beyond []lastro.BeyondProofItem
	for _, b := range []lastro.BeyondProofItem{
		{IntentID: "I8", Reason: lastro.TerminationTiming, Note: "No clause says anything about time; a hung query violates nothing in the spec. Measure with a load test."},
		{IntentID: "I9", Reason: lastro.HumanJudgment, Note: "Message clarity is a human judgment; needs product sign-off."},
	} {
		if has(b.IntentID) {
			beyond = append(beyond, b)
		}
	}
	return lastro.ProofPlan{Lastro: "proof-plan/v0", ArtifactHash: a.Hash, Model: Model, Clauses: clauses, Assumptions: assumptions, Scenarios: scenarios, BeyondProof: beyond}
}

// Bugs are the planted bugs of the TS example, by id.
var Bugs = []string{"refuse-everyone", "skip-password", "username-first", "admin-everywhere", "db-as-401", "bad-hash-as-wrong"}

func deny(status int) lastro.Env { return lastro.Env{"status": status, "token": false, "role": "none"} }

// Decide is the reference implementation (decide_login), optionally with one planted bug.
func Decide(bug string) lastro.Subject {
	return func(in lastro.Env) (lastro.Env, error) {
		sends, _ := in["sends"].(string)
		found, _ := in["accountFound"].(bool)
		password, _ := in["password"].(string)
		infra, _ := in["infra"].(string)

		if sends == "neither" {
			return deny(400), nil
		}
		table := "customers" // the email wins when both are sent
		if sends == "username" {
			table = "admins"
		}
		if bug == "username-first" && sends != "email" {
			table = "admins"
		}
		if infra == "db-fails" {
			if bug == "db-as-401" {
				return deny(401), nil
			}
			return deny(500), nil
		}
		if !found {
			return deny(401), nil
		}
		skip := bug == "skip-password"
		if password == "bad-hash" && !skip {
			if bug == "bad-hash-as-wrong" {
				return deny(401), nil
			}
			return deny(500), nil
		}
		if (password == "wrong" || bug == "refuse-everyone") && !skip {
			return deny(401), nil
		}
		if infra == "signer-fails" {
			return deny(500), nil
		}
		role := "customer"
		if table == "admins" {
			role = "admin"
		}
		if bug == "admin-everywhere" {
			role = "admin"
		}
		return lastro.Env{"status": 200, "token": true, "role": role}, nil
	}
}
