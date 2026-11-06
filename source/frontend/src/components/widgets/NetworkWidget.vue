<script setup lang="ts">
import { computed } from 'vue'
import { useNetworkStore } from '@/stores/network'
import LineChart from '@/components/ui/LineChart.vue'

const store = useNetworkStore()

function fmtRate(bps: number) {
  if (bps < 1024) return `${bps} B/s`
  if (bps < 1024 * 1024) return `${(bps / 1024).toFixed(1)} KB/s`
  return `${(bps / 1024 / 1024).toFixed(2)} MB/s`
}

const totalRx = computed(() => store.totalRxRate)
const totalTx = computed(() => store.totalTxRate)
</script>

<template>
  <div class="glass-card p-5 flex flex-col gap-4">
    <div class="flex items-center justify-between">
      <p class="text-xs font-semibold uppercase tracking-widest" style="color: rgba(255,255,255,0.4);">Network I/O</p>
      <span class="text-xs px-2 py-0.5 rounded-full" style="background: rgba(255,255,255,0.06); color: rgba(255,255,255,0.5);">
        WAN: {{ store.wanIp }}
      </span>
    </div>

    <div class="grid grid-cols-2 gap-4">
      <div>
        <p class="text-xs mb-1" style="color: var(--accent-green);">↓ Download</p>
        <p class="text-xl font-bold tabular-nums" style="color: var(--accent-green);">{{ store.trafficAvailable ? fmtRate(totalRx) : '—' }}</p>
        <LineChart v-if="store.trafficAvailable" :data="store.rxHistory" color="#68d391" label="RX" height="60px" :formatter="fmtRate" />
      </div>
      <div>
        <p class="text-xs mb-1" style="color: var(--accent-blue);">↑ Upload</p>
        <p class="text-xl font-bold tabular-nums" style="color: var(--accent-blue);">{{ store.trafficAvailable ? fmtRate(totalTx) : '—' }}</p>
        <LineChart v-if="store.trafficAvailable" :data="store.txHistory" color="#63b3ed" label="TX" height="60px" :formatter="fmtRate" />
      </div>
    </div>
    <p v-if="store.interfaces.length === 0" class="text-xs text-gray-500">Interface traffic is not available.</p>
  </div>
</template>
