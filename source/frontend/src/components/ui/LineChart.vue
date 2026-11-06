<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  data: number[]
  color?: string
  unit?: string
  height?: string
  label?: string
  showArea?: boolean
  formatter?: (val: number) => string
}>()

const stroke = computed(() => props.color || '#63b3ed')

const points = computed(() => {
  if (props.data.length === 0) return []
  const min = Math.min(0, ...props.data)
  const max = Math.max(...props.data)
  const range = max - min || 1
  const width = 1000
  const height = 100
  return props.data.map((value, index) => ({
    x: props.data.length === 1 ? width : (index / (props.data.length - 1)) * width,
    y: height - ((value - min) / range) * (height - 8) - 4,
  }))
})

const linePath = computed(() =>
  points.value.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x} ${point.y}`).join(' '),
)

const areaPath = computed(() => {
  if (!props.showArea || points.value.length === 0) return ''
  return `${linePath.value} L 1000 100 L 0 100 Z`
})

const latest = computed(() => {
  const value = props.data.at(-1)
  if (value === undefined) return 'No samples'
  if (props.formatter) return props.formatter(value)
  return props.unit ? `${value}${props.unit}` : String(value)
})
</script>

<template>
  <div :style="{ height: height ?? '80px', width: '100%' }">
    <svg
      class="h-full w-full overflow-visible"
      viewBox="0 0 1000 100"
      preserveAspectRatio="none"
      role="img"
      :aria-label="`${label ?? 'Metric'} history: ${latest}`"
    >
      <title>{{ label ? `${label}: ` : '' }}{{ latest }}</title>
      <path v-if="areaPath" :d="areaPath" :fill="stroke" fill-opacity="0.12" />
      <path
        v-if="linePath"
        :d="linePath"
        fill="none"
        :stroke="stroke"
        stroke-width="2"
        vector-effect="non-scaling-stroke"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
    </svg>
  </div>
</template>
