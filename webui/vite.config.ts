import { defineConfig } from "vitest/config";
import { env } from "node:process";

const apiTarget = env.TENSORS_ROUTER_WEBUI_API ?? "https://127.0.0.1:8443";

export default defineConfig({
  build: {
    outDir: "../internal/webui/assets",
    emptyOutDir: true
  },
  server: {
    proxy: {
      "/api": {
        target: apiTarget,
        changeOrigin: true,
        secure: false
      }
    }
  },
  test: {
    environment: "node"
  }
});
