import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { apiRequest } from '@/services/api'

interface SystemResponse {
  hostname: string
  kernel: string
  arch: string
  firmware_version: string
  openwrt: Record<string, string>
  cpu_aggregate_pct: number
  uptime_seconds: number
  load_avg: { load1: number; load5: number; load15: number }
  temperatures: Array<{ temp_celsius: number }>
}

interface MemoryResponse {
  total_kb: number
  used_kb: number
  used_percent: number
}

export interface SystemState {
  cpu: number
  cpuHistory: number[]
  ram: number
  ramTotal: number
  ramUsed: number
  temp: number | null
  uptime: number
  loadAvg: [number, number, number]
  hostname: string
  kernel: string
  arch: string
  firmwareVersion: string
  connected: boolean
  lastUpdate: number
}

export const useSystemStore = defineStore('system', () => {
  const cpu = ref(0)
  const cpuHistory = ref<number[]>(Array(60).fill(0))
  const ram = ref(0)
  const ramTotal = ref(0)
  const ramUsed = ref(0)
  const temp = ref<number | null>(null)
  const uptime = ref(0)
  const loadAvg = ref<[number, number, number]>([0, 0, 0])
  const hostname = ref('—')
  const kernel = ref('—')
  const arch = ref('—')
  const firmwareVersion = ref('—')
  const connected = ref(false)
  const lastUpdate = ref(0)
  const error = ref<string | null>(null)

  const dataAvailable = computed(() => lastUpdate.value > 0)

  const uptimeFormatted = computed(() => {
    if (!dataAvailable.value) return '—'
    const s = uptime.value
    const d = Math.floor(s / 86400)
    const h = Math.floor((s % 86400) / 3600)
    const m = Math.floor((s % 3600) / 60)
    if (d > 0) return `${d}d ${h}h ${m}m`
    if (h > 0) return `${h}h ${m}m`
    return `${m}m`
  })

  const cpuColor = computed(() => {
    if (!dataAvailable.value) return 'rgba(255,255,255,0.4)'
    if (cpu.value < 50) return '#68d391'
    if (cpu.value < 80) return '#ed8936'
    return '#fc8181'
  })

  const tempColor = computed(() => {
    if (temp.value === null) return 'rgba(255,255,255,0.4)'
    if (temp.value < 60) return '#68d391'
    if (temp.value < 75) return '#ed8936'
    return '#fc8181'
  })

  const ramPercent = computed(() =>
    ramTotal.value > 0 ? Math.round((ramUsed.value / ramTotal.value) * 100) : 0
  )

  function updateFromSSE(data: Partial<SystemState>) {
    if (data.cpu !== undefined) {
      cpu.value = data.cpu
      cpuHistory.value = [...cpuHistory.value.slice(1), data.cpu]
    }
    if (data.ram !== undefined) ram.value = data.ram
    if (data.ramTotal !== undefined) ramTotal.value = data.ramTotal
    if (data.ramUsed !== undefined) ramUsed.value = data.ramUsed
    if (data.temp !== undefined) temp.value = data.temp
    if (data.uptime !== undefined) uptime.value = data.uptime
    if (data.loadAvg !== undefined) loadAvg.value = data.loadAvg
    if (data.hostname !== undefined) hostname.value = data.hostname
    if (data.kernel !== undefined) kernel.value = data.kernel
    if (data.arch !== undefined) arch.value = data.arch
    if (data.firmwareVersion !== undefined) firmwareVersion.value = data.firmwareVersion
    connected.value = true
    lastUpdate.value = Date.now()
  }

  function setConnected(val: boolean) {
    connected.value = val
  }

  async function refresh() {
    error.value = null
    try {
      const [system, memory] = await Promise.all([
        apiRequest<SystemResponse>('/system'),
        apiRequest<MemoryResponse>('/memory'),
      ])
      cpu.value = Math.round(system.cpu_aggregate_pct)
      ramTotal.value = Math.round(memory.total_kb / 1024)
      ramUsed.value = Math.round(memory.used_kb / 1024)
      ram.value = Math.round(memory.used_percent)
      temp.value = (system.temperatures ?? []).length > 0
        ? system.temperatures[0].temp_celsius
        : null
      uptime.value = system.uptime_seconds
      loadAvg.value = [system.load_avg.load1, system.load_avg.load5, system.load_avg.load15]
      hostname.value = system.hostname || '—'
      kernel.value = system.kernel || '—'
      arch.value = system.arch || '—'
      firmwareVersion.value = system.firmware_version || system.openwrt.DISTRIB_RELEASE || '—'
      lastUpdate.value = Date.now()
    } catch (cause) {
      error.value = (cause as Error).message
    }
  }

  return {
    cpu, cpuHistory, ram, ramTotal, ramUsed, temp, uptime,
    loadAvg, hostname, kernel, arch, firmwareVersion, connected, lastUpdate, error,
    uptimeFormatted, cpuColor, tempColor, ramPercent, dataAvailable,
    updateFromSSE, setConnected, refresh,
  }
})
