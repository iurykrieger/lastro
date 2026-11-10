using Lastro;

namespace Lastro.Tests;

/// <summary>
/// The store login worked example, ported from packages/sdk/examples/login/login.ts. Everything an LLM
/// would produce is written by hand so the tests run offline. It must stay identical to the TypeScript
/// example: the cross-check compares hashes, verdicts and receipts between the two SDKs.
/// </summary>
public static class LoginExample
{
    public const string Prose = "In the store login, only the right password gets a token, and only administrator accounts get administrator tokens. Whoever sends an email signs in as a customer; the administrator signs in with a username. If both are sent, the email wins. Today's errors stay: a request with neither email nor username gets 400, and a database failure gets 500. Login must respond quickly and the error message must be clear to the user.";

    public const string AiI5 = "If email and username are sent together, the request is invalid (400).";
    public const string FixedI5 = "If both email and username are sent, the email wins: the account is looked up among customers.";

    public static IntentArtifact Extract(string prose = Prose) => Artifacts.Create(
        prose,
        [
            new("Only the right password for the named account gets a token.", IntentKind.Invariant, Provability.Formal, "only the right password gets a token"),
            new("The right password for the named account always gets a token, unless infrastructure fails (500).", IntentKind.Behavior, Provability.Formal, "only the right password gets a token"),
            new("Only administrator accounts (found by username) get a token with the admin role.", IntentKind.Invariant, Provability.Formal, "only administrator accounts get administrator tokens"),
            new("An email login always yields a token with the customer role.", IntentKind.Behavior, Provability.Formal, "Whoever sends an email signs in as a customer"),
            new(AiI5, IntentKind.Behavior, Provability.Formal, "If both are sent, the email wins."),
            new("A request with neither email nor username gets 400 and no token.", IntentKind.Preserve, Provability.Formal, "a request with neither email nor username gets 400"),
            new("A database failure gets 500 and no token.", IntentKind.Preserve, Provability.Formal, "a database failure gets 500"),
            new("Login responds quickly.", IntentKind.NonFunctional, Provability.BeyondProof, "Login must respond quickly", BeyondProofReason.TerminationTiming),
            new("The error message is clear to the user.", IntentKind.NonFunctional, Provability.BeyondProof, "the error message must be clear to the user", BeyondProofReason.HumanJudgment),
        ],
        [
            new(["I1", "I2"], "Must the right password ALWAYS get a token, or is it enough that a wrong one never does?",
                [new() { Label = "Both ways", Consequence = "A login that refuses everyone is rejected by the proof." },
                 new() { Label = "Only refuse the wrong one", Consequence = "A login that refuses everyone still passes the formal clauses." }], 0),
            new(["I1", "I7"], "When bcrypt cannot read the stored hash, does login answer 500 (as today) or 401?",
                [new() { Label = "500, as today", Consequence = "Keeps current behavior and separates corrupt data from a wrong password." },
                 new() { Label = "401", Consequence = "A corrupt hash becomes indistinguishable from a wrong password." }], 0),
        ],
        ["That login finishes at all (a hung query violates no clause).", "Rate limiting of attempts."]);

    public static Model Model => new()
    {
        Name = "login_flow",
        Inputs = new()
        {
            ["sends"] = D.Enum("email", "username", "both", "neither"),
            ["accountFound"] = D.Bool(),
            ["password"] = D.Enum("right", "wrong", "bad-hash"),
            ["infra"] = D.Enum("ok", "db-fails", "signer-fails"),
        },
        Outputs = new()
        {
            ["status"] = D.Enum(200, 400, 401, 500),
            ["token"] = D.Bool(),
            ["role"] = D.Enum("none", "customer", "admin"),
        },
        Defs = new()
        {
            ["named"] = new Def
            {
                Params = [],
                Description = "the request names an account that exists and the database answered",
                Body = L.And(L.Neq(L.In("sends"), "neither"), L.In("accountFound"), L.Neq(L.In("infra"), "db-fails")),
            },
            ["byEmail"] = new Def { Params = [], Description = "the email picks the table (email or both)", Body = L.OneOf(L.In("sends"), "email", "both") },
        },
    };

    public static ProofPlan Derive(IntentArtifact artifact)
    {
        var ok = Artifacts.ApprovedIntents(artifact).Select(i => i.Id).ToHashSet();
        bool Has(string id) => ok.Contains(id);
        int Choice(string id)
        {
            var d = artifact.Decisions.FirstOrDefault(x => x.Id == id);
            return d?.Chosen ?? d?.Recommended ?? 0;
        }
        var i5 = artifact.Intents.FirstOrDefault(i => i.Id == "I5");
        var i5SaysEmailWins = i5 is not null && System.Text.RegularExpressions.Regex.IsMatch(i5.Statement, "email wins|customer", System.Text.RegularExpressions.RegexOptions.IgnoreCase);

        var named = L.Call("named");
        var sends = L.In("sends");
        var st = L.Out("status");
        var tok = L.Out("token");
        var role = L.Out("role");
        Expr NoToken(int code) => L.And(L.Eq(st, code), L.Not(tok), L.Eq(role, "none"));

        var clauses = new List<Clause>();
        void Add(string[] intentIds, string description, Expr requires, Expr ensures, string[]? assumes = null, bool when = true)
        {
            if (when && intentIds.All(Has))
                clauses.Add(new Clause { Id = $"C{clauses.Count + 1}", IntentIds = [.. intentIds], Description = description, Requires = requires, Ensures = ensures, Assumes = assumes?.ToList() });
        }

        Add(["I6"], "Neither email nor username: 400 and no token.", L.Eq(sends, "neither"), NoToken(400));
        Add(["I1"], "A token only for the named account, with the right password, and status 200.", tok, L.And(named, L.Eq(L.In("password"), "right"), L.Eq(L.In("infra"), "ok"), L.Eq(st, 200)), ["T1", "T2"]);
        Add(["I2"], "The right password for the named account always gets a token, unless signing fails (500).", L.And(named, L.Eq(L.In("password"), "right"), L.Eq(L.In("infra"), "ok")), L.And(L.Eq(st, 200), tok), ["T1", "T2", "T3"], Choice("D1") == 0);
        Add(["I1"], "A login naming an account nobody holds gets 401, unless the database fails (500).", L.And(L.Neq(sends, "neither"), L.Not(L.In("accountFound")), L.Neq(L.In("infra"), "db-fails")), NoToken(401), ["T1"]);
        Add(["I1"], "A wrong password for a readable account gets 401.", L.And(named, L.Eq(L.In("password"), "wrong")), NoToken(401), ["T2"]);
        Add(["I1"], Choice("D2") == 0 ? "A hash bcrypt cannot read gets 500 and no token." : "A hash bcrypt cannot read gets 401 and no token.", L.And(named, L.Eq(L.In("password"), "bad-hash")), NoToken(Choice("D2") == 0 ? 500 : 401), ["T2"]);
        Add(["I7"], "A database failure gets 500 and no token.", L.And(L.Neq(sends, "neither"), L.Eq(L.In("infra"), "db-fails")), NoToken(500));
        Add(["I3"], "The admin role only for an account found by username, with no email sent.", L.Eq(role, "admin"), L.And(tok, L.Eq(sends, "username"), named));
        Add(["I3"], "A token issued for a username carries the admin role.", L.And(tok, L.Eq(sends, "username")), L.Eq(role, "admin"));
        Add(["I4"], "An email login's token always carries the customer role.", L.And(tok, L.Call("byEmail")), L.Eq(role, "customer"));
        if (i5SaysEmailWins)
            Add(["I5"], "With both email and username, the email wins: any token is a customer token.", L.And(L.Eq(sends, "both"), tok), L.Eq(role, "customer"));
        else Add(["I5"], "With both email and username, the request is invalid: 400.", L.Eq(sends, "both"), NoToken(400));

        Env In(string s, bool found, string pw, string infra) => new() { ["sends"] = s, ["accountFound"] = found, ["password"] = pw, ["infra"] = infra };

        return new ProofPlan
        {
            ArtifactHash = artifact.Hash!,
            Model = Model,
            Clauses = clauses,
            Assumptions = new List<Assumption>
            {
                new() { Id = "T1", IntentIds = new[] { "I1" }.Where(Has).ToList(), Piece = "find_by_email / find_by_username", Contract = "accountFound reflects exactly whether the row exists in the chosen table", YouAccept = "the lookups do not mix up tables or columns", IfWrong = "C2 and C4 hold only on paper: someone could sign in to the wrong account", CheckedBy = ["S1", "S2"] },
                new() { Id = "T2", IntentIds = new[] { "I1" }.Where(Has).ToList(), Piece = "check_password (bcrypt)", Contract = "password=right iff the password matches the hash; bad-hash iff the hash is unreadable", YouAccept = "bcrypt compares correctly", IfWrong = "any password may count as right: every password guarantee falls", CheckedBy = ["S3"] },
                new() { Id = "T3", IntentIds = new[] { "I2" }.Where(Has).ToList(), Piece = "generate_token (JWT)", Contract = "with infra=ok, signing always produces a valid token for the claims", YouAccept = "the JWT library signs what it is given", IfWrong = "the token may carry another role or the wrong expiry", CheckedBy = ["S4"] },
            }.Where(a => a.IntentIds.Count > 0).ToList(),
            Scenarios = new List<Scenario>
            {
                new() { Id = "S1", IntentIds = ["I4"], Given = "a registered customer", When = "they send email and the right password", Then = "they get 200 with the customer role", Input = In("email", true, "right", "ok"), Expect = L.And(L.Eq(st, 200), L.Eq(role, "customer")) },
                new() { Id = "S2", IntentIds = ["I1"], Given = "an email nobody holds", When = "someone tries to sign in", Then = "they get 401", Input = In("email", false, "right", "ok"), Expect = L.Eq(st, 401) },
                new() { Id = "S3", IntentIds = ["I1"], Given = "a registered customer", When = "they type the wrong password", Then = "they get 401 and no token", Input = In("email", true, "wrong", "ok"), Expect = L.And(L.Eq(st, 401), L.Not(tok)) },
                new() { Id = "S4", IntentIds = ["I3"], Given = "a registered admin", When = "they send username and the right password", Then = "they get 200 with the admin role", Input = In("username", true, "right", "ok"), Expect = L.And(L.Eq(st, 200), L.Eq(role, "admin")) },
            }.Where(s => s.IntentIds.All(Has)).ToList(),
            BeyondProof = new List<BeyondProofItem>
            {
                new() { IntentId = "I8", Reason = BeyondProofReason.TerminationTiming, Note = "No clause says anything about time; a hung query violates nothing in the spec. Measure with a load test." },
                new() { IntentId = "I9", Reason = BeyondProofReason.HumanJudgment, Note = "Message clarity is a human judgment; needs product sign-off." },
            }.Where(b => Has(b.IntentId)).ToList(),
        };
    }

    /// <summary>The bugs of the TypeScript example, as flags on the C# implementation.</summary>
    public static readonly string[] Bugs = ["refuse-everyone", "skip-password", "username-first", "admin-everywhere", "db-as-401", "bad-hash-as-wrong"];

    private static Env Deny(int status) => new() { ["status"] = status, ["token"] = false, ["role"] = "none" };

    /// <summary>The reference decision core, optionally with one planted bug.</summary>
    public static Env Decide(Env input, string? bug = null)
    {
        var sends = input["sends"].AsString;
        var accountFound = input["accountFound"].AsBool;
        var password = input["password"].AsString;
        var infra = input["infra"].AsString;
        if (sends == "neither") return Deny(400);
        var table = bug == "username-first" ? (sends == "email" ? "customers" : "admins") : (sends == "username" ? "admins" : "customers");
        if (infra == "db-fails") return Deny(bug == "db-as-401" ? 401 : 500);
        if (!accountFound) return Deny(401);
        if (bug != "skip-password")
        {
            if (password == "bad-hash") return Deny(bug == "bad-hash-as-wrong" ? 401 : 500);
            if (password == "wrong" || bug == "refuse-everyone") return Deny(401);
        }
        if (infra == "signer-fails") return Deny(500);
        return new Env { ["status"] = 200, ["token"] = true, ["role"] = bug == "admin-everywhere" ? "admin" : table == "admins" ? "admin" : "customer" };
    }

    public static Subject Subject(string? bug = null) => Subjects.From(input => Decide(input, bug));

    // Entry points the CLI loads by reflection (no Lastro types in the signature).
    public static IDictionary<string, object?> Reference(IDictionary<string, object?> input) => Decide(Env.From(input)).ToObjects();

    public static IDictionary<string, object?> SkipPassword(IDictionary<string, object?> input) => Decide(Env.From(input), "skip-password").ToObjects();
}
