import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig, loadEnv } from "vite"
import wails from "@wailsio/runtime/plugins/vite";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')

  return {
    plugins: [react(), tailwindcss(), wails("./bindings")],
    server: {
      host: "127.0.0.1",
      port: parseInt(env.VITE_PORT) || 9245,
      strictPort: true,
    },
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    build: {
      // Desktop/Wails loads assets locally — a ~1MB main chunk is fine.
      // Keep feature lazy-loads (e.g. Terminal/xterm); skip vendor chunking.
      chunkSizeWarningLimit: 1500,
    },
  }
})
