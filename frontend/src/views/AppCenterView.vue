<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'

interface RuntimeModule {
  id: string
  status: string
  detail: string
}

interface ModulesResponse {
  modules: RuntimeModule[]
}

const modules = ref<RuntimeModule[]>([])
const loading = ref(false)
const busyAction = ref('')
const error = ref<string | null>(null)
const notice = ref<string | null>(null)

const moduleInfo = [
  { id: 'ssh', name: 'SSH management', group: 'Network services', route: '/system' },
  { id: 'dns', name: 'DNS / DHCP', group: 'Network services', route: '/network' },
  { id: 'adblock', name: 'Adblock Fast', group: 'Network services', route: '' },
  { id: 'downloader', name: 'aria2 downloader', group: 'Mini-server', route: '/downloads' },
  { id: 'telegram', name: 'Telegram URL intake', group: 'Mini-server', route: '/downloads' },
  { id: 'nas', name: 'USB storage / NAS', group: 'Storage', route: '/files' },
  { id: 'ksmbd', name: 'SMB shares (ksmbd)', group: 'Storage', route: '/files' },
  { id: 'photos', name: 'Photo Library', group: 'Storage', route: '/photos' },
]

function moduleFor(id: string) {
  return modules.value.find((module) => module.id === id)
}

function statusLabel(status?: string) {
  const labels: Record<string, string> = {
    active: 'Active',
    ready: 'Ready',
    configured: 'Configured',
    enabled: 'Enabled at boot',
    installed: 'Installed',
    'not-installed': 'Not installed',
    'not-configured': 'Not configured',
  }
  return status ? labels[status] ?? status : 'Unknown'
}

async function refresh() {
  loading.value = true
  error.value = null
  try {
    const response = await apiRequest<ModulesResponse>('/modules')
    modules.value = response.modules
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Module status is unavailable.'
    modules.value = []
  } finally {
    loading.value = false
  }
}

async function adblockAction(action: 'start' | 'enable' | 'disable') {
  busyAction.value = action
  error.value = null
  notice.value = null
  try {
    await apiRequest('/services/adblock-fast', {
      method: 'PATCH',
      body: JSON.stringify({ action }),
    })
    notice.value = action === 'start'
      ? 'Adblock Fast start was requested. DNS filtering runtime remains separately reported.'
      : `Adblock Fast ${action} completed.`
    await refresh()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Adblock Fast action failed.'
  } finally {
    busyAction.value = ''
  }
}

onMounted(refresh)
</script>

<template>
  <section class="h-full overflow-y-auto p-5 md:p-6 space-y-6">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-sky-300">Platform modules</p>
        <h2 class="mt-1 text-xl font-semibold text-white">Services & app center</h2>
        <p class="mt-1 text-sm text-gray-400">Runtime states are read from the router; installed, enabled, and running are distinct states.</p>
      </div>
      <button class="secondary-button" :disabled="loading" @click="refresh">{{ loading ? 'Checking…' : 'Refresh status' }}</button>
    </header>

    <p v-if="error" role="alert" class="glass-card p-4 text-sm text-red-300">{{ error }}</p>
    <p v-if="notice" role="status" class="glass-card p-4 text-sm text-emerald-300">{{ notice }}</p>

    <div v-if="loading && modules.length === 0" class="glass-card p-5 text-sm text-gray-400">Reading module state…</div>
    <div v-else class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      <article v-for="item in moduleInfo" :key="item.id" class="glass-card min-w-0 p-4">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="text-[0.65rem] font-semibold uppercase tracking-[0.14em] text-gray-500">{{ item.group }}</p>
            <h3 class="mt-1 truncate font-medium text-white">{{ item.name }}</h3>
          </div>
          <span
            class="status-pill shrink-0"
            :class="moduleFor(item.id)?.status === 'active' || moduleFor(item.id)?.status === 'ready'
              ? 'status-pill-online'
              : 'status-pill-muted'"
          >
            {{ statusLabel(moduleFor(item.id)?.status) }}
          </span>
        </div>
        <p class="mt-3 min-h-10 text-xs leading-5 text-gray-400">
          {{ moduleFor(item.id)?.detail ?? 'Module state is unavailable from the router API.' }}
        </p>
        <div class="mt-3 flex flex-wrap gap-2">
          <RouterLink v-if="item.route" class="secondary-button" :to="item.route">Open module</RouterLink>
          <template v-if="item.id === 'adblock' && moduleFor('adblock')?.status !== 'not-installed'">
            <button class="secondary-button" :disabled="busyAction !== ''" @click="adblockAction('start')">
              {{ busyAction === 'start' ? 'Starting…' : 'Refresh blocklist' }}
            </button>
            <button
              v-if="moduleFor('adblock')?.status !== 'enabled'"
              class="secondary-button"
              :disabled="busyAction !== ''"
              @click="adblockAction('enable')"
            >Enable at boot</button>
            <button
              v-else
              class="secondary-button"
              :disabled="busyAction !== ''"
              @click="adblockAction('disable')"
            >Disable at boot</button>
          </template>
        </div>
      </article>
    </div>

    <section class="glass-card p-5">
      <h3 class="font-semibold text-white">Deliberate platform limits</h3>
      <p class="mt-2 text-sm leading-6 text-gray-400">
        Docker is not included: the R3P is a 512 MB MIPS router, and this firmware uses a low-overhead USB Photo Library instead.
        SMB packages are staged, but ksmbd shares remain disabled until a share and credentials are configured. Never expose an unauthenticated share.
      </p>
      <p class="mt-2 text-xs text-gray-500">
        Home Assistant/MQTT, package installation from this UI, and unrestricted shell access are not enabled.
      </p>
    </section>
  </section>
</template>
