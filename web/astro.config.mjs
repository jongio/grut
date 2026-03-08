// @ts-check
import { defineConfig } from 'astro/config';

export default defineConfig({
  site: 'https://jongio.github.io',
  base: '/grut',
  output: 'static',
  build: {
    assets: '_assets',
  },
  vite: {
    build: {
      cssMinify: true,
    },
  },
});
