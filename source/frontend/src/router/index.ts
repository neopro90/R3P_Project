import { createRouter, createWebHashHistory } from 'vue-router'
import DashboardView from '@/views/DashboardView.vue'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      name: 'dashboard',
      component: DashboardView,
    },
    {
      path: '/network',
      name: 'network',
      component: () => import('@/views/NetworkView.vue'),
    },
    {
      path: '/clients',
      name: 'clients',
      component: () => import('@/views/ClientsView.vue'),
    },
    {
      path: '/system',
      name: 'system',
      component: () => import('@/views/SystemView.vue'),
    },
    {
      path: '/apps',
      name: 'apps',
      component: () => import('@/views/AppCenterView.vue'),
    },
    {
      path: '/files',
      name: 'files',
      component: () => import('@/views/FilesView.vue'),
    },
    {
      path: '/photos',
      name: 'photos',
      component: () => import('@/views/PhotoLibraryView.vue'),
    },
    {
      path: '/downloads',
      name: 'downloads',
      component: () => import('@/views/DownloadsView.vue'),
    },
    {
      path: '/automation',
      name: 'automation',
      component: () => import('@/views/AutomationView.vue'),
    },
    {
      path: '/diagnostics',
      name: 'diagnostics',
      component: () => import('@/views/DiagnosticsView.vue'),
    },
    {
      path: '/preferences',
      name: 'preferences',
      component: () => import('@/views/PreferencesView.vue'),
    },
  ],
})

export default router
