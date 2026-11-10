/**
 * Where the sandbox gets Claude from.
 *
 * - Published on claude.ai: the page's `sample` capability (the viewer's own Claude).
 * - `npm run sandbox` locally: a dev-server endpoint that calls the Anthropic API
 *   with ANTHROPIC_API_KEY (see vite.config.ts). The key never reaches the browser.
 *
 * Either way the SDK's transport-free LLM steps do the rest, and nothing Claude
 * returns is trusted: quotes are anchored, LSL is validated, code is proved.
 */
import type { AskJson } from "@lastro/sdk";

type SampleFn = ((input: string, opts?: Record<string, unknown>) => Promise<{ text: string }>) & {
  json: (input: string, opts?: Record<string, unknown>) => Promise<unknown>;
};

declare global {
  interface Window {
    claude?: { use(name: string): Promise<unknown> };
  }
}

export interface Ai {
  kind: "claude-in-page" | "dev-server";
  ask: AskJson;
}

const ERRORS: Record<string, string> = {
  not_granted: "Claude was not allowed for this page. Allow it from the page's permissions to edit the prose.",
  sampling_disabled: "Claude is not available for this account.",
  rate_limited: "Too many Claude calls right now. Wait a moment and try again.",
  invalid_json: "Claude's answer was not valid JSON. Try again.",
  refused: "Claude declined this request. Rephrase the prose and try again.",
  prompt_too_large: "The prose is too long for one call.",
  session_expired: "Sign in to claude.ai again.",
};

function sampleAsk(sample: SampleFn): AskJson {
  return async ({ step, system, user, schema }) => {
    const prompt = `${system}\n\n${user}\n\nReply with only one JSON value, no other text, matching this JSON Schema:\n${JSON.stringify(schema)}`;
    try {
      return await sample.json(prompt, { modelTier: step === "extract" ? "default" : "complex", cache: false });
    } catch (e) {
      const code = (e as { code?: string })?.code ?? "upstream_error";
      throw new Error(ERRORS[code] ?? `Claude call failed (${code}). Try again.`);
    }
  };
}

const devAsk: AskJson = async (req) => {
  const res = await fetch("/api/lastro/ask", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(req) });
  const body = (await res.json()) as { ok: boolean; value?: unknown; error?: string };
  if (!body.ok) throw new Error(body.error ?? `dev server error ${res.status}`);
  return body.value;
};

export async function detectAi(): Promise<Ai | null> {
  try {
    const sample = (await window.claude?.use("sample")) as SampleFn | null | undefined;
    if (sample) return { kind: "claude-in-page", ask: sampleAsk(sample) };
  } catch {
    /* fall through */
  }
  if (import.meta.env.DEV) {
    try {
      const r = await fetch("/api/lastro/ask");
      if (r.ok) return { kind: "dev-server", ask: devAsk };
    } catch {
      /* no dev endpoint */
    }
  }
  return null;
}
