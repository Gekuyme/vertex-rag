import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  timeout: 120_000,
  retries: 0,
  use: {
    baseURL: process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080"
  }
});
