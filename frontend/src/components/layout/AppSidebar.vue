<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useSystemStore } from '@/stores/system'
import { useUiPreferences } from '@/composables/useUiPreferences'

const props = withDefaults(defineProps<{
  collapsed?: boolean
  isMobile?: boolean
}>(), {
  collapsed: false,
  isMobile: false,
})
const emit = defineEmits<{
  toggle: []
  close: []
}>()

const route = useRoute()
const system = useSystemStore()
const prefs = useUiPreferences()

interface NavItem {
  id: string
  labelKey: string
  icon: string
  to: string
  badge?: string
}

interface NavGroup {
  titleKey: string
  items: NavItem[]
}

// Requirement 1: AI Assistant placed right at the top, directly below Dashboard!
const navGroups = computed<NavGroup[]>(() => [
  {
    titleKey: 'group_overview',
    items: [
      { id: 'dashboard', labelKey: 'nav_dashboard', icon: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6', to: '/' },
      { id: 'ai', labelKey: 'nav_ai', icon: 'M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z', to: '/ai', badge: 'ROOT AI' },
      { id: 'network', labelKey: 'nav_network', icon: 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z', to: '/network' },
      { id: 'wireless', labelKey: 'nav_wireless', icon: 'M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0', to: '/wireless' },
    ],
  },
  {
    titleKey: 'group_security',
    items: [
      { id: 'adblock', labelKey: 'nav_adblock', icon: 'M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636', to: '/adblock', badge: 'DNS' },
      { id: 'firewall', labelKey: 'nav_firewall', icon: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z', to: '/firewall' },
      { id: 'access', labelKey: 'nav_access', icon: 'M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z', to: '/access' },
      { id: 'config', labelKey: 'nav_config', icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z', to: '/config' },
    ],
  },
  {
    titleKey: 'group_services',
    items: [
      { id: 'downloads', labelKey: 'nav_downloads', icon: 'M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4', to: '/downloads', badge: 'YT-DLP' },
      { id: 'nas', labelKey: 'nav_nas', icon: 'M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4', to: '/nas', badge: 'SHARE' },
      { id: 'explorer', labelKey: 'nav_explorer', icon: 'M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z', to: '/files', badge: 'TREE' },
      { id: 'apps', labelKey: 'nav_apps', icon: 'M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z', to: '/apps' },
      { id: 'automation', labelKey: 'nav_automation', icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z', to: '/automation' },
      { id: 'iot', labelKey: 'nav_iot', icon: 'M13 10V3L4 14h7v7l9-11h-7z', to: '/iot' },
      { id: 'plugins', labelKey: 'nav_plugins', icon: 'M11 4a2 2 0 114 0v1a1 1 0 001 1h3a1 1 0 011 1v3a1 1 0 01-1 1h-1a2 2 0 100 4h1a1 1 0 011 1v3a1 1 0 01-1 1h-3a1 1 0 01-1-1v-1a2 2 0 10-4 0v1a1 1 0 01-1 1H7a1 1 0 01-1-1v-3a1 1 0 00-1-1H4a2 2 0 110-4h1a1 1 0 001-1V7a1 1 0 011-1h3a1 1 0 001-1V4z', to: '/plugins' },
    ],
  },
  {
    titleKey: 'group_maintenance',
    items: [
      { id: 'system', labelKey: 'nav_system', icon: 'M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z', to: '/system' },
    ],
  },
])

function isActive(to: string) {
  if (to === '/') return route.path === '/'
  return route.path.startsWith(to)
}

function handleNavClick() {
  if (props.isMobile) {
    emit('close')
  }
}
</script>

<template>
  <aside
    class="flex flex-col h-full bg-[#0b0f19] select-none shrink-0 transition-all duration-150"
    :class="[
      isMobile ? 'w-full' : (collapsed ? 'w-16' : 'w-[312px]'),
      !isMobile ? 'border-r border-[var(--border-subtle)]' : '',
    ]"
  >
    <!-- Brand / Home Assistant Style Header -->
    <div class="h-14 px-3.5 flex items-center justify-between border-b border-[var(--border-subtle)]">
      <router-link
        to="/"
        class="flex items-center gap-2.5 no-underline cursor-pointer"
        @click="handleNavClick"
      >
        <div v-if="!collapsed || isMobile" class="flex items-center gap-2.5">
          <div class="w-8 h-8 rounded-lg bg-[#03a9f4] flex items-center justify-center text-white font-bold text-sm tracking-wider shadow">
            <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
              <path d="M12 3L2 12h3v8h6v-6h2v6h6v-8h3L12 3z" />
            </svg>
          </div>
          <div>
            <div class="text-[17px] font-bold text-white tracking-tight flex items-center gap-2">
              R3P MINI SERVER
              <span class="text-[11px] px-2 py-0.5 rounded-full bg-sky-500/20 text-sky-400 font-bold border border-sky-500/30">OS</span>
            </div>
          </div>
        </div>
        <div v-else class="w-full flex justify-center">
          <div class="w-8 h-8 rounded-lg bg-[#03a9f4] flex items-center justify-center text-white font-bold text-sm">
            R
          </div>
        </div>
      </router-link>

      <!-- Close button on Mobile vs Collapse button on Desktop -->
      <button
        v-if="isMobile"
        type="button"
        class="text-gray-400 hover:text-white p-1.5 rounded-lg hover:bg-white/10 transition cursor-pointer text-base font-bold"
        @click="emit('close')"
      >
        ✕
      </button>
      <button
        v-else
        type="button"
        class="text-gray-400 hover:text-white p-1.5 rounded-lg hover:bg-white/5 transition cursor-pointer"
        :title="collapsed ? 'Expand' : 'Collapse'"
        @click="emit('toggle')"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" :d="collapsed ? 'M13 5l7 7-7 7M5 5l7 7-7 7' : 'M11 19l-7-7 7-7m8 14l-7-7 7-7'" />
        </svg>
      </button>
    </div>

    <!-- Navigation Scroll Area -->
    <nav class="flex-1 overflow-y-auto px-2.5 py-3 space-y-4">
      <div v-for="group in navGroups" :key="group.titleKey">
        <div
          v-if="!collapsed || isMobile"
          class="px-2.5 mb-1.5 text-[13.5px] font-bold text-gray-400 uppercase tracking-wider flex items-center justify-between"
        >
          <span>{{ prefs.t(group.titleKey) }}</span>
        </div>
        <div class="space-y-1.5">
          <router-link
            v-for="item in group.items"
            :key="item.to"
            :to="item.to"
            class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl text-[17px] font-medium transition group relative text-left cursor-pointer no-underline select-none"
            :class="isActive(item.to)
              ? 'bg-sky-500/15 text-sky-400 border border-sky-500/35 shadow-sm font-bold'
              : 'text-gray-300 hover:text-white hover:bg-white/[0.06] border border-transparent'"
            :title="collapsed && !isMobile ? prefs.t(item.labelKey) : undefined"
            @click="handleNavClick"
          >
            <svg class="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" :d="item.icon" />
            </svg>
            <span v-if="!collapsed || isMobile" class="truncate flex-1">{{ prefs.t(item.labelKey) }}</span>
            <span
              v-if="(!collapsed || isMobile) && item.badge"
              class="badge text-[12px] px-2 py-0.5 font-bold"
              :class="item.id === 'ai' ? 'badge-purple' : 'badge-blue'"
            >
              {{ item.badge }}
            </span>
          </router-link>
        </div>
      </div>
    </nav>

    <!-- Footer System Status -->
    <div class="p-3 border-t border-[var(--border-subtle)] bg-[#090d16] text-[13px]">
      <div v-if="!collapsed || isMobile" class="space-y-1.5">
        <div class="flex items-center justify-between text-gray-300">
          <span class="flex items-center gap-2">
            <span class="w-2.5 h-2.5 rounded-full" :class="system.connected ? 'bg-emerald-400 animate-pulse' : 'bg-red-400'"></span>
            {{ system.connected ? (system.lanIp || '—') : prefs.t('disconnected') }}
          </span>
          <span class="mono text-gray-400 text-[12px] truncate max-w-[120px]">MT7621 · {{ system.appVersion }}</span>
        </div>
      </div>
      <div v-else class="flex justify-center">
        <span class="w-2.5 h-2.5 rounded-full" :class="system.connected ? 'bg-emerald-400' : 'bg-red-400'"></span>
      </div>
    </div>
  </aside>
</template>
