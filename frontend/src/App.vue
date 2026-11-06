<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterView, useRoute } from 'vue-router'
import AppSidebar from '@/components/layout/AppSidebar.vue'
import AppTopbar from '@/components/layout/AppTopbar.vue'
import LoginView from '@/views/LoginView.vue'
import { useAuthStore } from '@/stores/auth'
import { useNetworkStore } from '@/stores/network'
import { useSystemStore } from '@/stores/system'
import { useSSE } from '@/composables/useSSE'
import { useUiPreferences } from '@/composables/useUiPreferences'

const route = useRoute()
const auth = useAuthStore()
const network = useNetworkStore()
const system = useSystemStore()
const collapsed = ref(false)
const mobileSidebarOpen = ref(false)
const stream = useSSE()
const preferences = useUiPreferences()
const colorScheme = window.matchMedia('(prefers-color-scheme: light)')
let refreshTimer: ReturnType<typeof setInterval> | null = null

watch(
  () => route.path,
  () => {
    mobileSidebarOpen.value = false
  },
)

watch(
  () => [preferences.state.theme, preferences.state.accent],
  () => preferences.applyTheme(),
  { immediate: true },
)

async function loadOverview() {
  await Promise.all([system.refresh(), network.refreshInterfaces(), network.refreshClients()])
}

watch(
  () => auth.authenticated,
  (authenticated) => {
    if (authenticated) {
      stream.connect()
      void loadOverview()
      refreshTimer = setInterval(() => void loadOverview(), 15_000)
    } else {
      stream.disconnect()
      if (refreshTimer !== null) clearInterval(refreshTimer)
      refreshTimer = null
    }
  },
)

function handleUnauthorized() {
  auth.clearSession()
}

function handleSystemThemeChange() {
  if (preferences.state.theme === 'system') preferences.applyTheme()
}

onMounted(async () => {
  window.addEventListener('r3p:unauthorized', handleUnauthorized)
  colorScheme.addEventListener('change', handleSystemThemeChange)
  await auth.checkSession()
})

onUnmounted(() => {
  window.removeEventListener('r3p:unauthorized', handleUnauthorized)
  colorScheme.removeEventListener('change', handleSystemThemeChange)
  stream.disconnect()
  if (refreshTimer !== null) clearInterval(refreshTimer)
})
</script>

<template>
  <main v-if="auth.checking" class="min-h-full flex items-center justify-center text-sm text-gray-400">
    Checking router session…
  </main>
  <LoginView v-else-if="!auth.authenticated" />
  <div v-else class="flex h-screen overflow-hidden relative">
    <!-- Desktop Persistent Sidebar (Hidden on Mobile 9:16) -->
    <AppSidebar
      :collapsed="collapsed"
      class="hidden lg:flex"
      @toggle="collapsed = !collapsed"
    />

    <!-- Mobile Slide-Over Drawer for 9:16 Screens -->
    <div
      v-if="mobileSidebarOpen"
      class="fixed inset-0 z-50 lg:hidden flex"
    >
      <!-- Backdrop -->
      <div
        class="fixed inset-0 bg-black/75 backdrop-blur-sm transition-opacity"
        @click="mobileSidebarOpen = false"
      ></div>
      <!-- Drawer Panel -->
      <div class="relative w-[285px] max-w-[85vw] h-full z-10 shadow-2xl flex flex-col bg-[#0b0f19]">
        <AppSidebar
          :collapsed="false"
          :is-mobile="true"
          @close="mobileSidebarOpen = false"
        />
      </div>
    </div>

    <!-- Main Workspace Column -->
    <div class="flex min-w-0 flex-1 flex-col">
      <AppTopbar @open-mobile-sidebar="mobileSidebarOpen = true" />
      <div class="min-h-0 flex-1 overflow-y-auto pb-16 lg:pb-0">
        <RouterView />
      </div>
    </div>

    <!-- Mobile Bottom Navigation Bar (1-Thumb Access in 9:16 Portrait) -->
    <nav class="lg:hidden fixed bottom-0 left-0 right-0 z-40 bg-[#0d121e]/95 backdrop-blur-md border-t border-white/10 h-14 flex items-center justify-around px-1 text-[11px] font-medium select-none shadow-2xl">
      <router-link
        to="/"
        class="flex flex-col items-center justify-center flex-1 py-1 transition cursor-pointer"
        :class="route.path === '/' ? 'text-sky-400 font-bold' : 'text-gray-400 hover:text-gray-200'"
      >
        <span class="text-base leading-none">📊</span>
        <span class="text-[10px] tracking-tight mt-0.5">Tổng Quan</span>
      </router-link>
      <router-link
        to="/ai"
        class="flex flex-col items-center justify-center flex-1 py-1 transition cursor-pointer"
        :class="route.path === '/ai' ? 'text-sky-400 font-bold' : 'text-gray-400 hover:text-gray-200'"
      >
        <span class="text-base leading-none">🤖</span>
        <span class="text-[10px] tracking-tight mt-0.5">Trợ Lý AI</span>
      </router-link>
      <router-link
        to="/network"
        class="flex flex-col items-center justify-center flex-1 py-1 transition cursor-pointer"
        :class="route.path === '/network' ? 'text-sky-400 font-bold' : 'text-gray-400 hover:text-gray-200'"
      >
        <span class="text-base leading-none">🌐</span>
        <span class="text-[10px] tracking-tight mt-0.5">Mạng</span>
      </router-link>
      <router-link
        to="/downloads"
        class="flex flex-col items-center justify-center flex-1 py-1 transition cursor-pointer"
        :class="route.path === '/downloads' ? 'text-sky-400 font-bold' : 'text-gray-400 hover:text-gray-200'"
      >
        <span class="text-base leading-none">⬇️</span>
        <span class="text-[10px] tracking-tight mt-0.5">Download</span>
      </router-link>
      <button
        type="button"
        class="flex flex-col items-center justify-center flex-1 py-1 text-gray-400 hover:text-sky-400 transition cursor-pointer"
        @click="mobileSidebarOpen = true"
      >
        <span class="text-base leading-none">☰</span>
        <span class="text-[10px] tracking-tight mt-0.5">Tất Cả Tab</span>
      </button>
    </nav>
  </div>
</template>
