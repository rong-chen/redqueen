import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 9092,
    strictPort: false, // 若 9092 被占用，自动换其它端口启动
  }
})
