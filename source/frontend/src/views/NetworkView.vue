<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useNetworkStore } from '@/stores/network'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import LineChart from '@/components/ui/LineChart.vue'

const store = useNetworkStore()
let refreshTimer: ReturnType<typeof setInterval> | null = null

function fmtBytes(b: number) {
  if (b < 1024) return `${b} B`
  if (b < 1024 ** 2) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 ** 3) return `${(b / 1024 ** 2).toFixed(2)} MB`
  return `${(b / 1024 ** 3).toFixed(2)} GB`
}

function fmtRate(bps: number) {
  if (bps < 1024) return `${bps} B/s`
  if (bps < 1024 * 1024) return `${(bps / 1024).toFixed(1)} KB/s`
  return `${(bps / 1024 / 1024).toFixed(2)} MB/s`
}

const typeColors: Record<string, string> = {
  wan: 'var(--accent-blue)',
  lan: 'var(--accent-green)',
  wlan: 'var(--accent-purple)',
  loopback: 'rgba(255,255,255,0.3)',
  other: 'var(--accent-teal)',
}

const typeIcons: Record<string, string> = {
  wan: '🌐', lan: '🔌', wlan: '📶', loopback: '🔄', other: '🔗',
}

onMounted(() => {
  void store.refreshInterfaces()
  refreshTimer = setInterval(() => void store.refreshInterfaces(), 15_000)
})

onUnmounted(() => {
  if (refreshTimer !== null) clearInterval(refreshTimer)
})
</script>

<template>
  <div class="p-6 space-y-6 overflow-y-auto h-full">
    <p v-if="store.error" role="alert" class="glass-card p-3 text-sm text-red-300">{{ store.error }}</p>
    <div class="grid grid-cols-1 gap-4">
      <div
        v-for="iface in store.interfaces"
        :key="iface.name"
        class="glass-card p-5"
        :style="`border-color: ${typeColors[iface.type] ?? 'rgba(255,255,255,0.08)'}22;`"
      >
        <div class="flex items-start justify-between gap-4 mb-4">
          <div class="flex items-center gap-3">
            <span class="text-2xl">{{ typeIcons[iface.type] ?? '🔗' }}</span>
            <div>
              <h3 class="text-base font-semibold text-white">{{ iface.name }}</h3>
              <p class="text-xs" style="color: rgba(255,255,255,0.4);">{{ iface.type.toUpperCase() }}</p>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <StatusBadge
              :status="iface.up === undefined ? 'unknown' : iface.up ? 'online' : 'offline'"
              :label="iface.up === undefined ? 'UNKNOWN' : iface.up ? 'UP' : 'DOWN'"
            />
          </div>
        </div>

        <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 text-sm mb-4">
          <div>
            <p class="text-xs mb-1" style="color: rgba(255,255,255,0.35);">IPv4</p>
            <p class="font-medium text-white">{{ iface.ipv4 || '—' }}</p>
          </div>
          <div>
            <p class="text-xs mb-1" style="color: rgba(255,255,255,0.35);">MAC</p>
            <p class="font-medium text-white font-mono text-xs">{{ iface.mac || '—' }}</p>
          </div>
          <div>
            <p class="text-xs mb-1" style="color: var(--accent-green);">↓ RX</p>
            <p class="font-medium" style="color: var(--accent-green);">{{ fmtRate(iface.rxRate) }}</p>
            <p class="text-xs" style="color: rgba(255,255,255,0.3);">Total: {{ fmtBytes(iface.rxBytes) }}</p>
          </div>
          <div>
            <p class="text-xs mb-1" style="color: var(--accent-blue);">↑ TX</p>
            <p class="font-medium" style="color: var(--accent-blue);">{{ fmtRate(iface.txRate) }}</p>
            <p class="text-xs" style="color: rgba(255,255,255,0.3);">Total: {{ fmtBytes(iface.txBytes) }}</p>
          </div>
        </div>

        <!-- Mini charts -->
        <div class="grid grid-cols-2 gap-3">
          <LineChart :data="store.rxHistory" color="#68d391" label="RX" height="50px" :formatter="fmtRate" />
          <LineChart :data="store.txHistory" color="#63b3ed" label="TX" height="50px" :formatter="fmtRate" />
        </div>
      </div>

      <div v-if="store.interfaces.length === 0" class="glass-card p-10 text-center" style="color: rgba(255,255,255,0.3);">
        No interfaces detected. Waiting for data...
      </div>
    </div>
  </div>
</template>
