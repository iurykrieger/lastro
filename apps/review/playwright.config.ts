import { defineConfig, devices } from "@playwright/test";
import { WORK_DIR, resetWorkspace } from "./e2e/fixture";

// UI tests against the production build (vite preview), serving a fixture workspace (e2e/fixture.ts)
// copied to e2e/.work/.lastro. The tests write to it, so they run one at a time.
resetWorkspace();

export default defineConfig({
  testDir: "e2e",
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: "http://localhost:4176/",
    trace: "off",
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } } },
    { name: "mobile", use: { ...devices["Pixel 7"] } },
  ],
  webServer: {
    command: "npm run build && npx vite preview --port 4176 --strictPort",
    url: "http://localhost:4176/",
    env: { LASTRO_DIR: WORK_DIR, LASTRO_REVIEWER: "" },
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
