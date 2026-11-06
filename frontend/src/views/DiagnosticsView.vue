<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'

interface ApiSection<T> {
  data: T | null
  error: string | null
}

interface SystemInfo {
  hostname: string
  kernel: string
  firmware_version: string
  cpu_aggregate_pct: number
  uptime_seconds: number
  load_avg: { load1: number; load5: number; load15: number }
}

interface NetworkInfo {
  interfaces: Array<{ iface: string; up?: boolean; ipv4?: string }>
}

interface ServiceInfo {
  services: Array<{ name: string; running: boolean }>
}

interface LogsInfo {
  source: string
  lines: string[]
}

const system = ref<ApiSection<SystemInfo>>({ data: null, error: null })
const network = ref<ApiSection<NetworkInfo>>({ data: null, error: null })
const services = ref<ApiSection<ServiceInfo>>({ data: null, error: null })
const logs = ref<ApiSection<LogsInfo>>({ data: null, error: null })
const refreshing = ref(false)
const updatedAt = ref<number | null>(null)

async function loadSection<T>(path: string): Promise<ApiSection<T>> {
  try {
    return { data: await apiRequest<T>(path), error: null }
  } catch (cause) {
    return { data: null, error: cause instanceof Error ? cause.message : 'Diagnostic endpoint unavailable.' }
  }
}

async function refresh() {
  refreshing.value = true
  try {
    const [systemResult, networkResult, servicesResult, logsResult] = await Promise.all([
      loadSection<SystemInfo>('/system'),
      loadSection<NetworkInfo>('/network'),
      loadSection<ServiceInfo>('/services'),
      loadSection<LogsInfo>('/logs'),
    ])
    system.value = systemResult
    network.value = networkResult
    services.value = servicesResult
    logs.value = logsResult
    updatedAt.value = Date.now()
  } finally {
    refreshing.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <section class="h-full overflow-y-auto p-5 md:p-6 space-y-5">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="text-xl font-semibold text-white">Diagnostics</h2>
        <p class="mt-1 text-sm text-gray-400">Fixed, read-only checks against existing API endpoints. No arbitrary shell execution.</p>
        <p v-if="updatedAt" class="mt-1 text-xs text-gray-500">Last check: {{ new Date(updatedAt).toLocaleTimeString() }}</p>
      </div>
      <button class="secondary-button" :disabled="refreshing" @click="refresh">
        {{ refreshing ? 'Checking…' : 'Run checks' }}
      </button>
    </header>

    <div class="grid gap-4 xl:grid-cols-2">
      <article class="glass-card p-5 space-y-3">
        <div class="flex items-center justify-between gap-3">
          <h3 class="font-semibold text-white">System health</h3>
          <span class="status-pill" :class="system.data ? 'status-pill-online' : 'status-pill-muted'">
            {{ system.data ? 'Responding' : 'Unavailable' }}
          </span>
        </div>
        <p v-if="system.error" role="status" class="text-sm text-amber-300">{{ system.error }}</p>
        <template v-else-if="system.data">
          <p class="text-sm text-gray-300">{{ system.data.hostname }} · {{ system.data.firmware_version || system.data.kernel }}</p>
          <p class="text-xs text-gray-400">CPU {{ system.data.cpu_aggregate_pct.toFixed(1) }}% · load {{ system.data.load_avg.load1.toFixed(2) }} · uptime {{ Math.floor(system.data.uptime_seconds / 3600) }} h</p>
        </template>
      </article>

      <article class="glass-card p-5 space-y-3">
        <div class="flex items-center justify-between gap-3">
          <h3 class="font-semibold text-white">Network interfaces</h3>
          <span class="status-pill" :class="network.data ? 'status-pill-online' : 'status-pill-muted'">
            {{ network.data ? `${network.data.interfaces.length} found` : 'Unavailable' }}
          </span>
        </div>
        <p v-if="network.error" role="status" class="text-sm text-amber-300">{{ network.error }}</p>
        <ul v-else-if="network.data" class="space-y-2">
          <li v-for="iface in network.data.interfaces" :key="iface.iface" class="flex justify-between gap-3 text-sm">
            <span class="text-gray-200">{{ iface.iface }} <span class="text-xs text-gray-500">{{ iface.ipv4 || 'no IPv4' }}</span></span>
            <span :class="iface.up ? 'text-emerald-300' : 'text-gray-500'">{{ iface.up === undefined ? 'status unknown' : iface.up ? 'up' : 'down' }}</span>
          </li>
        </ul>
      </article>

      <article class="glass-card p-5 space-y-3">
        <div class="flex items-center justify-between gap-3">
          <h3 class="font-semibold text-white">Service process checks</h3>
          <span class="status-pill" :class="services.data ? 'status-pill-online' : 'status-pill-muted'">
            {{ services.data ? `${services.data.services.filter((service) => service.running).length} running` : 'Unavailable' }}
          </span>
        </div>
        <p v-if="services.error" role="status" class="text-sm text-amber-300">{{ services.error }}</p>
        <p v-else class="text-xs text-gray-500">Process presence is a basic check, not a full service health probe.</p>
        <ul v-if="services.data" class="flex flex-wrap gap-2">
          <li v-for="service in services.data.services" :key="service.name" class="status-pill" :class="service.running ? 'status-pill-online' : 'status-pill-muted'">
            {{ service.name }} · {{ service.running ? 'running' : 'not detected' }}
          </li>
        </ul>
      </article>

      <article class="glass-card p-5 space-y-3">
        <div class="flex items-center justify-between gap-3">
          <h3 class="font-semibold text-white">System log tail</h3>
          <span class="text-xs text-gray-500">{{ logs.data?.source || 'Unavailable' }}</span>
        </div>
        <p v-if="logs.error" role="status" class="text-sm text-amber-300">{{ logs.error }}</p>
        <pre v-else-if="logs.data" class="max-h-72 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-black/20 p-3 text-xs leading-5 text-gray-300">{{ logs.data.lines.join('\n') || 'No log lines returned.' }}</pre>
      </article>
    </div>
    <p v-if="!system.data && !network.data && !services.data && !logs.data && !refreshing" class="text-xs text-gray-500">
      No checks succeeded. Verify authentication, router reachability, and OpenWrt API availability.
    </p>
  </section>
</template>
