import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { apiRequest } from '@/services/api'

export interface InternetStatus {
  online: boolean
  external_ip: string
  isp?: string
  country?: string
  wan_mode: string
  wan_mode_key?: string
  wan_iface: string
  wan_local_ip: string
  gateway: string
  latency_ms: number
  checked_at: string
}

export interface TemperatureItem {
  zone: string
  type: string
  temp_celsius: number
  temp_milli?: number
  disk_state?: string
  status?: string
}

interface SystemResponse {
  hostname: string
  kernel: string
  arch: string
  version?: string
  hardware?: string
  lan_ip?: string
  timezone?: string
  firmware_version: string
  openwrt: Record<string, string>
  cpu_aggregate_pct: number
  uptime_seconds: number
  load_avg: { load1: number; load5: number; load15: number }
  temperatures: TemperatureItem[]
  internet?: InternetStatus
}

interface MemoryResponse {
  total_kb: number
  physical_total_kb?: number
  reserved_kb?: number
  used_kb: number
  used_percent: number
}

export interface SystemState {
  cpu: number
  cpuHistory: number[]
  ram: number
  ramTotal: number
  ramPhysical: number
  ramReserved: number
  ramUsed: number
  temp: number | null
  uptime: number
  loadAvg: [number, number, number]
  hostname: string
  kernel: string
  arch: string
  appVersion: string
  hardware: string
  lanIp: string
  firmwareVersion: string
  connected: boolean
  lastUpdate: number
  internet?: InternetStatus
}

export const useSystemStore = defineStore('system', () => {
  const cpu = ref(0)
  const cpuHistory = ref<number[]>(Array(60).fill(0))
  const ram = ref(0)
  const ramTotal = ref(0)
  const ramPhysical = ref(0)
  const ramReserved = ref(0)
  const ramUsed = ref(0)
  const temp = ref<number | null>(null)
  const temperatures = ref<TemperatureItem[]>([])
  const uptime = ref(0)
  const loadAvg = ref<[number, number, number]>([0, 0, 0])
  const hostname = ref('—')
  const kernel = ref('—')
  const arch = ref('—')
  const appVersion = ref('—')
  const hardware = ref('MT7621')
  const lanIp = ref(typeof window !== 'undefined' ? window.location.hostname : '')
  const timezone = ref('')
  const firmwareVersion = ref('—')
  const connected = ref(false)
  const lastUpdate = ref(0)
  const error = ref<string | null>(null)
  const internet = ref<InternetStatus>({
    online: false,
    external_ip: '—',
    wan_mode: 'detecting',
    wan_iface: '',
    wan_local_ip: '',
    gateway: '',
    latency_ms: 0,
    checked_at: '',
  })

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
    if (data.ramPhysical !== undefined) ramPhysical.value = data.ramPhysical
    if (data.ramReserved !== undefined) ramReserved.value = data.ramReserved
    if (data.ramUsed !== undefined) ramUsed.value = data.ramUsed
    if (data.temp !== undefined) temp.value = data.temp
    if (data.uptime !== undefined) uptime.value = data.uptime
    if (data.loadAvg !== undefined) loadAvg.value = data.loadAvg
    if (data.hostname !== undefined) hostname.value = data.hostname
    if (data.kernel !== undefined) kernel.value = data.kernel
    if (data.arch !== undefined) arch.value = data.arch
    if (data.appVersion !== undefined && data.appVersion) appVersion.value = data.appVersion
    if (data.hardware !== undefined && data.hardware) hardware.value = data.hardware
    if (data.lanIp !== undefined && data.lanIp) lanIp.value = data.lanIp
    if (data.firmwareVersion !== undefined) firmwareVersion.value = data.firmwareVersion
    if (data.internet !== undefined) internet.value = data.internet
    connected.value = true
    lastUpdate.value = Date.now()
  }

  function setConnected(val: boolean) {
    connected.value = val
  }

  async function refreshInternet(force = false) {
    try {
      const res = await apiRequest<InternetStatus>(`/network/internet${force ? '?refresh=1' : ''}`)
      if (res) internet.value = res
    } catch {
      // ignore
    }
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
      ramPhysical.value = memory.physical_total_kb
        ? Math.round(memory.physical_total_kb / 1024)
        : ramTotal.value
      ramReserved.value = memory.reserved_kb !== undefined
        ? Math.round(memory.reserved_kb / 1024)
        : Math.max(0, ramPhysical.value - ramTotal.value)
      ramUsed.value = Math.round(memory.used_kb / 1024)
      ram.value = Math.round(memory.used_percent)
      temperatures.value = system.temperatures || []
      temp.value = (system.temperatures ?? []).length > 0
        ? system.temperatures[0].temp_celsius
        : null
      uptime.value = system.uptime_seconds
      loadAvg.value = [system.load_avg.load1, system.load_avg.load5, system.load_avg.load15]
      hostname.value = system.hostname || '—'
      kernel.value = system.kernel || '—'
      arch.value = system.arch || '—'
      if (system.version) appVersion.value = system.version
      if (system.hardware) hardware.value = system.hardware
      if (system.lan_ip) lanIp.value = system.lan_ip
      if (system.timezone) timezone.value = system.timezone
      firmwareVersion.value = system.firmware_version || system.openwrt.DISTRIB_RELEASE || '—'
      if (system.internet) {
        internet.value = system.internet
      }
      lastUpdate.value = Date.now()
    } catch (cause) {
      error.value = (cause as Error).message
    }
  }

  return {
    cpu, cpuHistory, ram, ramTotal, ramPhysical, ramReserved, ramUsed, temp, temperatures, uptime,
    loadAvg, hostname, kernel, arch, appVersion, hardware, lanIp, timezone, firmwareVersion, connected, lastUpdate, error,
    internet,
    uptimeFormatted, cpuColor, tempColor, ramPercent, dataAvailable,
    updateFromSSE, setConnected, refresh, refreshInternet,
  }
})
