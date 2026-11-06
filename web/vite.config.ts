import { existsSync, mkdirSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import vue from '@vitejs/plugin-vue'
import { defineConfig, type Plugin } from 'vite'
import vuetify from 'vite-plugin-vuetify'

const backend = 'http://127.0.0.1:7788'
const outDir = resolve(import.meta.dirname, 'dist')

// emptyOutDir 会清掉 dist/.gitkeep，构建结束后补回，保证仓库里的占位文件一直在。
function keepGitkeep(): Plugin {
  return {
    name: 'scribekit-keep-gitkeep',
    apply: 'build',
    closeBundle() {
      if (!existsSync(outDir)) mkdirSync(outDir, { recursive: true })
      const keep = resolve(outDir, '.gitkeep')
      if (!existsSync(keep)) writeFileSync(keep, '')
    },
  }
}

export default defineConfig({
  plugins: [vue(), vuetify({ autoImport: true }), keepGitkeep()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: backend, changeOrigin: true, ws: true },
    },
  },
  build: {
    outDir,
    emptyOutDir: true,
    chunkSizeWarningLimit: 1600,
  },
})
