import type { ReactNode } from "react";

/** Minimal Markdown for the receipt: headings, lists (1 level of nesting), tables, inline code, bold. */
function inline(s: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  for (const m of s.matchAll(/(`[^`]+`|\*\*[^*]+\*\*)/g)) {
    out.push(s.slice(last, m.index));
    const t = m[0];
    out.push(
      t.startsWith("`") ? (
        <code key={m.index} className="break-words rounded bg-muted px-1 py-0.5 font-mono text-[12px]">
          {t.slice(1, -1)}
        </code>
      ) : (
        <strong key={m.index}>{t.slice(2, -2)}</strong>
      ),
    );
    last = m.index! + t.length;
  }
  out.push(s.slice(last));
  return out;
}

export function Markdown({ text }: { text: string }) {
  const lines = text.split("\n");
  const blocks: ReactNode[] = [];
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!;
    if (line.startsWith("# ")) blocks.push(<h2 key={i} className="text-lg font-semibold">{inline(line.slice(2))}</h2>);
    else if (line.startsWith("## ")) blocks.push(<h3 key={i} className="mt-2 font-semibold">{inline(line.slice(3))}</h3>);
    else if (line.startsWith("|")) {
      const rows: string[][] = [];
      while (i < lines.length && lines[i]!.startsWith("|")) {
        const cells = lines[i]!.replace(/\\\|/g, "\u0000").split("|").slice(1, -1).map((c) => c.replace(/\u0000/g, "|").trim());
        if (!cells.every((c) => /^-+$/.test(c))) rows.push(cells);
        i++;
      }
      i--;
      const [head, ...body] = rows;
      blocks.push(
        <div key={i} className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b">{head!.map((c, k) => <th key={k} className="px-2 py-1.5 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">{inline(c)}</th>)}</tr>
            </thead>
            <tbody>
              {body.map((r, k) => (
                <tr key={k} className="border-b last:border-0">
                  {r.map((c, kk) => <td key={kk} className="px-2 py-1.5 align-top">{inline(c)}</td>)}
                </tr>
              ))}
            </tbody>
          </table>
        </div>,
      );
    } else if (/^\s*- /.test(line)) {
      const items: { sub: boolean; text: string }[] = [];
      while (i < lines.length && /^\s*- /.test(lines[i]!)) {
        items.push({ sub: /^\s{2,}/.test(lines[i]!), text: lines[i]!.replace(/^\s*- /, "") });
        i++;
      }
      i--;
      blocks.push(
        <ul key={i} className="grid gap-1 pl-4">
          {items.map((it, k) => (
            <li key={k} className={it.sub ? "ml-4 list-[circle] text-muted-foreground" : "list-disc"}>
              {inline(it.text)}
            </li>
          ))}
        </ul>,
      );
    } else if (line.trim()) blocks.push(<p key={i}>{inline(line.trimEnd())}</p>);
  }
  return <div className="grid gap-2 text-sm">{blocks}</div>;
}
