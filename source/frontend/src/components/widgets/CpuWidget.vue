<script setup lang="ts">
import { computed } from 'vue'
import { useSystemStore } from '@/stores/system'
import LineChart from '@/components/ui/LineChart.vue'

const store = useSystemStore()

const ringPath = computed(() => {
  const r = 40
  const circ = 2 * Math.PI * r
  const pct = store.cpu / 100
  return {
    circ,
    dashoffset: circ * (1 - pct),
  }
})
</script>

<template>
  <div class="glass-card p-5 flex flex-col gap-4">
    <div class="flex items-center justify-between">
      <p class="text-xs font-semibold uppercase tracking-widest" style="color: rgba(255,255,255,0.4);">CPU Usage</p>
      <span class="text-xs px-2 py-0.5 rounded-full" style="background: rgba(255,255,255,0.06); color: rgba(255,255,255,0.5);">
        Load: {{ store.dataAvailable ? store.loadAvg[0].toFixed(2) : '—' }}
      </span>
    </div>

    <div class="flex items-center gap-5">
      <!-- Ring -->
      <div class="relative shrink-0 w-24 h-24">
        <svg width="96" height="96" viewBox="0 0 96 96" class="-rotate-90">
          <circle cx="48" cy="48" r="40" fill="none" stroke="rgba(255,255,255,0.06)" stroke-width="8" />
          <circle
            cx="48" cy="48" r="40"
            fill="none"
            :stroke="store.cpuColor"
            stroke-width="8"
            stroke-linecap="round"
            :stroke-dasharray="ringPath.circ"
            :stroke-dashoffset="ringPath.dashoffset"
            style="transition: stroke-dashoffset 0.5s ease, stroke 0.5s ease;"
          />
        </svg>
        <div class="absolute inset-0 flex flex-col items-center justify-center">
          <span class="text-2xl font-bold tabular-nums" :style="`color: ${store.cpuColor};`">{{ store.dataAvailable ? store.cpu : '—' }}</span>
          <span v-if="store.dataAvailable" class="text-xs" style="color: rgba(255,255,255,0.4);">%</span>
        </div>
      </div>

      <!-- Sparkline -->
      <div class="flex-1 min-w-0">
        <LineChart v-if="store.dataAvailable" :data="store.cpuHistory" :color="store.cpuColor" unit="%" label="CPU" height="80px" />
        <p v-else class="text-xs text-gray-500">Waiting for telemetry</p>
      </div>
    </div>
  </div>
</template>
