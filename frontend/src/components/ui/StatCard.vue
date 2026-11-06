<script setup lang="ts">
defineProps<{
  title: string
  value: number | string
  unit?: string
  color?: string
  icon?: string
  subtitle?: string
  trend?: 'up' | 'down' | 'neutral'
}>()
</script>

<template>
  <div
    class="glass-card p-5 flex flex-col gap-3 transition-all duration-200"
    :style="`border-color: ${color ? color + '22' : 'rgba(255,255,255,0.08)'};`"
  >
    <!-- Header row -->
    <div class="flex items-start justify-between">
      <p class="text-xs font-medium uppercase tracking-widest" style="color: rgba(255,255,255,0.45);">
        {{ title }}
      </p>
      <span v-if="icon" class="text-xl">{{ icon }}</span>
    </div>

    <!-- Value -->
    <div class="flex items-end gap-1">
      <span
        class="text-4xl font-bold leading-none tabular-nums"
        :style="`color: ${color || 'var(--accent-blue)'};`"
      >
        {{ value }}
      </span>
      <span v-if="unit" class="text-lg mb-0.5" style="color: rgba(255,255,255,0.4);">{{ unit }}</span>
    </div>

    <!-- Subtitle -->
    <p v-if="subtitle" class="text-xs" style="color: rgba(255,255,255,0.35);">{{ subtitle }}</p>

    <!-- Gradient bottom accent -->
    <div
      class="h-0.5 rounded-full mt-auto"
      :style="`background: linear-gradient(to right, ${color || 'var(--accent-blue)'}, transparent);`"
    ></div>
  </div>
</template>
