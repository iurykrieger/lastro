import { useEffect, useState } from "react";

/**
 * Hash routing: `#/concepts/intents`, with an optional in-page anchor after `~`
 * (`#/cli/overview~exit-codes`). Static hosting and file:// serve every page.
 */
export const href = (slug: string) => `#/${slug.replace(/^\/+/, "")}`;

export function currentSlug(): string {
  return decodeURIComponent(location.hash.replace(/^#\/?/, "").split("~")[0] ?? "").replace(/\/+$/, "");
}

export function useRoute(): string {
  const [slug, setSlug] = useState(currentSlug);
  useEffect(() => {
    const on = () => setSlug(currentSlug());
    window.addEventListener("hashchange", on);
    return () => window.removeEventListener("hashchange", on);
  }, []);
  return slug;
}

export function navigate(slug: string) {
  location.hash = href(slug);
}
