import { useEffect, useState } from "react";

/**
 * Hash routing, so links can be shared between reviewers of the same server:
 *   #/proposals                     every proposal
 *   #/proposals/P-001               a proposal's conversation
 *   #/proposals/P-001/changes       its use-case changes and intents (with ?uc=UC-005 to jump to one)
 *   #/proposals/P-001/proofs        its proofs
 *   #/usecases                      the application's use cases
 *   #/usecases/UC-005               one use case, its proofs and history
 */
export interface Route {
  path: string[];
  query: URLSearchParams;
}

export function currentRoute(): Route {
  const raw = decodeURIComponent(location.hash.replace(/^#\/?/, ""));
  const [p = "", q = ""] = raw.split("?");
  return { path: p.split("/").filter(Boolean), query: new URLSearchParams(q) };
}

export function useRoute(): Route {
  const [route, setRoute] = useState(currentRoute);
  useEffect(() => {
    const on = () => setRoute(currentRoute());
    window.addEventListener("hashchange", on);
    return () => window.removeEventListener("hashchange", on);
  }, []);
  return route;
}

export const href = (path: string) => `#/${path.replace(/^\/+/, "")}`;
