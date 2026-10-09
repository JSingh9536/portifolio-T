// Live ops console: client-rendered SPA, no SSR needed behind a VPN.
export default defineNuxtConfig({
  compatibilityDate: '2026-09-01',
  ssr: false,
  devtools: { enabled: false },
  css: ['~/assets/main.css'],
  app: { head: { title: 'Terraform Fleet · Site Console', htmlAttrs: { lang: 'en' } } },
  runtimeConfig: {
    // Server-only (NUXT_CONTROL_URL, NUXT_OPERATOR_KEY): used by the
    // /api/control proxy so the operator key never ships to the browser.
    controlUrl: 'http://localhost:3001',
    operatorKey: 'dev-operator',
    public: {
      // Browser-visible (NUXT_PUBLIC_INGEST_URL, NUXT_PUBLIC_SITE_ID).
      ingestUrl: 'http://localhost:8080',
      siteId: 'alameda-pilot',
    },
  },
  typescript: { strict: true },
});
