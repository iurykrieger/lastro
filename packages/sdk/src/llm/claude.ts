/**
 * Claude-backed extractor, deriver and gap reviewer over the Anthropic API.
 *
 * The prompts, schemas and checks live in ./llm.ts and are shared with any
 * other transport. `@anthropic-ai/sdk` is an optional peer dependency,
 * imported from the `lastro/claude` entry point only.
 */
import Anthropic from "@anthropic-ai/sdk";
import { LlmGapReviewer, LlmIntentExtractor, LlmProofDeriver, type AskJson } from "./llm.js";

export interface ClaudeOptions {
  client?: Anthropic;
  model?: string;
  effort?: "low" | "medium" | "high" | "xhigh" | "max";
}

const DEFAULT_MODEL = "claude-opus-5-5";

/** An AskJson transport over the Anthropic Messages API, with structured outputs. */
export function claudeAsk(opts: ClaudeOptions = {}): AskJson {
  return async ({ system, user, schema }) => {
    const client = opts.client ?? new Anthropic();
    const stream = client.beta.messages.stream({
      model: opts.model ?? DEFAULT_MODEL,
      max_tokens: 64000,
      betas: ["server-side-fallback-2026-07-01"],
      // On a policy decline, the API re-runs the request on a fallback model.
      fallbacks: "default",
      thinking: { type: "adaptive" },
      output_config: { effort: opts.effort ?? "high", format: { type: "json_schema", schema } },
      system,
      messages: [{ role: "user", content: user }],
    } as unknown as Parameters<typeof client.beta.messages.stream>[0]);
    const msg = await stream.finalMessage();
    if (msg.stop_reason === "refusal") throw new Error("Claude declined the request (stop_reason=refusal)");
    if (msg.stop_reason === "max_tokens") throw new Error("Claude output was truncated (max_tokens)");
    const text = msg.content.flatMap((b) => (b.type === "text" ? [b.text] : [])).join("");
    return JSON.parse(text) as unknown;
  };
}

export class ClaudeIntentExtractor extends LlmIntentExtractor {
  constructor(opts: ClaudeOptions = {}) {
    super(claudeAsk(opts));
  }
}

export class ClaudeProofDeriver extends LlmProofDeriver {
  constructor(opts: ClaudeOptions = {}) {
    super(claudeAsk(opts));
  }
}

export class ClaudeGapReviewer extends LlmGapReviewer {
  constructor(opts: ClaudeOptions = {}) {
    super(claudeAsk(opts));
  }
}
