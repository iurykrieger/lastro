import type { ReactNode } from "react";

/** Inline Markdown: `code`, **bold**, *emphasis*. */
export function inline(s: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  for (const m of s.matchAll(/(`[^`]+`|\*\*[^*]+\*\*|\*[^*\s][^*]*\*)/g)) {
    out.push(s.slice(last, m.index));
    const t = m[0];
    out.push(
      t.startsWith("`") ? (
        <code key={m.index} className="break-words rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]">
          {t.slice(1, -1)}
        </code>
      ) : t.startsWith("**") ? (
        <strong key={m.index}>{t.slice(2, -2)}</strong>
      ) : (
        <em key={m.index}>{t.slice(1, -1)}</em>
      ),
    );
    last = m.index! + t.length;
  }
  out.push(s.slice(last));
  return out;
}

/** The Markdown a proposal's prose uses: headings, paragraphs, lists, tables and code fences. */
export function Markdown({ text }: { text: string }) {
  const lines = text.split("\n");
  const blocks: ReactNode[] = [];
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!;
    const h = line.match(/^(#{1,6})\s+(.*)$/);
    if (h) {
      const level = h[1]!.length;
      const cls = level <= 1 ? "text-xl font-semibold" : level === 2 ? "mt-3 text-lg font-semibold" : "mt-2 font-semibold";
      blocks.push(
        <p key={i} role="heading" aria-level={Math.min(level + 2, 6)} className={cls}>
          {inline(h[2]!)}
        </p>,
      );
    } else if (line.startsWith("```")) {
      const body: string[] = [];
      i++;
      while (i < lines.length && !lines[i]!.startsWith("```")) body.push(lines[i++]!);
      blocks.push(
        <pre key={i} className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs leading-relaxed">
          {body.join("\n")}
        </pre>,
      );
    } else if (line.startsWith("|")) {
      const rows: string[][] = [];
      while (i < lines.length && lines[i]!.startsWith("|")) {
        const cells = lines[i]!.replace(/\\\|/g, "\u0000").split("|").slice(1, -1).map((c) => c.replace(/\u0000/g, "|").trim());
        if (!cells.every((c) => /^:?-+:?$/.test(c))) rows.push(cells);
        i++;
      }
      i--;
      const [head, ...body] = rows;
      blocks.push(
        <div key={i} className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b">
                {head!.map((c, k) => (
                  <th key={k} className="px-2 py-1.5 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {inline(c)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {body.map((r, k) => (
                <tr key={k} className="border-b last:border-0">
                  {r.map((c, kk) => (
                    <td key={kk} className="px-2 py-1.5 align-top">
                      {inline(c)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>,
      );
    } else if (/^\s*([-*]|\d+\.) /.test(line)) {
      const items: { sub: boolean; text: string }[] = [];
      const ordered = /^\s*\d+\. /.test(line);
      while (i < lines.length && (/^\s*([-*]|\d+\.) /.test(lines[i]!) || (/^\s{2,}\S/.test(lines[i]!) && items.length))) {
        const l = lines[i]!;
        if (/^\s*([-*]|\d+\.) /.test(l)) items.push({ sub: /^\s{2,}/.test(l), text: l.replace(/^\s*([-*]|\d+\.) /, "") });
        else items[items.length - 1]!.text += ` ${l.trim()}`;
        i++;
      }
      i--;
      const List = ordered ? "ol" : "ul";
      blocks.push(
        <List key={i} className={`grid gap-1 pl-5 ${ordered ? "list-decimal" : "list-disc"}`}>
          {items.map((it, k) => (
            <li key={k} className={it.sub ? "ml-4 list-[circle]" : undefined}>
              {inline(it.text)}
            </li>
          ))}
        </List>,
      );
    } else if (line.trim()) {
      // A paragraph runs until a blank line or another block.
      const para = [line.trim()];
      while (i + 1 < lines.length && lines[i + 1]!.trim() && !/^(#{1,6}\s|```|\||\s*([-*]|\d+\.) )/.test(lines[i + 1]!)) para.push(lines[++i]!.trim());
      blocks.push(<p key={i}>{inline(para.join(" "))}</p>);
    }
  }
  return <div className="grid gap-2 text-sm leading-relaxed">{blocks}</div>;
}
