/**
 * Reads the Mermaid statechart the SDK writes (mermaidStatechart) back into regions, states and transitions, so the
 * review app can draw it without a Mermaid runtime. Only the SDK's own output is read; anything else is ignored.
 */
export interface ChartState {
  id: string;
  label: string;
  initial: boolean;
  forbidden: boolean;
}
export interface ChartTransition {
  from: string;
  to: string;
  event: string;
  counterexample: boolean;
}
export interface ChartRegion {
  name: string;
  states: ChartState[];
  transitions: ChartTransition[];
}

export function parseStatechart(mermaid: string): { regions: ChartRegion[] } {
  const regions: ChartRegion[] = [];
  const labels = new Map<string, string>();
  const forbidden = new Set<string>();
  let region: ChartRegion | undefined;
  const current = () => (region ??= (regions.push({ name: "states", states: [], transitions: [] }), regions[regions.length - 1]!));
  for (const raw of mermaid.split("\n")) {
    const line = raw.trim();
    let m: RegExpMatchArray | null;
    if ((m = line.match(/^%% (.+)$/))) {
      region = { name: m[1]!, states: [], transitions: [] };
      regions.push(region);
    } else if (line === "--") region = undefined;
    else if ((m = line.match(/^state "(.*)" as (\w+)$/))) {
      labels.set(m[2]!, m[1]!);
      current().states.push({ id: m[2]!, label: m[1]!, initial: false, forbidden: false });
    } else if ((m = line.match(/^\[\*\] --> (\w+)$/))) {
      const s = current().states.find((x) => x.id === m![1]);
      if (s) s.initial = true;
    } else if ((m = line.match(/^(\w+) --> (\w+) : (.+?)( \(counterexample\))?$/))) {
      current().transitions.push({ from: m[1]!, to: m[2]!, event: m[3]!, counterexample: !!m[4] });
    } else if ((m = line.match(/^class (.+) forbidden$/))) for (const id of m[1]!.split(",")) forbidden.add(id.trim());
  }
  for (const r of regions) {
    for (const s of r.states) s.forbidden = forbidden.has(s.id);
    for (const t of r.transitions) {
      t.from = labels.get(t.from) ?? t.from;
      t.to = labels.get(t.to) ?? t.to;
    }
  }
  return { regions: regions.filter((r) => r.states.length || r.transitions.length) };
}
