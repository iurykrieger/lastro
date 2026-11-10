/**
 * A deliberately small highlighter: comments, strings, numbers and keywords for the languages the docs show.
 * No dependency, no HTML strings: it returns tokens that React renders as spans.
 */
export type Lang = "ts" | "js" | "go" | "csharp" | "json" | "bash" | "text" | "markdown";
export type TokenKind = "comment" | "string" | "number" | "keyword" | "prompt" | "plain";
export interface Token {
  kind: TokenKind;
  text: string;
}

const KEYWORDS: Partial<Record<Lang, string[]>> = {
  ts: ["import", "from", "export", "default", "const", "let", "await", "async", "function", "return", "if", "else", "new", "type", "interface", "for", "of", "true", "false", "null", "undefined"],
  go: ["package", "import", "func", "return", "if", "else", "for", "range", "var", "const", "type", "struct", "nil", "true", "false", "defer", "map", "err"],
  csharp: ["using", "var", "new", "await", "async", "return", "if", "else", "public", "static", "class", "true", "false", "null", "string", "bool", "int", "long"],
  json: ["true", "false", "null"],
  bash: ["export", "cd", "npm", "npx", "go", "cargo", "dotnet", "node"],
};
KEYWORDS.js = KEYWORDS.ts;

const COMMENT: Partial<Record<Lang, RegExp>> = {
  ts: /\/\/[^\n]*|\/\*[\s\S]*?\*\//y,
  js: /\/\/[^\n]*|\/\*[\s\S]*?\*\//y,
  go: /\/\/[^\n]*/y,
  csharp: /\/\/[^\n]*/y,
  bash: /#[^\n]*/y,
};

const STRING = /"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|`(?:[^`\\]|\\.)*`/y;
const NUMBER = /\b\d[\d_]*(?:\.\d+)?\b/y;
const WORD = /[A-Za-z_][\w-]*/y;

export function tokenize(code: string, lang: Lang): Token[] {
  if (lang === "text" || lang === "markdown") return [{ kind: "plain", text: code }];
  const out: Token[] = [];
  const keywords = new Set(KEYWORDS[lang] ?? []);
  const comment = COMMENT[lang];
  let plain = "";
  const flush = () => {
    if (plain) out.push({ kind: "plain", text: plain });
    plain = "";
  };
  const at = (re: RegExp, i: number) => {
    re.lastIndex = i;
    const m = re.exec(code);
    return m ? m[0] : null;
  };
  let i = 0;
  let lineStart = true;
  while (i < code.length) {
    // A shell prompt at the start of a line is shown but not copied.
    if (lang === "bash" && lineStart && code.startsWith("$ ", i)) {
      flush();
      out.push({ kind: "prompt", text: "$ " });
      i += 2;
      continue;
    }
    const c = comment && (lang !== "bash" || /\s/.test(code[i - 1] ?? " ")) ? at(comment, i) : null;
    if (c) {
      flush();
      out.push({ kind: "comment", text: c });
      i += c.length;
      continue;
    }
    const s = lang === "bash" ? at(/"(?:[^"\\\n]|\\.)*"|'[^'\n]*'/y, i) : at(STRING, i);
    if (s) {
      flush();
      out.push({ kind: "string", text: s });
      i += s.length;
      continue;
    }
    const prevWordChar = /[\w.-]/.test(code[i - 1] ?? "");
    const n = !prevWordChar ? at(NUMBER, i) : null;
    if (n) {
      flush();
      out.push({ kind: "number", text: n });
      i += n.length;
      continue;
    }
    const w = !prevWordChar ? at(WORD, i) : null;
    if (w) {
      if (keywords.has(w) && (lang !== "bash" || lineStart)) {
        flush();
        out.push({ kind: "keyword", text: w });
      } else plain += w;
      i += w.length;
      lineStart = false;
      continue;
    }
    plain += code[i];
    lineStart = code[i] === "\n" || (lineStart && code[i] === " ");
    i++;
  }
  flush();
  return out;
}

/** What the copy button puts on the clipboard: the code without shell prompts. */
export function copyText(code: string, lang: Lang): string {
  return lang === "bash" ? code.replace(/^\$ /gm, "") : code;
}
