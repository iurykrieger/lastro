import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ArrowLeft, ArrowRight, BookOpen, FlaskConical, Github, Menu, Moon, Search, Sun, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { NAV, PAGES, sectionOf, type NavPage } from "@/content/nav";
import { href, navigate } from "@/lib/router";
import { cn } from "@/lib/utils";

const REPO = "https://github.com/iurykrieger/lastro";

// ---------------------------------------------------------------------------
// Theme: follows the OS until the reader picks one.
// ---------------------------------------------------------------------------

function useTheme() {
  const [theme, setTheme] = useState<"light" | "dark" | null>(() => {
    try {
      return (localStorage.getItem("lastro-docs-theme") as "light" | "dark" | null) ?? null;
    } catch {
      return null;
    }
  });
  useEffect(() => {
    if (theme) document.documentElement.dataset.theme = theme;
    else delete document.documentElement.dataset.theme;
  }, [theme]);
  const dark = theme ? theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
  const toggle = () => {
    const next = dark ? "light" : "dark";
    setTheme(next);
    try {
      localStorage.setItem("lastro-docs-theme", next);
    } catch {
      /* not persisted */
    }
  };
  return { dark, toggle };
}

// ---------------------------------------------------------------------------
// Sidebar
// ---------------------------------------------------------------------------

function SidebarNav({ slug, onPick }: { slug: string; onPick?: () => void }) {
  return (
    <nav aria-label="Documentation" className="grid gap-6 text-sm">
      {NAV.map((s) => (
        <div key={s.title}>
          <h4 className="mb-1.5 px-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">{s.title}</h4>
          <ul className="grid gap-0.5">
            {s.pages.map((p) => (
              <li key={p.slug}>
                <a
                  href={href(p.slug)}
                  onClick={onPick}
                  aria-current={p.slug === slug ? "page" : undefined}
                  className={cn(
                    "block rounded-md px-2 py-1.5 transition-colors hover:bg-accent hover:text-accent-foreground",
                    p.slug === slug ? "bg-accent font-medium text-foreground" : "text-muted-foreground",
                  )}
                >
                  {p.title}
                </a>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </nav>
  );
}

// ---------------------------------------------------------------------------
// Search: titles, descriptions and keywords of every page. Ctrl/Cmd+K or "/".
// ---------------------------------------------------------------------------

function score(p: NavPage, q: string): number {
  const words = q.toLowerCase().split(/\s+/).filter(Boolean);
  const title = p.title.toLowerCase();
  const rest = `${p.description} ${p.keywords ?? ""} ${p.slug}`.toLowerCase();
  let s = 0;
  for (const w of words) {
    if (title.includes(w)) s += 3;
    else if (rest.includes(w)) s += 1;
    else return 0;
  }
  return s;
}

function SearchDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [q, setQ] = useState("");
  const [active, setActive] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const results = useMemo(
    () =>
      q.trim()
        ? PAGES.map((p) => ({ p, s: score(p, q) }))
            .filter((x) => x.s > 0)
            .sort((a, b) => b.s - a.s)
            .map((x) => x.p)
        : PAGES,
    [q],
  );
  useEffect(() => {
    if (open) {
      setQ("");
      setActive(0);
      requestAnimationFrame(() => input.current?.focus());
    }
  }, [open]);
  useEffect(() => setActive(0), [q]);
  if (!open) return null;
  const go = (p: NavPage | undefined) => {
    if (!p) return;
    navigate(p.slug);
    onClose();
  };
  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 px-4 pt-[12vh] backdrop-blur-sm" onMouseDown={onClose}>
      <div role="dialog" aria-modal="true" aria-label="Search the docs" className="w-full max-w-lg overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-2xl" onMouseDown={(e) => e.stopPropagation()}>
        <div className="flex items-center gap-2 border-b px-3">
          <Search className="h-4 w-4 shrink-0 text-muted-foreground" />
          <input
            ref={input}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") onClose();
              else if (e.key === "ArrowDown") {
                e.preventDefault();
                setActive((a) => Math.min(a + 1, results.length - 1));
              } else if (e.key === "ArrowUp") {
                e.preventDefault();
                setActive((a) => Math.max(a - 1, 0));
              } else if (e.key === "Enter") go(results[active]);
            }}
            placeholder="Search pages…"
            aria-label="Search pages"
            className="h-12 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
          <kbd className="rounded border bg-muted px-1.5 font-mono text-[10px] text-muted-foreground">Esc</kbd>
        </div>
        <ul role="listbox" aria-label="Results" className="max-h-[50vh] overflow-y-auto p-1.5">
          {results.length === 0 && <li className="px-3 py-6 text-center text-sm text-muted-foreground">No page matches “{q}”.</li>}
          {results.map((p, i) => (
            <li key={p.slug} role="option" aria-selected={i === active}>
              <button
                type="button"
                onMouseEnter={() => setActive(i)}
                onClick={() => go(p)}
                className={cn("grid w-full gap-0.5 rounded-md px-3 py-2 text-left", i === active && "bg-accent")}
              >
                <span className="text-sm font-medium">{p.title}</span>
                <span className="text-xs text-muted-foreground">
                  {sectionOf(p.slug)?.title} · {p.description}
                </span>
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// "On this page": the h2/h3 headings of the rendered page.
// ---------------------------------------------------------------------------

function Toc({ slug }: { slug: string }) {
  const [items, setItems] = useState<{ id: string; text: string; level: number }[]>([]);
  const [current, setCurrent] = useState<string | null>(null);
  useEffect(() => {
    const hs = Array.from(document.querySelectorAll<HTMLElement>("main [data-toc]"));
    setItems(hs.map((h) => ({ id: h.id, text: h.textContent?.replace(/#$/, "").trim() ?? "", level: h.dataset.level === "3" ? 3 : 2 })));
    const io = new IntersectionObserver(
      (es) => {
        const visible = es.filter((e) => e.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible[0]) setCurrent(visible[0].target.id);
      },
      { rootMargin: "-80px 0px -70% 0px" },
    );
    hs.forEach((h) => io.observe(h));
    return () => io.disconnect();
  }, [slug]);
  if (items.length < 2) return null;
  return (
    <nav aria-label="On this page" className="text-sm">
      <h4 className="mb-2 font-medium">On this page</h4>
      <ul className="grid gap-1.5">
        {items.map((it) => (
          <li key={it.id} className={it.level === 3 ? "pl-3" : ""}>
            <a
              href={`${href(slug)}~${it.id}`}
              onClick={(e) => {
                e.preventDefault();
                document.getElementById(it.id)?.scrollIntoView({ behavior: "smooth", block: "start" });
                history.replaceState(null, "", `${href(slug)}~${it.id}`);
              }}
              className={cn("block leading-snug transition-colors hover:text-foreground", current === it.id ? "font-medium text-foreground" : "text-muted-foreground")}
            >
              {it.text}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function Pager({ slug }: { slug: string }) {
  const i = PAGES.findIndex((p) => p.slug === slug);
  const prev = i > 0 ? PAGES[i - 1] : undefined;
  const next = i >= 0 && i < PAGES.length - 1 ? PAGES[i + 1] : undefined;
  return (
    <nav aria-label="Pagination" className="mt-16 flex items-center justify-between gap-4 border-t pt-6">
      {prev ? (
        <a href={href(prev.slug)} rel="prev" className="group grid gap-0.5 text-sm">
          <span className="flex items-center gap-1 text-muted-foreground">
            <ArrowLeft className="h-3.5 w-3.5" /> Previous
          </span>
          <span className="font-medium group-hover:underline">{prev.title}</span>
        </a>
      ) : (
        <span />
      )}
      {next && (
        <a href={href(next.slug)} rel="next" className="group grid gap-0.5 text-right text-sm">
          <span className="flex items-center justify-end gap-1 text-muted-foreground">
            Next <ArrowRight className="h-3.5 w-3.5" />
          </span>
          <span className="font-medium group-hover:underline">{next.title}</span>
        </a>
      )}
    </nav>
  );
}

// ---------------------------------------------------------------------------
// Shell
// ---------------------------------------------------------------------------

export function Layout({ slug, children }: { slug: string; children: ReactNode }) {
  const { dark, toggle } = useTheme();
  const [menu, setMenu] = useState(false);
  const [search, setSearch] = useState(false);

  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      const typing = e.target instanceof HTMLElement && /^(INPUT|TEXTAREA)$/.test(e.target.tagName);
      if ((e.key === "k" && (e.metaKey || e.ctrlKey)) || (e.key === "/" && !typing)) {
        e.preventDefault();
        setSearch(true);
      }
    };
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  }, []);
  useEffect(() => setMenu(false), [slug]);

  return (
    <div className="min-h-screen">
      <a href="#content" onClick={(e) => (e.preventDefault(), document.getElementById("content")?.focus())} className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-3 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2">
        Skip to content
      </a>
      <header className="sticky top-0 z-40 border-b bg-background/85 backdrop-blur supports-[backdrop-filter]:bg-background/70">
        <div className="mx-auto flex h-14 max-w-[1400px] items-center gap-2 px-4 sm:px-6">
          <Button variant="ghost" size="icon" className="lg:hidden" aria-label={menu ? "Close menu" : "Open menu"} aria-expanded={menu} onClick={() => setMenu((m) => !m)}>
            {menu ? <X /> : <Menu />}
          </Button>
          <a href={href("introduction")} className="mr-4 flex items-center gap-2 font-semibold tracking-tight">
            <span aria-hidden className="h-5 w-5 rounded bg-gradient-to-br from-brass to-foreground/70" />
            Lastro <span className="font-normal text-muted-foreground">Docs</span>
          </a>
          <nav aria-label="Main" className="hidden items-center gap-5 text-sm text-muted-foreground md:flex">
            <a href={href("quickstart")} className="hover:text-foreground">
              Quickstart
            </a>
            <a href={href("concepts/pipeline")} className="hover:text-foreground">
              Concepts
            </a>
            <a href={href("cli/overview")} className="hover:text-foreground">
              CLI
            </a>
            <a href={href("sdks/typescript")} className="hover:text-foreground">
              SDKs
            </a>
          </nav>
          <div className="ml-auto flex items-center gap-1">
            <Button variant="outline" size="sm" className="w-9 justify-start px-0 text-muted-foreground sm:w-56 sm:px-3" onClick={() => setSearch(true)} aria-label="Search the docs">
              <Search className="mx-auto sm:mx-0" />
              <span className="hidden sm:inline">Search docs…</span>
              <kbd className="ml-auto hidden rounded border bg-muted px-1.5 font-mono text-[10px] sm:inline">Ctrl K</kbd>
            </Button>
            <Button variant="ghost" size="icon" asChild>
              <a href={`${REPO}/tree/main/apps/sandbox`} target="_blank" rel="noreferrer" aria-label="Sandbox" title="Sandbox">
                <FlaskConical />
              </a>
            </Button>
            <Button variant="ghost" size="icon" asChild>
              <a href={REPO} target="_blank" rel="noreferrer" aria-label="GitHub repository" title="GitHub">
                <Github />
              </a>
            </Button>
            <Button variant="ghost" size="icon" onClick={toggle} aria-label={dark ? "Switch to light theme" : "Switch to dark theme"}>
              {dark ? <Sun /> : <Moon />}
            </Button>
          </div>
        </div>
      </header>

      {menu && (
        <div className="fixed inset-x-0 bottom-0 top-14 z-30 overflow-y-auto border-t bg-background px-4 py-6 lg:hidden" data-testid="mobile-nav">
          <SidebarNav slug={slug} onPick={() => setMenu(false)} />
        </div>
      )}

      <div className="mx-auto flex max-w-[1400px] gap-10 px-4 sm:px-6">
        <aside className="sticky top-14 hidden h-[calc(100vh-3.5rem)] w-60 shrink-0 overflow-y-auto py-8 pr-2 lg:block">
          <SidebarNav slug={slug} />
        </aside>
        <main id="content" tabIndex={-1} className="min-w-0 flex-1 py-8 outline-none sm:py-10">
          <div className="mx-auto max-w-3xl">
            {sectionOf(slug) && (
              <p className="mb-2 flex items-center gap-1.5 text-sm font-medium text-brass">
                <BookOpen className="h-3.5 w-3.5" /> {sectionOf(slug)!.title}
              </p>
            )}
            <article>{children}</article>
            {sectionOf(slug) && <Pager slug={slug} />}
          </div>
        </main>
        <aside className="sticky top-14 hidden h-[calc(100vh-3.5rem)] w-56 shrink-0 overflow-y-auto py-10 xl:block">
          <Toc slug={slug} />
        </aside>
      </div>
      <SearchDialog open={search} onClose={() => setSearch(false)} />
    </div>
  );
}
