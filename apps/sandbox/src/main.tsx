import { StrictMode, useState } from "react";
import { createRoot } from "react-dom/client";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Badge } from "@/components/ui/badge";
import { ProsePanel } from "@/components/ProsePanel";
import { CodePanel } from "@/components/CodePanel";
import { ContrastPanel } from "@/components/ContrastPanel";
import { Pipeline } from "@/components/Pipeline";
import { useLastro } from "@/state";
import "./index.css";

function App() {
  const l = useLastro();
  const [hover, setHover] = useState<string | null>(null);
  return (
    <TooltipProvider>
      <div className="mx-auto grid max-w-[1400px] gap-4 py-5">
        <header className="flex flex-wrap items-end justify-between gap-3">
          <div className="grid gap-1">
            <div className="flex items-center gap-2.5">
              <span aria-hidden className="h-5 w-5 rounded bg-gradient-to-br from-brass to-foreground/70" />
              <h1 className="text-2xl font-semibold tracking-tight">Lastro Sandbox</h1>
            </div>
            <p className="text-xs text-muted-foreground">Proof-driven development: prose becomes reviewed intents, intents become proofs, proofs judge the code.</p>
          </div>
          <div className="flex flex-wrap items-center gap-1.5 text-xs">
            <Badge variant="proved">Proved</Badge>
            <Badge variant="trusted">Under assumptions</Badge>
            <Badge variant="evidence">Evidenced</Badge>
            <Badge variant="beyond">Beyond proof</Badge>
            <Badge variant="fail">Violated</Badge>
          </div>
        </header>
        <Pipeline l={l} />
        <div className="grid gap-4 xl:grid-cols-2">
          <ProsePanel l={l} hover={hover} setHover={setHover} />
          <CodePanel l={l} />
        </div>
        <ContrastPanel l={l} hover={hover} setHover={setHover} />
        <footer className="pb-6 text-center text-xs text-muted-foreground">
          Every verdict comes from the Lastro SDK and the lastro-core Rust prover running in this page.
        </footer>
      </div>
    </TooltipProvider>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
