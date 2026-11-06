<script setup lang="ts">
import { useRoute } from 'vue-router'
import { useSystemStore } from '@/stores/system'
import { useAuthStore } from '@/stores/auth'
import { computed } from 'vue'

const route = useRoute()
const systemStore = useSystemStore()
const authStore = useAuthStore()

const pageTitle = computed(() => {
  const map: Record<string, string> = {
    '/': 'Dashboard',
    '/network': 'Network Interfaces',
    '/clients': 'DHCP Leases',
    '/system': 'System Information',
    '/apps': 'Services & Apps',
    '/files': 'USB File Browser',
    '/photos': 'Photo Library',
    '/downloads': 'Auto Download',
    '/automation': 'Automation Builder',
    '/diagnostics': 'Diagnostics',
    '/preferences': 'Preferences',
  }
  return map[route.path] || 'R3P Dashboard'
})

function signOut() {
  void authStore.logout().catch((cause: unknown) => {
    console.error('Sign-out request failed', cause)
  })
}
</script>

<template>
  <header
    class="flex items-center justify-between px-3 py-3 shrink-0 sm:px-6"
    style="background: var(--bg-surface); border-bottom: 1px solid rgba(255,255,255,0.06); min-height: 56px;"
  >
    <div class="flex items-center gap-3">
      <h1 class="text-sm font-semibold text-white sm:text-base">{{ pageTitle }}</h1>
    </div>

    <div class="flex items-center gap-2 text-sm sm:gap-5">
      <!-- Uptime -->
      <div class="hidden items-center gap-1.5 text-gray-400 md:flex">
        <span>⏱</span>
        <span>{{ systemStore.uptimeFormatted }}</span>
      </div>

      <!-- Hostname -->
      <div class="hidden items-center gap-1.5 text-gray-400 lg:flex">
        <span>🖥</span>
        <span>{{ systemStore.hostname }}</span>
      </div>

      <!-- Connection indicator -->
      <div
        class="flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-full"
        :style="systemStore.connected
          ? 'background: rgba(104,211,145,0.12); color: var(--accent-green);'
          : 'background: rgba(237,137,54,0.12); color: var(--accent-orange);'"
      >
        <span class="w-1.5 h-1.5 rounded-full animate-pulse"
              :style="systemStore.connected ? 'background: var(--accent-green);' : 'background: var(--accent-orange);'">
        </span>
        <span class="hidden sm:inline">{{ systemStore.connected ? 'Connected' : 'Disconnected' }}</span>
      </div>
      <button class="text-xs text-gray-400 hover:text-white" aria-label="Sign out" @click="signOut">
        <span class="hidden sm:inline">Sign out</span>
        <span class="sm:hidden">Exit</span>
      </button>
    </div>
  </header>
</template>
