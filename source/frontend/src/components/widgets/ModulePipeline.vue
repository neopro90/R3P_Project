<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { apiRequest } from '@/services/api'

type ModuleState = 'active' | 'ready' | 'configured' | 'enabled' | 'installed' | 'not-installed' | 'not-configured' | 'unavailable' | 'checking'

interface RuntimeModule {
  id: string
  status: ModuleState
  detail: string
}

interface ModulesResponse {
  modules: RuntimeModule[]
}

interface PipelineNode {
  id: string
  title: string
  route: string
  status: ModuleState
  detail: string
  x: number
  y: number
}

const router = useRouter()
const canvas = ref<HTMLElement | null>(null)
const modules = ref<RuntimeModule[]>([])
const loading = ref(false)
const unavailable = ref(false)
const canvasWidth = ref(0)
let resizeObserver: ResizeObserver | null = null
let refreshTimer: ReturnType<typeof setInterval> | null = null

const flow = [
  { id: 'telegram', title: 'Telegram', route: '/downloads' },
  { id: 'parser', title: 'URL parser', route: '/downloads' },
  { id: 'downloader', title: 'aria2', route: '/downloads' },
  { id: 'nas', title: 'USB / NAS', route: '/files' },
  { id: 'photos', title: 'Photo Library', route: '/photos' },
]

const supportModules = [
  { id: 'ssh', title: 'SSH / Dropbear', route: '/apps' },
  { id: 'dns', title: 'DNS / dnsmasq', route: '/apps' },
  { id: 'adblock', title: 'Adblock Fast', route: '/apps' },
  { id: 'ksmbd', title: 'SMB / ksmbd', route: '/apps' },
]

const isCompact = computed(() => canvasWidth.value < 700)
const viewBox = computed(() => isCompact.value ? '0 0 360 760' : '0 0 1000 390')

function moduleFor(id: string): RuntimeModule {
  if (id === 'parser') {
    return {
      id,
      status: 'ready',
      detail: 'Built-in parser extracts HTTP(S) and magnet links from the allowlisted Telegram chat.',
    }
  }
  const module = modules.value.find((item) => item.id === id)
  if (module) return module
  if (loading.value) {
    return { id, status: 'checking', detail: 'Checking module status with the router API.' }
  }
  if (unavailable.value) {
    return { id, status: 'unavailable', detail: 'The router API did not report this module status.' }
  }
  return { id, status: 'unavailable', detail: 'No module status has been reported by the router API.' }
}

const flowNodes = computed<PipelineNode[]>(() => {
  const compact = isCompact.value
  const step = compact ? 118 : 195
  const start = compact ? 56 : 115
  return flow.map((item, index) => ({
    ...item,
    ...moduleFor(item.id),
    x: compact ? 180 : start + step * index,
    y: compact ? 56 + 108 * index : 100,
  }))
})

const serviceNodes = computed<PipelineNode[]>(() => supportModules.map((item, index) => ({
  ...item,
  ...moduleFor(item.id),
  x: isCompact.value ? 92 + (index % 2) * 176 : 130 + index * 247,
  y: isCompact.value ? 630 + Math.floor(index / 2) * 76 : 310,
})))

const flowEdges = computed(() => flowNodes.value.slice(0, -1).map((node, index) => {
  const next = flowNodes.value[index + 1]
  if (isCompact.value) {
    const y = node.y + 31
    return { id: `${node.id}-${next.id}`, d: `M180 ${y} V${next.y - 31}` }
  }
  const x = node.x + 82
  return { id: `${node.id}-${next.id}`, d: `M${x} 100 H${next.x - 82}` }
}))

const serviceEdges = computed(() => serviceNodes.value.map((node) => {
  if (isCompact.value) {
    const row = Math.floor(supportModules.findIndex((service) => service.id === node.id) / 2)
    const rowY = row === 0 ? 590 : 654
    return { id: `core-${node.id}`, d: `M${node.x} ${rowY} V${node.y - 28}` }
  }
  return { id: `core-${node.id}`, d: `M${node.x} 252 V282` }
}))

const serviceBusEdges = computed(() => isCompact.value
  ? [
      { id: 'core-services-trunk', d: 'M180 519 V590 M92 590 H268 M180 590 V654 M92 654 H268' },
    ]
  : [{ id: 'core-services-bus', d: 'M130 252 H871' }])

function statusLabel(status: ModuleState) {
  const labels: Record<ModuleState, string> = {
    active: 'Active',
    ready: 'Ready',
    configured: 'Configured',
    enabled: 'Enabled at boot',
    installed: 'Installed',
    'not-installed': 'Not installed',
    'not-configured': 'Not configured',
    unavailable: 'Unavailable',
    checking: 'Checking…',
  }
  return labels[status]
}

function activateNode(node: PipelineNode) {
  void router.push(node.route)
}

function handleNodeKeydown(event: KeyboardEvent, node: PipelineNode) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    activateNode(node)
  }
}

async function refresh() {
  loading.value = true
  try {
    const response = await apiRequest<ModulesResponse>('/modules')
    modules.value = response.modules
    unavailable.value = false
  } catch (cause) {
    unavailable.value = true
    modules.value = []
    console.warn('Could not read runtime module states.', cause)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (canvas.value) {
    resizeObserver = new ResizeObserver((entries) => {
      canvasWidth.value = entries[0]?.contentRect.width ?? 0
    })
    resizeObserver.observe(canvas.value)
  }
  void refresh()
  refreshTimer = setInterval(() => void refresh(), 15_000)
})

onUnmounted(() => {
  resizeObserver?.disconnect()
  if (refreshTimer !== null) clearInterval(refreshTimer)
})
</script>

<template>
  <section class="glass-card module-pipeline" aria-label="Mini-server module pipeline">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-sky-300">Module pipeline</p>
        <h3 class="mt-1 text-base font-semibold text-white">Message → download → library</h3>
        <p class="mt-1 text-xs text-gray-400">
          Select a node to open its module. Connections show the logical integration; badges show reported readiness.
        </p>
      </div>
      <button class="secondary-button" :disabled="loading" @click="refresh">
        {{ loading ? 'Checking…' : 'Refresh modules' }}
      </button>
    </div>

    <p v-if="unavailable" role="status" class="mt-4 rounded-lg border border-amber-400/20 bg-amber-400/5 p-3 text-xs text-amber-200">
      Router module states are unavailable. This host is not reporting live OpenWrt service data.
    </p>

    <div ref="canvas" class="pipeline-canvas mt-4">
      <svg
        class="pipeline-map"
        :viewBox="viewBox"
        role="group"
        aria-label="Interactive mini-server module topology"
      >
        <path v-for="edge in flowEdges" :key="edge.id" :d="edge.d" class="pipeline-edge" />
        <path v-for="edge in serviceBusEdges" :key="edge.id" :d="edge.d" class="pipeline-edge pipeline-edge-support" />
        <path v-for="edge in serviceEdges" :key="edge.id" :d="edge.d" class="pipeline-edge pipeline-edge-support" />
        <text
          :x="isCompact ? 180 : 500"
          :y="isCompact ? 570 : 239"
          text-anchor="middle"
          class="pipeline-map-status pipeline-core-label"
        >Router core services</text>

        <g
          v-for="node in flowNodes"
          :key="node.id"
          class="pipeline-map-node"
          :class="`pipeline-map-${node.status}`"
          role="link"
          tabindex="0"
          :aria-label="`${node.title}: ${statusLabel(node.status)}. ${node.detail}`"
          @click="activateNode(node)"
          @keydown="handleNodeKeydown($event, node)"
        >
          <title>{{ node.title }} — {{ statusLabel(node.status) }}. {{ node.detail }}</title>
          <rect :x="node.x - 82" :y="node.y - 31" width="164" height="62" rx="14" />
          <circle :cx="node.x - 65" :cy="node.y - 10" r="4" class="pipeline-map-dot" />
          <text :x="node.x" :y="node.y - 2" text-anchor="middle" class="pipeline-map-title">{{ node.title }}</text>
          <text :x="node.x" :y="node.y + 17" text-anchor="middle" class="pipeline-map-status" :class="`pipeline-status-text-${node.status}`">{{ statusLabel(node.status) }}</text>
        </g>

        <g
          v-for="node in serviceNodes"
          :key="node.id"
          class="pipeline-map-node pipeline-map-service"
          :class="`pipeline-map-${node.status}`"
          role="link"
          tabindex="0"
          :aria-label="`${node.title}: ${statusLabel(node.status)}. ${node.detail}`"
          @click="activateNode(node)"
          @keydown="handleNodeKeydown($event, node)"
        >
          <title>{{ node.title }} — {{ statusLabel(node.status) }}. {{ node.detail }}</title>
          <rect :x="node.x - 82" :y="node.y - 28" width="164" height="56" rx="13" />
          <circle :cx="node.x - 65" :cy="node.y - 8" r="4" class="pipeline-map-dot" />
          <text :x="node.x" :y="node.y - 1" text-anchor="middle" class="pipeline-map-title">{{ node.title }}</text>
          <text :x="node.x" :y="node.y + 17" text-anchor="middle" class="pipeline-map-status" :class="`pipeline-status-text-${node.status}`">{{ statusLabel(node.status) }}</text>
        </g>
      </svg>
    </div>
    <ul class="pipeline-status-legend" aria-label="Module status legend">
      <li><span class="pipeline-legend-dot pipeline-legend-active" />Active</li>
      <li><span class="pipeline-legend-dot pipeline-legend-ready" />Ready</li>
      <li><span class="pipeline-legend-dot pipeline-legend-installed" />Installed / enabled</li>
      <li><span class="pipeline-legend-dot pipeline-legend-unconfigured" />Not configured</li>
      <li><span class="pipeline-legend-dot pipeline-legend-unavailable" />Unavailable</li>
    </ul>
    <p class="mt-2 text-[0.68rem] text-gray-500">Logical module links only; no live download traffic is implied.</p>
  </section>
</template>
