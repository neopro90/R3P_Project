<script setup lang="ts">
import { onMounted, onUnmounted, ref, computed } from 'vue'
import { useNetworkStore } from '@/stores/network'
import StatusBadge from '@/components/ui/StatusBadge.vue'

const store = useNetworkStore()
const search = ref('')
let refreshTimer: ReturnType<typeof setInterval> | null = null

const filtered = computed(() => {
  const q = search.value.toLowerCase()
  if (!q) return store.clients
  return store.clients.filter(c =>
    c.hostname.toLowerCase().includes(q) ||
    c.ip.includes(q) ||
    c.mac.toLowerCase().includes(q)
  )
})

function fmtBytes(b: number) {
  if (b < 1024) return `${b} B`
  if (b < 1024 ** 2) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 ** 3) return `${(b / 1024 ** 2).toFixed(2)} MB`
  return `${(b / 1024 ** 3).toFixed(2)} GB`
}

onMounted(() => {
  void store.refreshClients()
  refreshTimer = setInterval(() => void store.refreshClients(), 15_000)
})

onUnmounted(() => {
  if (refreshTimer !== null) clearInterval(refreshTimer)
})

function ifaceIcon(iface: string) {
  if (iface.startsWith('wlan')) return '📶'
  return '🔌'
}

</script>

<template>
  <div class="p-6 space-y-5 overflow-y-auto h-full">
    <p class="text-xs text-gray-500">Lease activity is not a real-time Wi-Fi association list.</p>
    <p v-if="store.error" role="alert" class="glass-card p-3 text-sm text-red-300">{{ store.error }}</p>
    <!-- Search bar -->
    <div class="glass-card px-4 py-2 flex items-center gap-3">
      <span class="text-gray-400">🔍</span>
      <input
        v-model="search"
        type="text"
        placeholder="Search by hostname, IP, or MAC..."
        class="flex-1 bg-transparent outline-none text-sm text-white placeholder-gray-500"
      />
      <span v-if="search" class="text-gray-400 cursor-pointer hover:text-white" @click="search = ''">✕</span>
    </div>

    <!-- Stats row -->
    <div class="flex gap-4 text-sm flex-wrap">
      <div class="glass-card px-4 py-2 flex items-center gap-2">
        <span style="color: var(--accent-green);">●</span>
        <span class="text-white font-semibold">{{ store.clientCount }}</span>
        <span style="color: rgba(255,255,255,0.4);">active leases</span>
      </div>
      <div class="glass-card px-4 py-2 flex items-center gap-2">
        <span style="color: rgba(255,255,255,0.3);">●</span>
        <span class="text-white font-semibold">{{ store.clients.length - store.clientCount }}</span>
        <span style="color: rgba(255,255,255,0.4);">expired leases</span>
      </div>
      <div class="glass-card px-4 py-2 flex items-center gap-2">
        <span style="color: var(--accent-blue);">≡</span>
        <span class="text-white font-semibold">{{ store.clients.length }}</span>
        <span style="color: rgba(255,255,255,0.4);">leases</span>
      </div>
    </div>

    <!-- Table -->
    <div class="glass-card overflow-hidden">
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid rgba(255,255,255,0.06);">
            <th class="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wider" style="color: rgba(255,255,255,0.35);">Device</th>
            <th class="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wider" style="color: rgba(255,255,255,0.35);">IP Address</th>
            <th class="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wider hidden md:table-cell" style="color: rgba(255,255,255,0.35);">MAC</th>
            <th class="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wider hidden lg:table-cell" style="color: rgba(255,255,255,0.35);">Interface</th>
            <th class="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wider hidden lg:table-cell" style="color: rgba(255,255,255,0.35);">RX / TX</th>
            <th class="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wider" style="color: rgba(255,255,255,0.35);">Status</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="client in filtered"
            :key="client.mac"
            class="transition-colors duration-100"
            style="border-bottom: 1px solid rgba(255,255,255,0.04);"
            @mouseover="($event.currentTarget as HTMLElement).style.background = 'var(--bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = ''"
          >
            <td class="px-4 py-3">
              <div class="flex items-center gap-2">
                <span>{{ ifaceIcon(client.interface) }}</span>
                <span class="text-white font-medium">{{ client.hostname || '—' }}</span>
              </div>
            </td>
            <td class="px-4 py-3 font-mono text-xs" style="color: var(--accent-blue);">{{ client.ip }}</td>
            <td class="px-4 py-3 font-mono text-xs hidden md:table-cell" style="color: rgba(255,255,255,0.4);">{{ client.mac }}</td>
            <td class="px-4 py-3 hidden lg:table-cell" style="color: rgba(255,255,255,0.4);">{{ client.interface }}</td>
            <td class="px-4 py-3 hidden lg:table-cell text-xs" style="color: rgba(255,255,255,0.35);">
              {{ client.rxBytes === undefined ? '—' : fmtBytes(client.rxBytes) }} / {{ client.txBytes === undefined ? '—' : fmtBytes(client.txBytes) }}
            </td>
            <td class="px-4 py-3">
              <StatusBadge
                :status="client.leaseActive ? 'online' : 'offline'"
                :label="client.leaseActive ? 'ACTIVE' : 'EXPIRED'"
              />
            </td>
          </tr>
        </tbody>
      </table>
      <div v-if="filtered.length === 0" class="text-center py-10" style="color: rgba(255,255,255,0.3);">
        No clients match your search
      </div>
    </div>
  </div>
</template>
