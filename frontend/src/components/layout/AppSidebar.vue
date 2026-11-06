<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { useSystemStore } from '@/stores/system'

defineProps<{ collapsed: boolean }>()
const emit = defineEmits<{ toggle: [] }>()

const route = useRoute()
const router = useRouter()
const system = useSystemStore()

interface NavItem {
  id: string
  label: string
  icon: string
  to: string
  badge?: string
}

interface NavGroup {
  title: string
  items: NavItem[]
}

const navGroups: NavGroup[] = [
  {
    title: 'Overview',
    items: [
      { id: 'dashboard', label: 'Dashboard', icon: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6', to: '/' },
      { id: 'network', label: 'Network (LAN/WAN)', icon: 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z', to: '/network' },
      { id: 'wireless', label: 'Wireless (Wi-Fi)', icon: 'M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0', to: '/wireless' },
    ],
  },
  {
    title: 'Security & Access',
    items: [
      { id: 'adblock', label: 'AdBlock DNS', icon: 'M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636', to: '/adblock', badge: 'DNS' },
      { id: 'firewall', label: 'Firewall', icon: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z', to: '/firewall' },
      { id: 'access', label: 'Access & Accounts', icon: 'M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z', to: '/access' },
      { id: 'config', label: 'Config (UCI)', icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z', to: '/config' },
    ],
  },
  {
    title: 'Services & Storage',
    items: [
      { id: 'downloads', label: 'Download Bot', icon: 'M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4', to: '/downloads', badge: 'ESP32' },
      { id: 'nas', label: 'NAS & USB 3.0', icon: 'M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4', to: '/nas', badge: '500G' },
      { id: 'ai', label: 'AI Assistant', icon: 'M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z', to: '/ai', badge: 'ROOT' },
      { id: 'apps', label: 'Applications', icon: 'M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z', to: '/apps' },
      { id: 'automation', label: 'Automation & Cron', icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z', to: '/automation' },
      { id: 'iot', label: 'IoT (MQTT)', icon: 'M13 10V3L4 14h7v7l9-11h-7z', to: '/iot' },
      { id: 'plugins', label: 'Plugins', icon: 'M11 4a2 2 0 114 0v1a1 1 0 001 1h3a1 1 0 011 1v3a1 1 0 01-1 1h-1a2 2 0 100 4h1a1 1 0 011 1v3a1 1 0 01-1 1h-3a1 1 0 01-1-1v-1a2 2 0 10-4 0v1a1 1 0 01-1 1H7a1 1 0 01-1-1v-3a1 1 0 00-1-1H4a2 2 0 110-4h1a1 1 0 001-1V7a1 1 0 011-1h3a1 1 0 001-1V4z', to: '/plugins' },
    ],
  },
  {
    title: 'Maintenance',
    items: [
      { id: 'system', label: 'System & Logs', icon: 'M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z', to: '/system' },
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
  <aside
    class="flex flex-col h-full bg-[#0b0f19] border-r border-[rgba(255,255,255,0.08)] select-none shrink-0 transition-all duration-150"
    :class="collapsed ? 'w-14' : 'w-52'"
  >
    <!-- Brand / Header -->
    <div class="h-11 px-3 flex items-center justify-between border-b border-[rgba(255,255,255,0.08)]">
      <div v-if="!collapsed" class="flex items-center gap-2">
        <div class="w-6 h-6 rounded bg-sky-500 flex items-center justify-center text-white font-bold text-xs tracking-wider shadow">
          R3
        </div>
        <div>
          <div class="text-[12.5px] font-bold text-white tracking-tight flex items-center gap-1.5">
            R3P SERVER
            <span class="text-[9px] px-1 py-0.2 rounded bg-sky-500/20 text-sky-400 font-semibold border border-sky-500/30">PRO</span>
          </div>
        </div>
      </div>
      <div v-else class="w-full flex justify-center">
        <div class="w-6 h-6 rounded bg-sky-500 flex items-center justify-center text-white font-bold text-xs">
          R
        </div>
      </div>
      <button
        class="text-gray-400 hover:text-white p-1 rounded hover:bg-white/5 transition"
        :title="collapsed ? 'Expand' : 'Collapse'"
        @click="emit('toggle')"
      >
        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" :d="collapsed ? 'M13 5l7 7-7 7M5 5l7 7-7 7' : 'M11 19l-7-7 7-7m8 14l-7-7 7-7'" />
        </svg>
      </button>
    </div>

    <!-- Navigation Scroll Area -->
    <nav class="flex-1 overflow-y-auto px-1.5 py-2 space-y-3.5">
      <div v-for="group in navGroups" :key="group.title">
        <div v-if="!collapsed" class="px-2 mb-1 text-[10px] font-bold text-gray-500 uppercase tracking-wider">
          {{ group.title }}
        </div>
        <div class="space-y-0.5">
          <button
            v-for="item in group.items"
            :key="item.to"
            class="w-full flex items-center gap-2.5 px-2 py-1.5 rounded text-[12px] font-medium transition group relative text-left"
            :class="isActive(item.to)
              ? 'bg-sky-500/15 text-sky-300 border border-sky-500/30 shadow-sm'
              : 'text-gray-400 hover:text-gray-200 hover:bg-white/[0.04] border border-transparent'"
            :title="collapsed ? item.label : undefined"
            @click="navigate(item.to)"
          >
            <svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" :d="item.icon" />
            </svg>
            <span v-if="!collapsed" class="truncate flex-1">{{ item.label }}</span>
            <span v-if="!collapsed && item.badge" class="badge badge-blue text-[9px] px-1 py-0">{{ item.badge }}</span>
          </button>
        </div>
      </div>
    </nav>

    <!-- Footer System Status -->
    <div class="p-2 border-t border-[rgba(255,255,255,0.08)] bg-[#090d16] text-[11px]">
      <div v-if="!collapsed" class="space-y-1.5">
        <div class="flex items-center justify-between text-gray-400">
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full" :class="system.connected ? 'bg-emerald-400 animate-pulse' : 'bg-red-400'"></span>
            {{ system.connected ? '192.168.2.1' : 'Disconnected' }}
          </span>
          <span class="mono text-gray-500 text-[10px]">MT7621</span>
        </div>
      </div>
      <div v-else class="flex justify-center">
        <span class="w-2 h-2 rounded-full" :class="system.connected ? 'bg-emerald-400' : 'bg-red-400'"></span>
      </div>
    </div>
  </aside>
</template>
