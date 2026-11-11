import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The docs are a static site: hash routing, so any static host (or file://) serves every page.
// The interactive examples run the real TypeScript SDK (from source) and the Rust prover (WASM) in the page.
export default defineConfig({
  base: "./",
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
      "@lastro/sdk": path.resolve(__dirname, "../../sdks/typescript/src/index.ts"),
      "@lastro/example-login": path.resolve(__dirname, "../../sdks/typescript/examples/login/login.ts"),
      "@lastro/wasm": path.resolve(__dirname, "../../sdks/typescript/wasm"),
    },
  },
  assetsInclude: ["**/*.wasm"],
  server: { port: 5174 },
  preview: { port: 4174 },
  build: { target: "es2022" },
});
