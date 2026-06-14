import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

const backendUrl = process.env.BACKEND_URL || "http://localhost:8080";

// Dev server proxies API + WebSocket traffic to the running Go backend so
// `npm run dev` works against `docker compose up porque-api porque-db`.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
  },
  server: {
    port: 5173,
    // Poll for file changes: bind-mounted source on Docker Desktop (Windows/macOS)
    // doesn't deliver inotify events, so HMR needs polling to pick up edits.
    watch: { usePolling: true, interval: 300 },
    proxy: {
      "/api": { target: backendUrl, changeOrigin: true },
      "/ws": { target: backendUrl, ws: true, changeOrigin: true },
    },
  },
});
