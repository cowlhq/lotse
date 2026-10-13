import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The build goes into internal/ui/dist, which the Go binary embeds. The dev
// server sends API calls and WebSockets to a hub on port 8080.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": path.resolve(import.meta.dirname, "src") },
  },
  build: {
    outDir: "../internal/ui/dist",
    emptyOutDir: true,
    // The app is served from the binary, often on the local network; one
    // bundle of this size is fine.
    chunkSizeWarningLimit: 1000,
  },
  server: {
    port: 3200,
    proxy: {
      "/api": { target: "http://localhost:8080", ws: true },
    },
  },
});
