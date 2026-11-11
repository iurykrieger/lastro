import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { lastroReviewApi } from "./server/api";

// The review app reads and writes a `.lastro/` workspace through the TypeScript SDK, so it runs on a
// server (`vite` or `vite preview`), not as a static site. LASTRO_DIR picks the workspace; by default it
// is this repository's own `.lastro/`.
export default defineConfig({
  base: "./",
  plugins: [react(), lastroReviewApi({ dir: path.resolve(__dirname, "../../.lastro") })],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
      "@lastro/sdk": path.resolve(__dirname, "../../sdks/typescript/src/index.ts"),
    },
  },
  server: { port: 5175 },
  preview: { port: 4175 },
  build: { target: "es2022" },
});
