import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The build goes into internal/ui/dist, which the Go binary embeds. The dev
// server sends API calls to a hub on port 8080, or at LOTSE_DEV_HUB.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": path.resolve(import.meta.dirname, "src") },
  },
  build: {
    outDir: "../internal/ui/dist",
    emptyOutDir: true,
  },
  server: {
    port: 3200,
    proxy: {
      "/api": { target: process.env.LOTSE_DEV_HUB ?? "http://localhost:8080" },
    },
  },
});
