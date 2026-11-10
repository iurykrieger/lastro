// Generated from packages/sdk/src/llm/llm.ts. Keep identical: both SDKs must send the same prompts,
// so a handoff request written by one CLI is answered once for both.
namespace Lastro;

public static partial class LlmPrompts
{
    public const string ExtractSystem = """"
You turn a business rule or change request, written as prose, into a list of reviewable intents.

Rules:
- One intent = one checkable promise. Split compound sentences ("only X and always Y" is two intents).
- "quote" must be copied verbatim from the prose (it anchors the intent map). Never paraphrase the quote.
- "statement" restates the promise precisely in the prose's language.
- kind: behavior (must happen), invariant (must never happen), preserve (existing behavior to keep), non-functional.
- provability: "formal" when it is a pure input→output fact about the program's decision logic; "empirical" when it can only be checked by running tests; "beyond-proof" when it falls where proof stops.
- beyondProofReason (required for beyond-proof): external-code (libraries the verifier cannot see), world-facts (database, clock), termination-timing (whether/how fast it finishes), human-judgment (UX, clarity), unstated.
- When the prose is ambiguous, DO NOT guess. Add a decision: one question, 2-4 options each with its consequence, and the index of the option you recommend. Prefer questions the author has not considered (e.g. "must the right password ALWAYS get a token, or only must a wrong one be refused?").
- notPromised: things a reader might assume but the prose does not promise (timing, rate limits...).
"""";

    public const string DeriveSystem = """"
You derive proof obligations from APPROVED intents, in LSL (Lastro Spec Language).

LSL expressions are JSON:
  {"var":"in.<input>"} | {"var":"out.<output>"} | {"const": true|1|"admin"}
  {"op": and|or|not|implies|iff|eq|neq|lt|le|gt|ge|add|sub|mul|ite|in, "args":[...]}
  {"call":"<def>", "args":[...]}
Only use the variables and defs of the given model. Each expression is returned as a JSON STRING.

Obligations:
- clauses: formal if-then promises "for every input: requires ⇒ ensures". One sentence of intent may need several clauses. Prefer "both ways" clauses (e.g. the right password ALWAYS gets a token, AND a wrong one NEVER does), since a one-sided clause lets a program that refuses everyone pass.
- Never weaken a clause to make it provable. If an intent cannot be expressed in the model, put it in beyondProof.
- assumptions: trusted pieces (external code, world facts) the proof relies on, each with youAccept / ifWrong and the scenarios that test it.
- scenarios: Given/When/Then examples with a concrete input (every model input) and an LSL "expect".
- beyondProof: intents (or parts) where proof stops, with the reason.
- Every approved intent must be covered by at least one obligation or a beyondProof entry. Never reference rejected intents.
"""";

    public const string GapReviewSystem = """"
You review a proof-driven change for gaps the proofs cannot see.

You get the approved intents, the formal clauses that were proved, the trusted pieces, what is beyond proof, and the code.
Report only real gaps, each tied to an intent:
- an intent whose meaning is only partly captured by its clauses (the proof passes but the intent could still be broken),
- code behavior that relies on a trusted piece or an input the model does not represent,
- an intent with no clause and no beyond-proof reason.
Do not restate what is already proved. An empty list is a fine answer.
"""";

    public const string ExtractSchemaJson = """"
{"type":"object","additionalProperties":false,"required":["intents","decisions","notPromised"],"properties":{"intents":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["statement","kind","provability","beyondProofReason","quote"],"properties":{"statement":{"type":"string"},"kind":{"type":"string","enum":["behavior","invariant","preserve","non-functional"]},"provability":{"type":"string","enum":["formal","empirical","beyond-proof"]},"beyondProofReason":{"type":"string","enum":["none","external-code","world-facts","termination-timing","human-judgment","unstated"]},"quote":{"type":"string"}}}},"decisions":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["intentIndexes","question","options","recommended"],"properties":{"intentIndexes":{"type":"array","items":{"type":"integer"}},"question":{"type":"string"},"options":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["label","consequence"],"properties":{"label":{"type":"string"},"consequence":{"type":"string"}}}},"recommended":{"type":"integer"}}}},"notPromised":{"type":"array","items":{"type":"string"}}}}
"""";

    public const string DeriveSchemaJson = """"
{"type":"object","additionalProperties":false,"required":["clauses","assumptions","scenarios","beyondProof"],"properties":{"clauses":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["id","intentIds","description","requires","ensures","assumes"],"properties":{"id":{"type":"string"},"intentIds":{"type":"array","items":{"type":"string"}},"description":{"type":"string"},"requires":{"type":"string","description":"LSL JSON string, or empty string for no if-part"},"ensures":{"type":"string","description":"LSL JSON string"},"assumes":{"type":"array","items":{"type":"string"}}}}},"assumptions":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["id","intentIds","piece","contract","youAccept","ifWrong","checkedBy"],"properties":{"id":{"type":"string"},"intentIds":{"type":"array","items":{"type":"string"}},"piece":{"type":"string"},"contract":{"type":"string"},"youAccept":{"type":"string"},"ifWrong":{"type":"string"},"checkedBy":{"type":"array","items":{"type":"string"}}}}},"scenarios":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["id","intentIds","given","when","then","inputJson","expect"],"properties":{"id":{"type":"string"},"intentIds":{"type":"array","items":{"type":"string"}},"given":{"type":"string"},"when":{"type":"string"},"then":{"type":"string"},"inputJson":{"type":"string","description":"JSON object with a value for every model input"},"expect":{"type":"string","description":"LSL JSON string over in.* and out.*"}}}},"beyondProof":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["intentId","reason","note"],"properties":{"intentId":{"type":"string"},"reason":{"type":"string","enum":["external-code","world-facts","termination-timing","human-judgment","unstated"]},"note":{"type":"string"}}}}}}
"""";

    public const string GapReviewSchemaJson = """"
{"type":"object","additionalProperties":false,"required":["gaps"],"properties":{"gaps":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["intentId","gap","suggestion","severity"],"properties":{"intentId":{"type":"string"},"gap":{"type":"string"},"suggestion":{"type":"string"},"severity":{"type":"string","enum":["low","medium","high"]}}}}}}
"""";
}
