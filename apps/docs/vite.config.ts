import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The docs are a static site: hash routing, so any static host (or file://) serves every page.
export default defineConfig({
  base: "./",
  plugins: [react()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  server: { port: 5174 },
  preview: { port: 4174 },
  build: { target: "es2022" },
});
