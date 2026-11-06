<script setup lang="ts">
defineProps<{
  status: 'online' | 'offline' | 'warning' | 'unknown'
  label?: string
}>()

const colorMap = {
  online:  { bg: 'rgba(104,211,145,0.15)', color: 'var(--accent-green)',  dot: 'var(--accent-green)' },
  offline: { bg: 'rgba(252,129,129,0.15)', color: 'var(--accent-red)',    dot: 'var(--accent-red)' },
  warning: { bg: 'rgba(237,137,54,0.15)',  color: 'var(--accent-orange)', dot: 'var(--accent-orange)' },
  unknown: { bg: 'rgba(255,255,255,0.08)', color: 'rgba(255,255,255,0.4)', dot: 'rgba(255,255,255,0.3)' },
}
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium"
    :style="`background: ${colorMap[status].bg}; color: ${colorMap[status].color};`"
  >
    <span
      class="w-1.5 h-1.5 rounded-full shrink-0"
      :style="`background: ${colorMap[status].dot};`"
    ></span>
    {{ label ?? status }}
  </span>
</template>
