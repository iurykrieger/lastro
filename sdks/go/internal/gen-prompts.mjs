// Regenerates ../prompts.go from the TS SDK, so both SDKs send byte-identical prompts and schemas.
// Usage (from sdks/go): npm run build -w lastro && go generate ./...
import { writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
const L = await import(new URL("../../typescript/dist/src/index.js", import.meta.url).href);
const q = (s) => JSON.stringify(s);
let out = `// Code generated from sdks/typescript/src/llm/llm.ts and llm/proposal.ts by sdks/go/internal/gen-prompts.mjs. DO NOT EDIT.\n// The TS and Go SDKs must send the same prompts and schemas; crosscheck_test.go checks they agree.\n\npackage lastro\n\n`;
for (const [go, ts, step] of [["ExtractSystem", "EXTRACT_SYSTEM", "extract"], ["DeriveSystem", "DERIVE_SYSTEM", "derive"], ["GapReviewSystem", "GAP_REVIEW_SYSTEM", "gap-review"], ["ProposeSystem", "PROPOSE_SYSTEM", "propose (extract)"], ["RecombineSystem", "RECOMBINE_SYSTEM", "recombine (extract)"], ["DeriveProofsSystem", "DERIVE_PROOFS_SYSTEM", "derive (proposal/v1)"]])
  out += `// ${go} is the system prompt of the ${step} step.\nconst ${go} = ${q(L[ts])}\n\n`;
for (const [go, ts, step] of [["ExtractSchema", "EXTRACT_SCHEMA", "extract"], ["DeriveSchema", "DERIVE_SCHEMA", "derive"], ["GapReviewSchema", "GAP_REVIEW_SCHEMA", "gap-review"], ["ProposeSchema", "PROPOSE_SCHEMA", "propose (extract)"], ["RecombineSchema", "RECOMBINE_SCHEMA", "recombine (extract)"], ["DeriveProofsSchema", "DERIVE_PROOFS_SCHEMA", "derive (proposal/v1)"]])
  out += `// ${go} is the JSON Schema of the ${step} answer, with the TS key order.\nconst ${go} = ${q(JSON.stringify(L[ts]))}\n\n`;
writeFileSync(fileURLToPath(new URL("../prompts.go", import.meta.url)), out.trimEnd() + "\n");
