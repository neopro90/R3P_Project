import { ref, onUnmounted } from 'vue'
import { useSystemStore } from '@/stores/system'
import { useNetworkStore } from '@/stores/network'
import { apiURL } from '@/services/api'

const SSE_URL = apiURL('/monitor/stream')

export function useSSE() {
  const systemStore = useSystemStore()
  const networkStore = useNetworkStore()
  const isConnected = ref(false)
  const error = ref<string | null>(null)
  let evtSource: EventSource | null = null

  function connect() {
    if (evtSource !== null) return
    evtSource = new EventSource(SSE_URL, { withCredentials: true })
    evtSource.onopen = () => {
      isConnected.value = true
      error.value = null
      systemStore.setConnected(true)
    }
    evtSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as {
          cpu_percent?: number
          mem_used_percent?: number
          net_rx_bytes_per_second?: number
          net_tx_bytes_per_second?: number
          wan_rx_bytes_per_second?: number
          wan_tx_bytes_per_second?: number
          mem_total_mb?: number
          mem_physical_mb?: number
          mem_reserved_mb?: number
          mem_used_mb?: number
          temp_celsius?: number | null
          uptime_seconds?: number
          load_avg?: [number, number, number]
          hostname?: string
          kernel?: string
          arch?: string
          version?: string
          hardware?: string
          lan_ip?: string
          firmware_version?: string
          internet?: import('@/stores/system').InternetStatus
        }
        systemStore.updateFromSSE({
          cpu: data.cpu_percent,
          ram: data.mem_used_percent,
          ramTotal: data.mem_total_mb !== undefined ? Math.round(data.mem_total_mb) : undefined,
          ramPhysical: data.mem_physical_mb !== undefined ? Math.round(data.mem_physical_mb) : undefined,
          ramReserved: data.mem_reserved_mb !== undefined ? Math.round(data.mem_reserved_mb) : undefined,
          ramUsed: data.mem_used_mb !== undefined ? Math.round(data.mem_used_mb) : undefined,
          uptime: data.uptime_seconds,
          temp: data.temp_celsius,
          hostname: data.hostname,
          kernel: data.kernel,
          arch: data.arch,
          appVersion: data.version,
          hardware: data.hardware,
          lanIp: data.lan_ip,
          firmwareVersion: data.firmware_version,
          internet: data.internet,
          ...(data.load_avg === undefined ? {} : { loadAvg: data.load_avg }),
        })
        const rx = data.internet?.wan_iface && data.wan_rx_bytes_per_second !== undefined
          ? data.wan_rx_bytes_per_second
          : data.net_rx_bytes_per_second
        const tx = data.internet?.wan_iface && data.wan_tx_bytes_per_second !== undefined
          ? data.wan_tx_bytes_per_second
          : data.net_tx_bytes_per_second
        if (rx !== undefined && tx !== undefined) {
          networkStore.updateTrafficRates(rx, tx)
        }
      } catch (cause) {
        error.value = `Invalid monitor event: ${(cause as Error).message}`
        console.error('[SSE] Failed to parse event data', cause)
      }
    }
    evtSource.onerror = () => {
      isConnected.value = false
      systemStore.setConnected(false)
      error.value = 'Live updates are disconnected; retrying automatically.'
    }
  }

  function disconnect() {
    evtSource?.close()
    evtSource = null
    isConnected.value = false
    systemStore.setConnected(false)
  }

  onUnmounted(() => {
    disconnect()
  })

  return { isConnected, error, connect, disconnect }
}
