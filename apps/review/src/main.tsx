import { StrictMode, useEffect } from "react";
import { createRoot } from "react-dom/client";
import { Layout } from "@/components/Layout";
import { Empty } from "@/components/bits";
import { ProposalPage } from "@/pages/Proposal";
import { ProposalsPage } from "@/pages/Proposals";
import { UseCasePage, UseCasesPage } from "@/pages/UseCases";
import { useRoute } from "@/lib/router";
import { WorkspaceProvider, useWorkspace } from "@/lib/workspace";
import "./index.css";

function Pages() {
  const route = useRoute();
  const { data, error } = useWorkspace();
  const [top = "proposals", sub] = route.path;
  const section = top === "usecases" ? "usecases" : "proposals";
  useEffect(() => {
    if (!sub) document.title = `${section === "usecases" ? "Use cases" : "Proposals"} · Lastro Review`;
  }, [section, sub]);

  let page;
  if (error && !data) page = <Empty>Could not read the workspace: {error}</Empty>;
  else if (!data) page = <p className="text-sm text-muted-foreground">Loading the workspace…</p>;
  else if (top === "usecases") page = sub ? <UseCasePage route={route} /> : <UseCasesPage />;
  else if (top === "proposals") page = sub ? <ProposalPage route={route} /> : <ProposalsPage />;
  else page = <Empty>There is nothing at this address.</Empty>;
  return <Layout section={section}>{page}</Layout>;
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <WorkspaceProvider>
      <Pages />
    </WorkspaceProvider>
  </StrictMode>,
);
