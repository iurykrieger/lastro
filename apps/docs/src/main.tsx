import { StrictMode, useEffect } from "react";
import { createRoot } from "react-dom/client";
import { Layout } from "@/components/Layout";
import { A, H1, Lead, P, SdkProvider, useAnchorScroll } from "@/components/doc";
import { PAGES } from "@/content/nav";
import { PAGE_COMPONENTS } from "@/pages";
import { useRoute } from "@/lib/router";
import "./index.css";

function NotFound({ slug }: { slug: string }) {
  return (
    <>
      <H1>Page not found</H1>
      <Lead>
        There is no page at <code className="font-mono">/{slug}</code>.
      </Lead>
      <P>
        Start from the <A to="introduction">introduction</A> or search with Ctrl K.
      </P>
    </>
  );
}

function App() {
  const route = useRoute();
  const slug = route || "introduction";
  const Page = PAGE_COMPONENTS[slug];
  const meta = PAGES.find((p) => p.slug === slug);
  useAnchorScroll(slug);
  useEffect(() => {
    document.title = meta ? `${meta.title} · Lastro Docs` : "Lastro Docs";
  }, [meta]);
  return (
    <SdkProvider>
      <Layout slug={slug}>{Page ? <Page /> : <NotFound slug={slug} />}</Layout>
    </SdkProvider>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
