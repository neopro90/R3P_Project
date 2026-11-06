<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterView } from 'vue-router'
import AppSidebar from '@/components/layout/AppSidebar.vue'
import AppTopbar from '@/components/layout/AppTopbar.vue'
import LoginView from '@/views/LoginView.vue'
import { useAuthStore } from '@/stores/auth'
import { useNetworkStore } from '@/stores/network'
import { useSystemStore } from '@/stores/system'
import { useSSE } from '@/composables/useSSE'
import { useUiPreferences } from '@/composables/useUiPreferences'

const auth = useAuthStore()
const network = useNetworkStore()
const system = useSystemStore()
const collapsed = ref(false)
const stream = useSSE()
const preferences = useUiPreferences()
const colorScheme = window.matchMedia('(prefers-color-scheme: light)')
let refreshTimer: ReturnType<typeof setInterval> | null = null

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
  <div v-else class="flex h-screen overflow-hidden">
    <AppSidebar :collapsed="collapsed" @toggle="collapsed = !collapsed" />
    <div class="flex min-w-0 flex-1 flex-col">
      <AppTopbar />
      <div class="min-h-0 flex-1">
        <RouterView />
      </div>
    </div>
  </div>
</template>
