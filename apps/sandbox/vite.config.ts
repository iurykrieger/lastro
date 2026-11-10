import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { viteSingleFile } from "vite-plugin-singlefile";

// `SINGLEFILE=1 vite build` inlines everything (JS, CSS, WASM) into one HTML file.
export default defineConfig({
  plugins: [react(), ...(process.env.SINGLEFILE ? [viteSingleFile()] : [])],
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
