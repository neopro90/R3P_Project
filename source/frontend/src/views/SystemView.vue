<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'

interface SystemInfo {
  hostname: string
  kernel: string
  openwrt: Record<string, string>
  cpu_aggregate_pct: number
  uptime_seconds: number
  load_avg: { load1: number; load5: number; load15: number }
}

interface MemoryInfo {
  total_kb: number
  used_kb: number
  used_percent: number
  available_kb: number
  buffers_kb: number
  cached_kb: number
  swap_total_kb: number
  swap_used_kb: number
}

interface StorageInfo {
  usb_mounts: Array<{ device: string; mount_point: string; fstype: string }>
  df_output: string[]
}

const system = ref<SystemInfo | null>(null)
const memory = ref<MemoryInfo | null>(null)
const storage = ref<StorageInfo | null>(null)
const error = ref<string | null>(null)
const loading = ref(true)

async function load() {
  loading.value = true
  error.value = null
  try {
    const results = await Promise.all([
      apiRequest<SystemInfo>('/system'),
      apiRequest<MemoryInfo>('/memory'),
      apiRequest<StorageInfo>('/storage'),
    ])
    system.value = results[0]
    memory.value = results[1]
    storage.value = results[2]
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not load system information.'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="h-full overflow-y-auto p-6 space-y-5">
    <div class="flex items-center justify-between">
      <p class="text-sm text-gray-400">Live information from OpenWrt system interfaces.</p>
      <button class="rounded-lg border border-white/10 px-3 py-2 text-sm text-gray-300 hover:text-white" @click="load">
        Refresh
      </button>
    </div>
    <p v-if="loading" class="text-sm text-gray-400">Loading system information…</p>
    <p v-else-if="error" role="alert" class="glass-card p-4 text-sm text-red-300">{{ error }}</p>
    <template v-else-if="system && memory && storage">
      <div class="grid gap-4 md:grid-cols-2">
        <section class="glass-card p-5 space-y-3">
          <h2 class="font-semibold text-white">System</h2>
          <p class="text-sm text-gray-300">Hostname: {{ system.hostname || 'Unavailable' }}</p>
          <p class="text-sm text-gray-300">Kernel: {{ system.kernel || 'Unavailable' }}</p>
          <p class="text-sm text-gray-300">OpenWrt: {{ system.openwrt.DISTRIB_DESCRIPTION || 'Unavailable' }}</p>
          <p class="text-sm text-gray-300">Uptime: {{ Math.floor(system.uptime_seconds / 3600) }} h</p>
          <p class="text-sm text-gray-300">
            Load: {{ system.load_avg.load1.toFixed(2) }} / {{ system.load_avg.load5.toFixed(2) }} / {{ system.load_avg.load15.toFixed(2) }}
          </p>
        </section>
        <section class="glass-card p-5 space-y-3">
          <h2 class="font-semibold text-white">Memory, cache & swap</h2>
          <p class="text-sm text-gray-300">Used: {{ (memory.used_kb / 1024).toFixed(1) }} / {{ (memory.total_kb / 1024).toFixed(1) }} MiB</p>
          <p class="text-sm text-gray-300">Utilization: {{ memory.used_percent.toFixed(1) }}%</p>
          <p class="text-sm text-gray-300">Available: {{ (memory.available_kb / 1024).toFixed(1) }} MiB</p>
          <p class="text-sm text-gray-300">Page cache: {{ (memory.cached_kb / 1024).toFixed(1) }} MiB · buffers: {{ (memory.buffers_kb / 1024).toFixed(1) }} MiB</p>
          <p class="text-sm text-gray-300">
            Swap: {{ (memory.swap_used_kb / 1024).toFixed(1) }} / {{ (memory.swap_total_kb / 1024).toFixed(1) }} MiB
            <span class="text-xs text-gray-500">(zram is not enabled by this image)</span>
          </p>
        </section>
        <section class="glass-card p-5 space-y-3 md:col-span-2">
          <h2 class="font-semibold text-white">USB storage mounts</h2>
          <p v-if="storage.usb_mounts.length === 0" class="text-sm text-gray-400">No USB storage mount detected.</p>
          <ul v-else class="space-y-2">
            <li v-for="mount in storage.usb_mounts" :key="`${mount.device}:${mount.mount_point}`" class="text-sm text-gray-300">
              {{ mount.device }} · {{ mount.mount_point }} · {{ mount.fstype }}
            </li>
          </ul>
        </section>
      </div>
    </template>
  </section>
</template>
