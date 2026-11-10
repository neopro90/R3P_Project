import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      name: 'dashboard',
      component: () => import('@/views/DashboardView.vue'),
    },
    {
      path: '/network',
      name: 'network',
      component: () => import('@/views/NetworkMonitorView.vue'),
    },
    {
      path: '/wireless',
      name: 'wireless',
      component: () => import('@/views/WirelessView.vue'),
    },
    {
      path: '/adblock',
      name: 'adblock',
      component: () => import('@/views/AdblockView.vue'),
    },
    {
      path: '/firewall',
      name: 'firewall',
      component: () => import('@/views/FirewallView.vue'),
    },
    {
      path: '/downloads',
      name: 'downloads',
      component: () => import('@/views/DownloadBotView.vue'),
    },
    {
      path: '/nas',
      name: 'nas',
      component: () => import('@/views/NasView.vue'),
    },
    {
      path: '/files',
      name: 'files',
      component: () => import('@/views/FileExplorerView.vue'),
    },
    {
      path: '/ai',
      name: 'ai',
      component: () => import('@/views/AiAssistantView.vue'),
    },
    {
      path: '/access',
      name: 'access',
      component: () => import('@/views/AccessView.vue'),
    },
    {
      path: '/automation',
      name: 'automation',
      component: () => import('@/views/AutomationView.vue'),
    },
    {
      path: '/apps',
      name: 'apps',
      component: () => import('@/views/ApplicationsView.vue'),
    },
    {
      path: '/iot',
      name: 'iot',
      component: () => import('@/views/IotView.vue'),
    },
    {
      path: '/plugins',
      name: 'plugins',
      component: () => import('@/views/PluginsView.vue'),
    },
    {
      path: '/system',
      name: 'system',
      component: () => import('@/views/SystemView.vue'),
    },
    {
      path: '/config',
      name: 'config',
      component: () => import('@/views/ConfigView.vue'),
    },
  ],
})

// Automatically handle stale chunks after firmware/UI updates in active sessions
router.onError((error) => {
  const msg = error?.message || String(error)
  if (
    msg.includes('Failed to fetch dynamically imported module') ||
    msg.includes('Importing a module script failed') ||
    error?.name === 'ChunkLoadError'
  ) {
    console.warn('[R3P Router] Stale UI build chunk detected after deployment, reloading latest version…', error)
    window.location.reload()
  }
})

if (typeof window !== 'undefined') {
  window.addEventListener('vite:preloadError', (event) => {
    console.warn('[R3P Router] Vite preload error, reloading latest application assets…', event)
    window.location.reload()
  })
}

export default router
