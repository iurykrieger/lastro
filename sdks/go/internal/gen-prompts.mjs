// Regenerates ../prompts.go from the TS SDK, so both SDKs send byte-identical prompts and schemas.
// Usage (from sdks/go): npm run build -w lastro && go generate ./...
import { writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
const L = await import(new URL("../../../packages/sdk/dist/src/index.js", import.meta.url).href);
const q = (s) => JSON.stringify(s);
let out = `// Code generated from packages/sdk/src/llm/llm.ts by sdks/go/internal/gen-prompts.mjs. DO NOT EDIT.\n// The TS and Go SDKs must send the same prompts and schemas; crosscheck_test.go checks they agree.\n\npackage lastro\n\n`;
for (const [go, ts, step] of [["ExtractSystem", "EXTRACT_SYSTEM", "extract"], ["DeriveSystem", "DERIVE_SYSTEM", "derive"], ["GapReviewSystem", "GAP_REVIEW_SYSTEM", "gap-review"]])
  out += `// ${go} is the system prompt of the ${step} step.\nconst ${go} = ${q(L[ts])}\n\n`;
for (const [go, ts, step] of [["ExtractSchema", "EXTRACT_SCHEMA", "extract"], ["DeriveSchema", "DERIVE_SCHEMA", "derive"], ["GapReviewSchema", "GAP_REVIEW_SCHEMA", "gap-review"]])
  out += `// ${go} is the JSON Schema of the ${step} answer, with the TS key order.\nconst ${go} = ${q(JSON.stringify(L[ts]))}\n\n`;
writeFileSync(fileURLToPath(new URL("../prompts.go", import.meta.url)), out.trimEnd() + "\n");
