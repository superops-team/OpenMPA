import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { darkTheme } from './src/build/dark-theme';
export default defineConfig({
  base: './',
  plugins: [
    react(),
    {
      name: 'desktop-entry',
      enforce: 'post',
      configureServer(server) {
        server.middlewares.use((request, _response, next) => {
          if (request.url === '/' || request.url === '/index.html')
            request.url = '/desktop.html';
          next();
        });
      },
    },
  ],
  css: { postcss: { plugins: [darkTheme()] } },
  build: { outDir: 'dist/desktop', rollupOptions: { input: 'desktop.html' } },
  server: { strictPort: true, host: '127.0.0.1' },
});
