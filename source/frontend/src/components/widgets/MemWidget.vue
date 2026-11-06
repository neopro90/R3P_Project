<script setup lang="ts">
import { computed } from 'vue'
import { useSystemStore } from '@/stores/system'

const store = useSystemStore()

const barColor = computed(() => {
  const pct = store.ramPercent
  if (pct < 60) return 'var(--accent-green)'
  if (pct < 85) return 'var(--accent-orange)'
  return 'var(--accent-red)'
})

function fmtMB(mb: number) {
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`
  return `${mb} MB`
}
</script>

<template>
  <div class="glass-card p-5 flex flex-col gap-4">
    <div class="flex items-center justify-between">
      <p class="text-xs font-semibold uppercase tracking-widest" style="color: rgba(255,255,255,0.4);">Memory</p>
      <span class="text-xs" style="color: rgba(255,255,255,0.4);">
        {{ store.dataAvailable ? `${fmtMB(store.ramUsed)} / ${fmtMB(store.ramTotal)}` : 'Waiting for telemetry' }}
      </span>
    </div>

    <div class="flex items-end justify-between gap-2">
      <span class="text-4xl font-bold tabular-nums" :style="`color: ${barColor};`">{{ store.dataAvailable ? store.ramPercent : '—' }}</span>
      <span v-if="store.dataAvailable" class="text-lg mb-1" style="color: rgba(255,255,255,0.4);">%</span>
      <span class="flex-1"></span>
      <span class="text-xs mb-1" style="color: rgba(255,255,255,0.35);">RAM</span>
    </div>

    <!-- Bar -->
    <div class="h-2.5 rounded-full overflow-hidden" style="background: rgba(255,255,255,0.06);">
      <div
        class="h-full rounded-full transition-all duration-500"
        :style="`width: ${store.dataAvailable ? store.ramPercent : 0}%; background: ${barColor};`"
      ></div>
    </div>

    <!-- Segments -->
    <div v-if="store.dataAvailable" class="flex justify-between text-xs" style="color: rgba(255,255,255,0.3);">
      <span>0</span>
      <span>{{ fmtMB(store.ramTotal / 2) }}</span>
      <span>{{ fmtMB(store.ramTotal) }}</span>
    </div>
    <p v-else class="text-xs text-gray-500">Memory data unavailable</p>
  </div>
</template>
