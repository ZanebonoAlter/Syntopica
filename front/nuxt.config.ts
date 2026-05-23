// https://nuxt.com/docs/api/configuration/nuxt-config
import tailwindcss from "@tailwindcss/vite";

export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  devtools: { enabled: true },
  ignore: ['app/_deprecated/**'],
  runtimeConfig: {
    apiInternalBase: process.env.API_INTERNAL_BASE || 'http://localhost:5000/api',
    public: {
      apiOrigin: process.env.NUXT_PUBLIC_API_ORIGIN || 'http://localhost:5000',
      apiBase: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:5000/api',
    },
  },
  vite: {
    plugins: [
      tailwindcss(),
    ],
  },
  css: ['~/assets/css/main.css'],
  modules: ['motion-v/nuxt', '@pinia/nuxt'],
  app: {
    head: {
      title: 'Syntopica',
      link: [
        { rel: 'icon', type: 'image/png', href: '/favicon.png' },
      ],
    },
  },
})
