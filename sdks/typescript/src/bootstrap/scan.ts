/**
 * The repository digest (Node only): what `lastro bootstrap` reads before the
 * survey step, computed without any LLM.
 *
 *   files     every tracked file, with its role, language, line count and exported symbols
 *   tests     every test found by name, per file (the closest thing to written-down behavior)
 *   commits   the newest commits with their subjects and touched files (why the code is the way it is)
 *   manifests package.json, go.mod, Cargo.toml, *.csproj, pyproject.toml: what the repo ships
 *   areas     the folders a large repository can be bootstrapped one at a time
 *
 * The digest is deterministic for a given checkout and options: files and tests are
 * sorted, commits come from `git log` in order, and its hash binds the survey's
 * answer to it (every file, test and commit the answer cites must be in it).
 */
import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, posix } from "node:path";
import { digestHash, type RepoArea, type RepoCommit, type RepoDigest, type RepoFile, type RepoManifest, type RepoTest } from "../bootstrap.js";

export interface ScanOptions {
  /** Only these path prefixes (folders or files), relative to the root. Default: the whole repository. */
  scope?: string[];
  /** Newest commits to read (default 400). 0 skips the history. */
  maxCommits?: number;
  /** Only commits after this date (anything `git log --since` accepts). */
  since?: string;
  /** Files to list (default 5000); the digest says when it was cut. */
  maxFiles?: number;
}

export const DEFAULT_MAX_COMMITS = 400;
export const DEFAULT_MAX_FILES = 5000;
const MAX_READ_BYTES = 512 * 1024;
const MAX_SYMBOLS = 30;
const MAX_FILES_PER_COMMIT = 25;

/** Folders never worth reading when the repository is not a git checkout. */
const SKIP_DIRS = new Set([".git", ".lastro", "node_modules", "dist", "build", "out", "target", "bin", "obj", "vendor", ".venv", "venv", "__pycache__", ".next", "coverage"]);

const LANGUAGES: Record<string, string> = {
  ts: "typescript", tsx: "typescript", mts: "typescript", cts: "typescript",
  js: "javascript", jsx: "javascript", mjs: "javascript", cjs: "javascript",
  go: "go", rs: "rust", cs: "csharp", fs: "fsharp", py: "python", java: "java", kt: "kotlin",
  rb: "ruby", php: "php", swift: "swift", c: "c", h: "c", cc: "cpp", cpp: "cpp", hpp: "cpp",
  scala: "scala", ex: "elixir", exs: "elixir", sql: "sql", sh: "shell", vue: "vue", svelte: "svelte",
};
const DOC_EXT = new Set(["md", "mdx", "rst", "txt", "adoc"]);
const CONFIG_EXT = new Set(["json", "yaml", "yml", "toml", "ini", "lock", "xml", "csproj", "sln", "props", "mod", "sum", "cfg", "env"]);

const ext = (p: string) => (p.includes(".") ? p.slice(p.lastIndexOf(".") + 1).toLowerCase() : "");

export function roleOf(path: string): RepoFile["role"] {
  const e = ext(path);
  const base = posix.basename(path);
  if (
    /(^|\/)(test|tests|__tests__|spec|specs|e2e|testdata)\//i.test(path) ||
    /\.(test|spec)\.[cm]?[jt]sx?$/.test(base) ||
    /_test\.(go|py)$/.test(base) ||
    /^test_.*\.py$/.test(base) ||
    /Tests?\.cs$/.test(base)
  )
    return LANGUAGES[e] ? "test" : "other";
  if (LANGUAGES[e]) return "source";
  if (DOC_EXT.has(e) || /^(readme|changelog|contributing|license)/i.test(base)) return "doc";
  if (CONFIG_EXT.has(e) || /^(dockerfile|makefile|\.[a-z]+rc)$/i.test(base)) return "config";
  return "other";
}

const SYMBOL_PATTERNS: Record<string, RegExp[]> = {
  typescript: [/^export\s+(?:default\s+)?(?:declare\s+)?(?:async\s+)?(?:function\*?|class|const|let|var|interface|type|enum|abstract\s+class)\s+([A-Za-z_$][\w$]*)/gm],
  javascript: [/^export\s+(?:default\s+)?(?:async\s+)?(?:function\*?|class|const|let|var)\s+([A-Za-z_$][\w$]*)/gm, /^module\.exports\.([A-Za-z_$][\w$]*)\s*=/gm],
  go: [/^func\s+(?:\([^)]*\)\s*)?([A-Z]\w*)/gm, /^type\s+([A-Z]\w*)/gm],
  rust: [/^\s*pub\s+(?:async\s+)?(?:fn|struct|enum|trait|type)\s+(\w+)/gm],
  csharp: [/^\s*public\s+(?:static\s+|sealed\s+|abstract\s+|partial\s+|readonly\s+)*(?:class|record|interface|enum|struct)\s+(\w+)/gm, /^\s*public\s+(?:static\s+|async\s+|override\s+|virtual\s+)*[\w<>[\],.? ]+\s+(\w+)\s*\(/gm],
  python: [/^def\s+([A-Za-z]\w*)/gm, /^class\s+([A-Za-z]\w*)/gm],
  java: [/^\s*public\s+(?:final\s+|abstract\s+)*(?:class|interface|enum|record)\s+(\w+)/gm],
  kotlin: [/^\s*(?:data\s+|sealed\s+|open\s+)?(?:class|interface|object|fun)\s+(\w+)/gm],
};

const TEST_PATTERNS: Record<string, RegExp[]> = {
  typescript: [/\b(?:it|test)(?:\.only|\.skip)?\s*\(\s*(["'`])((?:(?!\1).){1,200})\1/g],
  javascript: [/\b(?:it|test)(?:\.only|\.skip)?\s*\(\s*(["'`])((?:(?!\1).){1,200})\1/g],
  go: [/^func\s+(Test\w+)\s*\(/gm],
  rust: [/#\[(?:tokio::)?test\]\s*(?:#\[[^\]]*\]\s*)*(?:async\s+)?fn\s+(\w+)/g],
  csharp: [/\[(?:Fact|Theory|Test|TestMethod|TestCase)[^\]]*\]\s*(?:\[[^\]]*\]\s*)*public\s+(?:async\s+)?[\w<>]+\s+(\w+)\s*\(/g],
  python: [/^\s*(?:async\s+)?def\s+(test_\w+)/gm],
  java: [/@Test\s*(?:@\w+(?:\([^)]*\))?\s*)*(?:public\s+)?void\s+(\w+)\s*\(/g],
  kotlin: [/@Test\s*fun\s+`?([^`(]+)`?\s*\(/g],
};

function matches(text: string, patterns: RegExp[] | undefined, group: (m: RegExpExecArray) => string): string[] {
  const out: string[] = [];
  for (const re of patterns ?? []) {
    re.lastIndex = 0;
    let m: RegExpExecArray | null;
    while ((m = re.exec(text))) out.push(group(m).trim());
  }
  return out;
}

export function symbolsOf(language: string | undefined, text: string): string[] {
  if (!language) return [];
  return [...new Set(matches(text, SYMBOL_PATTERNS[language], (m) => m[1]!))].slice(0, MAX_SYMBOLS);
}

export function testsOf(language: string | undefined, text: string): string[] {
  if (!language) return [];
  return [...new Set(matches(text, TEST_PATTERNS[language], (m) => m[m.length - 1]!))];
}

function git(root: string, args: string[]): string | undefined {
  try {
    return execFileSync("git", ["-C", root, ...args], { encoding: "utf8", maxBuffer: 256 * 1024 * 1024, stdio: ["ignore", "pipe", "ignore"] });
  } catch {
    return undefined;
  }
}

function walk(root: string, dir = ""): string[] {
  const out: string[] = [];
  for (const name of readdirSync(join(root, dir)).sort()) {
    const rel = dir ? `${dir}/${name}` : name;
    const st = statSync(join(root, rel));
    if (st.isDirectory()) {
      if (!SKIP_DIRS.has(name) && !name.startsWith(".")) out.push(...walk(root, rel));
    } else if (st.isFile()) out.push(rel);
  }
  return out;
}

const inScope = (scope: string[], path: string) => !scope.length || scope.some((s) => path === s || path.startsWith(s.endsWith("/") ? s : `${s}/`));

function normalizeScope(scope: string[] | undefined): string[] {
  return [...new Set((scope ?? []).map((s) => s.replace(/\\/g, "/").replace(/^\.\//, "").replace(/\/+$/, "")).filter((s) => s && s !== "."))].sort();
}

function manifestOf(path: string, text: string): RepoManifest | undefined {
  const base = posix.basename(path);
  try {
    if (base === "package.json") {
      const j = JSON.parse(text) as { name?: string; bin?: string | Record<string, string>; main?: string; scripts?: Record<string, string>; workspaces?: unknown };
      const bins = typeof j.bin === "string" ? [j.name ?? j.bin] : Object.keys(j.bin ?? {});
      return { path, kind: "npm", ...(j.name ? { name: j.name } : {}), entries: [...(j.main ? [j.main] : []), ...bins.map((b) => `bin:${b}`)], scripts: Object.keys(j.scripts ?? {}).sort() };
    }
    if (base === "go.mod") return { path, kind: "go", name: text.match(/^module\s+(\S+)/m)?.[1] ?? "", entries: [], scripts: [] };
    if (base === "Cargo.toml") {
      const bins = [...text.matchAll(/\[\[bin\]\][^[]*?name\s*=\s*"([^"]+)"/g)].map((m) => `bin:${m[1]}`);
      const name = text.match(/\[package\][^[]*?name\s*=\s*"([^"]+)"/)?.[1];
      return { path, kind: "cargo", ...(name ? { name } : {}), entries: bins, scripts: [] };
    }
    if (base.endsWith(".csproj")) {
      const exe = /<OutputType>\s*Exe\s*<\/OutputType>/i.test(text) ? ["exe"] : [];
      const tool = text.match(/<ToolCommandName>\s*([^<]+)</i)?.[1];
      return { path, kind: "dotnet", name: text.match(/<AssemblyName>\s*([^<]+)</i)?.[1] ?? base.replace(/\.csproj$/, ""), entries: [...exe, ...(tool ? [`bin:${tool.trim()}`] : [])], scripts: [] };
    }
    if (base === "pyproject.toml") {
      const scripts = text.match(/\[project\.scripts\]([^[]*)/)?.[1] ?? "";
      return { path, kind: "python", name: text.match(/^name\s*=\s*"([^"]+)"/m)?.[1] ?? "", entries: [...scripts.matchAll(/^\s*([\w.-]+)\s*=/gm)].map((m) => `bin:${m[1]}`), scripts: [] };
    }
  } catch {
    return { path, kind: "unknown", entries: [], scripts: [] };
  }
  return undefined;
}

/** The folder a file belongs to for bootstrapping one part at a time: its nearest manifest folder, else its first folder. */
function areaOf(path: string, manifestDirs: string[]): string {
  const dirs = manifestDirs.filter((d) => d && (path.startsWith(`${d}/`))).sort((a, b) => b.length - a.length);
  if (dirs[0]) return dirs[0];
  return path.includes("/") ? path.slice(0, path.indexOf("/")) : ".";
}

/** Read the repository at `root` into a digest. Deterministic for a given checkout and options. */
export function scanRepo(root: string, opts: ScanOptions = {}): RepoDigest {
  const scope = normalizeScope(opts.scope);
  const maxFiles = opts.maxFiles ?? DEFAULT_MAX_FILES;
  const maxCommits = opts.maxCommits ?? DEFAULT_MAX_COMMITS;
  const tracked = git(root, ["ls-files", "-z"]);
  const isGit = tracked !== undefined;
  const allPaths = (isGit ? tracked!.split("\0").filter(Boolean) : walk(root)).filter((p) => !p.startsWith(".lastro/") && existsSync(join(root, p))).sort();
  const scoped = allPaths.filter((p) => inScope(scope, p));
  for (const s of scope) if (!allPaths.some((p) => inScope([s], p))) throw new Error(`scope "${s}" matches no file`);

  const files: RepoFile[] = [];
  const tests: RepoTest[] = [];
  const manifests: RepoManifest[] = [];
  for (const path of scoped.slice(0, maxFiles)) {
    const role = roleOf(path);
    const language = LANGUAGES[ext(path)];
    const st = statSync(join(root, path));
    let text = "";
    if (st.size <= MAX_READ_BYTES) {
      const buf = readFileSync(join(root, path));
      if (!buf.subarray(0, 8000).includes(0)) text = buf.toString("utf8");
    }
    const lines = text ? text.split("\n").length : 0;
    const symbols = role === "source" ? symbolsOf(language, text) : [];
    files.push({ path, role, ...(language ? { language } : {}), lines, symbols });
    if (role === "test") for (const name of testsOf(language, text)) tests.push({ id: `${path}::${name}`, file: path, name });
    const m = text ? manifestOf(path, text) : undefined;
    if (m) manifests.push(m);
  }

  const commits: RepoCommit[] = [];
  let totalCommits = 0;
  let head: string | null = null;
  if (isGit) {
    head = git(root, ["rev-parse", "HEAD"])?.trim() || null;
    totalCommits = Number(git(root, ["rev-list", "--count", "HEAD", ...(opts.since ? [`--since=${opts.since}`] : []), ...(scope.length ? ["--", ...scope] : [])])?.trim() ?? 0) || 0;
    if (maxCommits > 0 && head) {
      const log = git(root, ["log", `-n`, String(maxCommits), "--no-renames", "--name-only", "--format=%x1e%H%x1f%aI%x1f%s%x1f%b%x1f", ...(opts.since ? [`--since=${opts.since}`] : []), ...(scope.length ? ["--", ...scope] : [])]) ?? "";
      for (const rec of log.split("\x1e").slice(1)) {
        const [sha, date, subject, body, rest] = rec.split("\x1f");
        const touched = (rest ?? "").split("\n").map((x) => x.trim()).filter((x) => x && inScope(scope, x));
        const firstBody = (body ?? "").split("\n").map((x) => x.trim()).filter((x) => x && !/^(co-authored-by|signed-off-by|claude-session):/i.test(x)).slice(0, 3).join(" ");
        commits.push({
          sha: sha!.trim(),
          date: date!.trim(),
          subject: subject!.trim(),
          ...(firstBody ? { body: firstBody.slice(0, 300) } : {}),
          files: touched.slice(0, MAX_FILES_PER_COMMIT),
          ...(touched.length > MAX_FILES_PER_COMMIT ? { moreFiles: touched.length - MAX_FILES_PER_COMMIT } : {}),
        });
      }
    }
  }

  const manifestDirs = manifests.map((m) => posix.dirname(m.path)).map((d) => (d === "." ? "" : d));
  const areaMap = new Map<string, RepoArea>();
  const area = (path: string) => {
    const a = areaOf(path, manifestDirs);
    if (!areaMap.has(a)) areaMap.set(a, { path: a, files: 0, lines: 0, tests: 0, commits: 0 });
    return areaMap.get(a)!;
  };
  for (const f of files) {
    const a = area(f.path);
    a.files++;
    a.lines += f.lines;
  }
  for (const t of tests) area(t.file).tests++;
  for (const c of commits) for (const a of new Set(c.files.map((f) => areaOf(f, manifestDirs)))) if (areaMap.has(a)) areaMap.get(a)!.commits++;

  const core: Omit<RepoDigest, "hash"> = {
    lastro: "repo-digest/v1",
    head,
    scope,
    totals: { files: scoped.length, commits: totalCommits },
    truncated: { files: scoped.length > maxFiles, commits: totalCommits > commits.length },
    ...(opts.since ? { since: opts.since } : {}),
    manifests,
    areas: [...areaMap.values()].sort((a, b) => a.path.localeCompare(b.path)),
    files,
    tests,
    commits,
  };
  return { ...core, hash: digestHash(core) };
}

/**
 * Source text for the survey prompt, within a character budget: manifests and
 * READMEs first, then tests (written-down behavior), then source files with the
 * most exported symbols. Deterministic for a given digest and budget.
 */
export function excerptsOf(root: string, digest: RepoDigest, budget = 120_000, perFile = 8_000): { path: string; text: string; truncated: boolean }[] {
  const manifestPaths = new Set(digest.manifests.map((m) => m.path));
  const rank = (f: RepoFile) => (manifestPaths.has(f.path) ? 0 : /(^|\/)readme(\.\w+)?$/i.test(f.path) ? 1 : f.role === "test" ? 2 : f.role === "source" ? 3 : 9);
  const order = digest.files
    .filter((f) => rank(f) < 9 && f.lines > 0)
    .sort((a, b) => rank(a) - rank(b) || (rank(a) === 3 ? b.symbols.length - a.symbols.length : 0) || a.path.localeCompare(b.path));
  const out: { path: string; text: string; truncated: boolean }[] = [];
  let left = budget;
  for (const f of order) {
    if (left <= 0) break;
    let text: string;
    try {
      text = readFileSync(join(root, f.path), "utf8");
    } catch {
      continue;
    }
    const cap = Math.min(perFile, left);
    out.push({ path: f.path, text: text.slice(0, cap), truncated: text.length > cap });
    left -= Math.min(text.length, cap);
  }
  return out;
}
