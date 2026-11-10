/**
 * The building blocks every docs page is written with: headings that feed the "On this page" outline,
 * code blocks with copy, language tabs that remember the reader's SDK, callouts and steps.
 */
import { Children, createContext, isValidElement, useContext, useEffect, useState, type ReactNode } from "react";
import { AlertTriangle, Check, Copy, Info, Lightbulb } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { copyText, tokenize, type Lang } from "@/lib/highlight";
import { cn } from "@/lib/utils";
import { href } from "@/lib/router";

export const slugify = (s: string) =>
  s
    .toLowerCase()
    .replace(/[`'"()?.,:/]/g, "")
    .trim()
    .replace(/\s+/g, "-");

const text = (n: ReactNode): string =>
  typeof n === "string" || typeof n === "number" ? String(n) : Array.isArray(n) ? n.map(text).join("") : isValidElement(n) ? text((n.props as { children?: ReactNode }).children) : "";

export function H1({ children }: { children: ReactNode }) {
  return <h1 className="scroll-m-20 text-3xl font-semibold tracking-tight sm:text-4xl">{children}</h1>;
}

export function Lead({ children }: { children: ReactNode }) {
  return <p className="mt-3 text-lg leading-relaxed text-muted-foreground">{children}</p>;
}

export function H2({ children, id }: { children: ReactNode; id?: string }) {
  const anchor = id ?? slugify(text(children));
  return (
    <h2 id={anchor} data-toc className="group mt-12 scroll-m-20 border-b pb-2 text-2xl font-semibold tracking-tight first:mt-0">
      <a href={`#${anchor}`} onClick={(e) => jump(e, anchor)} className="no-underline">
        {children}
        <span aria-hidden className="ml-2 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100">
          #
        </span>
      </a>
    </h2>
  );
}

export function H3({ children, id }: { children: ReactNode; id?: string }) {
  const anchor = id ?? slugify(text(children));
  return (
    <h3 id={anchor} data-toc data-level="3" className="mt-8 scroll-m-20 text-lg font-semibold tracking-tight">
      {children}
    </h3>
  );
}

/** In-page anchors live after the route in the hash (`#/cli/overview` keeps working), so scroll by hand. */
function jump(e: React.MouseEvent, anchor: string) {
  e.preventDefault();
  document.getElementById(anchor)?.scrollIntoView({ behavior: "smooth", block: "start" });
  history.replaceState(null, "", `${location.hash.split("~")[0]}~${anchor}`);
}

export function P({ children, className }: { children: ReactNode; className?: string }) {
  return <p className={cn("leading-7 [&:not(:first-child)]:mt-5", className)}>{children}</p>;
}

export function C({ children }: { children: ReactNode }) {
  return <code className="relative rounded bg-muted px-[0.3rem] py-[0.15rem] font-mono text-[0.85em]">{children}</code>;
}

export function A({ to, children }: { to: string; children: ReactNode }) {
  const external = /^https?:/.test(to);
  return (
    <a
      href={external ? to : href(to)}
      {...(external ? { target: "_blank", rel: "noreferrer" } : {})}
      className="font-medium text-foreground underline decoration-brass/60 underline-offset-4 hover:decoration-brass"
    >
      {children}
    </a>
  );
}

export function Ul({ children }: { children: ReactNode }) {
  return <ul className="my-5 ml-6 list-disc leading-7 [&>li]:mt-2">{children}</ul>;
}

export function Ol({ children }: { children: ReactNode }) {
  return <ol className="my-5 ml-6 list-decimal leading-7 [&>li]:mt-2">{children}</ol>;
}

// ---------------------------------------------------------------------------
// Code
// ---------------------------------------------------------------------------

function CopyButton({ value }: { value: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      aria-label={done ? "Copied" : "Copy code"}
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(value);
        } catch {
          /* clipboard blocked (http, iframe): the code is still selectable */
        }
        setDone(true);
        setTimeout(() => setDone(false), 1500);
      }}
      className="inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-background hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      {done ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
    </button>
  );
}

export function Code({ code, lang = "text", title, className, wrap = lang === "markdown" }: { code: string; lang?: Lang; title?: string; className?: string; wrap?: boolean }) {
  const body = code.replace(/\n+$/, "");
  return (
    <figure data-code className={cn("group relative my-5 overflow-hidden rounded-lg border bg-muted/50", className)}>
      <figcaption className="flex h-9 items-center justify-between border-b bg-muted/60 pl-4 pr-1.5 text-xs text-muted-foreground">
        <span className="truncate font-mono">{title ?? lang}</span>
        <CopyButton value={copyText(body, lang)} />
      </figcaption>
      <pre className={cn("overflow-x-auto p-4 font-mono text-[13px] leading-6 [tab-size:4]", (lang === "markdown" || lang === "text") && wrap && "whitespace-pre-wrap break-words")}>
        <code>
          {tokenize(body, lang).map((t, i) =>
            t.kind === "plain" ? (
              t.text
            ) : (
              <span key={i} className={`tok-${t.kind}`}>
                {t.text}
              </span>
            ),
          )}
        </code>
      </pre>
    </figure>
  );
}

/** One tab per SDK. The reader's choice is shared by every tab group and remembered. */
const LANG_KEY = "lastro-docs-sdk";
const SdkContext = createContext<{ sdk: string; setSdk: (s: string) => void }>({ sdk: "TypeScript", setSdk: () => {} });

export function SdkProvider({ children }: { children: ReactNode }) {
  const [sdk, set] = useState(() => {
    try {
      return localStorage.getItem(LANG_KEY) ?? "TypeScript";
    } catch {
      return "TypeScript";
    }
  });
  const setSdk = (s: string) => {
    set(s);
    try {
      localStorage.setItem(LANG_KEY, s);
    } catch {
      /* private mode: the choice lasts for this visit */
    }
  };
  return <SdkContext.Provider value={{ sdk, setSdk }}>{children}</SdkContext.Provider>;
}

export interface Variant {
  label: string;
  lang: Lang;
  code: string;
  title?: string;
}

export function CodeTabs({ variants }: { variants: Variant[] }) {
  const { sdk, setSdk } = useContext(SdkContext);
  const value = variants.some((v) => v.label === sdk) ? sdk : variants[0]!.label;
  return (
    <Tabs value={value} onValueChange={setSdk} className="my-5">
      <TabsList aria-label="Language">
        {variants.map((v) => (
          <TabsTrigger key={v.label} value={v.label}>
            {v.label}
          </TabsTrigger>
        ))}
      </TabsList>
      {variants.map((v) => (
        <TabsContent key={v.label} value={v.label} className="mt-0">
          <Code code={v.code} lang={v.lang} title={v.title} className="mt-2" />
        </TabsContent>
      ))}
    </Tabs>
  );
}

// ---------------------------------------------------------------------------
// Callouts, steps, tables
// ---------------------------------------------------------------------------

const CALLOUT = {
  note: { icon: Info, variant: "default" as const, title: "Note" },
  tip: { icon: Lightbulb, variant: "success" as const, title: "Tip" },
  warning: { icon: AlertTriangle, variant: "warning" as const, title: "Careful" },
};

export function Callout({ kind = "note", title, children }: { kind?: keyof typeof CALLOUT; title?: string; children: ReactNode }) {
  const c = CALLOUT[kind];
  const Icon = c.icon;
  return (
    <Alert variant={c.variant} className="my-6 bg-card">
      <Icon className="h-4 w-4" />
      <AlertTitle>{title ?? c.title}</AlertTitle>
      <AlertDescription className="text-foreground/90 [&_p]:mt-0">{children}</AlertDescription>
    </Alert>
  );
}

/** Numbered steps with a rail, like shadcn's docs. Each child is one <Step>. */
export function Steps({ children }: { children: ReactNode }) {
  return (
    <div className="my-6 ml-3.5 border-l pl-7 [counter-reset:step]">
      {Children.map(children, (c) => c)}
    </div>
  );
}

export function Step({ title, children }: { title: string; children: ReactNode }) {
  const anchor = slugify(title);
  return (
    <section className="relative pb-6 last:pb-0 [counter-increment:step]">
      <span
        aria-hidden
        className="absolute -left-[2.6rem] top-0 flex h-7 w-7 items-center justify-center rounded-full border bg-background font-mono text-xs font-medium before:content-[counter(step)]"
      />
      <h3 id={anchor} data-toc data-level="3" className="scroll-m-20 font-semibold leading-7 tracking-tight">
        {title}
      </h3>
      <div className="[&>*:first-child]:mt-2">{children}</div>
    </section>
  );
}

export function DataTable({ head, rows }: { head: ReactNode[]; rows: ReactNode[][] }) {
  return (
    <div className="my-6 overflow-hidden rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow className="bg-muted/50 hover:bg-muted/50">
            {head.map((h, i) => (
              <TableHead key={i}>{h}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((r, i) => (
            <TableRow key={i}>
              {r.map((c, j) => (
                <TableCell key={j} className="align-top leading-6">
                  {c}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

/** Scroll to `~anchor` after a page renders (deep links into a section). */
export function useAnchorScroll(dep: string) {
  useEffect(() => {
    const anchor = location.hash.split("~")[1];
    if (anchor) requestAnimationFrame(() => document.getElementById(anchor)?.scrollIntoView({ block: "start" }));
    else window.scrollTo(0, 0);
  }, [dep]);
}
