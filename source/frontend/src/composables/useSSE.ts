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
          mem_total_mb?: number
          mem_used_mb?: number
          temp_celsius?: number | null
          uptime_seconds?: number
          load_avg?: [number, number, number]
          hostname?: string
          kernel?: string
          arch?: string
          firmware_version?: string
        }
        systemStore.updateFromSSE({
          cpu: data.cpu_percent,
          ram: data.mem_used_percent,
          ramTotal: data.mem_total_mb,
          ramUsed: data.mem_used_mb,
          uptime: data.uptime_seconds,
          temp: data.temp_celsius,
          hostname: data.hostname,
          kernel: data.kernel,
          arch: data.arch,
          firmwareVersion: data.firmware_version,
          ...(data.load_avg === undefined ? {} : { loadAvg: data.load_avg }),
        })
        if (data.net_rx_bytes_per_second !== undefined && data.net_tx_bytes_per_second !== undefined) {
          networkStore.updateTrafficRates(data.net_rx_bytes_per_second, data.net_tx_bytes_per_second)
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
  }

  onUnmounted(() => {
    disconnect()
  })

  return { isConnected, error, connect, disconnect }
}
