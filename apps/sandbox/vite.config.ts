import path from "node:path";
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import { viteSingleFile } from "vite-plugin-singlefile";

/**
 * Dev only: POST /api/lastro/ask runs one LLM step through the SDK's Claude
 * transport, so the key stays on the server. GET answers 200 when a key is set.
 */
function lastroAsk(): Plugin {
  return {
    name: "lastro-ask",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use("/api/lastro/ask", async (req, res) => {
        const send = (code: number, body: unknown) => {
          res.statusCode = code;
          res.setHeader("content-type", "application/json");
          res.end(JSON.stringify(body));
        };
        if (!process.env.ANTHROPIC_API_KEY) return send(404, { ok: false, error: "ANTHROPIC_API_KEY is not set" });
        if (req.method !== "POST") return send(200, { ok: true });
        let raw = "";
        for await (const chunk of req) raw += chunk;
        try {
          const { claudeAsk } = await server.ssrLoadModule(path.resolve(__dirname, "../../packages/sdk/src/llm/claude.ts"));
          send(200, { ok: true, value: await claudeAsk()(JSON.parse(raw)) });
        } catch (e) {
          send(200, { ok: false, error: (e as Error).message });
        }
      });
    },
  };
}

// `SINGLEFILE=1 vite build` inlines everything (JS, CSS, WASM) into one HTML file.
export default defineConfig({
  plugins: [react(), lastroAsk(), ...(process.env.SINGLEFILE ? [viteSingleFile()] : [])],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
      "@lastro/sdk": path.resolve(__dirname, "../../packages/sdk/src/index.ts"),
      "@lastro/example-login": path.resolve(__dirname, "../../packages/sdk/examples/login/login.ts"),
      "@lastro/wasm": path.resolve(__dirname, "../../packages/sdk/wasm"),
    },
  },
  assetsInclude: ["**/*.wasm"],
  build: { assetsInlineLimit: 100_000_000, target: "es2022" },
});
