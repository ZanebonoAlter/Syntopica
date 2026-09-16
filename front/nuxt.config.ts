// https://nuxt.com/docs/api/configuration/nuxt-config
import tailwindcss from "@tailwindcss/vite";

export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  ssr: false,
  devtools: { enabled: true },
  // 显式绑 0.0.0.0：默认 localhost 在 Windows 上解析为 ::1 只绑 v6 环回，
  // WSL mirrored v4 (127.0.0.1) 够不着（v6-only 监听问题，契约见 openspec spec dev-api-networking）
  devServer: {
    host: '0.0.0.0',
  },
  ignore: ['app/_deprecated/**'],
  runtimeConfig: {
    public: {
      // 后端默认 5100（避开 Windows 5000 端口 WSD/svchost 保留段冲突）。
      // 绝对直连 + 后端 CORS 白名单是既有已验证拓扑；跨域/远程后端用 NUXT_PUBLIC_API_BASE 覆盖。
      apiBase: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:5100/api',
    },
  },
  vite: {
    plugins: [
      tailwindcss(),
    ],
  },
  components: [
    { path: '~/components', pathPrefix: false },
  ],
  css: ['~/assets/css/main.css'],
  modules: ['motion-v/nuxt', '@pinia/nuxt'],
  app: {
    head: {
      title: 'Syntopica',
      htmlAttrs: {
        'data-theme': 'editorial',
      },
      link: [
        { key: 'app-favicon', rel: 'icon', type: 'image/png', href: '/favicon.png' },
        // Noto Serif SC 由 @fontsource 自托管（spa-loading-ux）：同源分片加载，
        // 弱网/离线不再被 fonts.googleapis.com 渲染阻塞
      ],
      script: [
        {
          innerHTML: `(function(){try{var t=localStorage.getItem('syntopica-theme');if(t==='editorial'||t==='dark'){document.documentElement.setAttribute('data-theme',t)}else{document.documentElement.setAttribute('data-theme','editorial')}}catch(e){document.documentElement.setAttribute('data-theme','editorial')}})()`,
          tagPosition: 'head',
        },
      ],
    },
  },
})
