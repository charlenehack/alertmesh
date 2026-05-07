import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    cssCodeSplit: true,
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        // Manual splits keep the heaviest deps in their own caches so a
        // route change doesn't re-download antd / icons / charts on
        // first paint of an unrelated page.
        manualChunks(id) {
          if (!id.includes('node_modules')) return
          if (id.includes('@ant-design/icons')) return 'antd-icons'
          if (id.includes('antd') || id.includes('rc-')) return 'antd-core'
          if (id.includes('@tanstack/react-query')) return 'query'
          if (id.includes('recharts') || id.includes('d3-')) return 'charts'
          if (
            id.includes('react-markdown') ||
            id.includes('rehype') ||
            id.includes('remark') ||
            id.includes('highlight.js') ||
            id.includes('hast-') ||
            id.includes('mdast-') ||
            id.includes('micromark') ||
            id.includes('unified')
          )
            return 'markdown'
          if (id.includes('jsencrypt')) return 'crypto-vendor'
          if (
            id.includes('react-router') ||
            id.includes('react-dom') ||
            id.includes('/react/')
          )
            return 'react-vendor'
        },
      },
    },
  },
})
