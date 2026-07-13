import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  expect: { timeout: 8_000 },
  use: {
    baseURL: process.env.LORE_E2E_BASE_URL ?? "http://127.0.0.1:18081",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  reporter: process.env.CI ? "github" : "line",
});
