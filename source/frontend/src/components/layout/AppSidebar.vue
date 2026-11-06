<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { useSystemStore } from '@/stores/system'

defineProps<{ collapsed: boolean }>()
const emit = defineEmits<{ toggle: [] }>()

const route = useRoute()
const router = useRouter()
const system = useSystemStore()

const sections = [
  {
    label: 'Overview',
    links: [{ label: 'Dashboard', icon: '⌂', to: '/' }],
  },
  {
    label: 'Network',
    links: [
      { label: 'Monitor', icon: '⌁', to: '/network' },
      { label: 'DHCP leases', icon: '◉', to: '/clients' },
    ],
  },
  {
    label: 'Mini-server',
    links: [
      { label: 'Services & apps', icon: '▦', to: '/apps' },
      { label: 'USB files', icon: '▤', to: '/files' },
      { label: 'Photo Library', icon: '▧', to: '/photos' },
      { label: 'Auto Download', icon: '⇩', to: '/downloads' },
      { label: 'Automation', icon: '⌘', to: '/automation' },
      { label: 'Diagnostics', icon: '⌕', to: '/diagnostics' },
    ],
  },
  {
    label: 'System',
    links: [
      { label: 'System info', icon: '⚙', to: '/system' },
      { label: 'Preferences', icon: '◐', to: '/preferences' },
    ],
  },
]

function isActive(to: string) {
  return route.path === to
}

function navigate(to: string) {
  void router.push(to)
}
</script>

<template>
  <aside class="app-sidebar" :class="collapsed ? 'app-sidebar-collapsed' : ''">
    <div class="sidebar-brand">
      <div class="brand-mark">R3</div>
      <span v-if="!collapsed" class="brand-title">R3P <span>Mini Server</span></span>
    </div>

    <nav class="sidebar-nav" aria-label="Main navigation">
      <section v-for="section in sections" :key="section.label" class="sidebar-section">
        <p v-if="!collapsed" class="sidebar-section-title">{{ section.label }}</p>
        <button
          v-for="link in section.links"
          :key="link.to"
          class="sidebar-link"
          :class="{ 'sidebar-link-active': isActive(link.to) }"
          :title="collapsed ? link.label : undefined"
          :aria-current="isActive(link.to) ? 'page' : undefined"
          @click="navigate(link.to)"
        >
          <span class="sidebar-icon" aria-hidden="true">{{ link.icon }}</span>
          <span v-if="!collapsed" class="truncate">{{ link.label }}</span>
        </button>
      </section>
    </nav>

    <div class="sidebar-footer">
      <div v-if="!collapsed" class="connection-state" :class="system.connected ? 'connection-live' : 'connection-offline'">
        <span class="connection-dot"></span>
        {{ system.connected ? 'Live data' : 'Disconnected' }}
      </div>
      <button class="sidebar-collapse" :aria-label="collapsed ? 'Expand navigation' : 'Collapse navigation'" @click="emit('toggle')">
        {{ collapsed ? '→' : '←' }}
      </button>
    </div>
  </aside>
</template>
