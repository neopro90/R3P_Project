import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { apiRequest } from '@/services/api'

export interface InterfaceStats {
  name: string
  rxBytes: number
  txBytes: number
  rxRate: number    // bytes/s
  txRate: number    // bytes/s
  type: 'wan' | 'lan' | 'wlan' | 'loopback' | 'other'
  ipv4?: string
  ipv6?: string
  mac?: string
  up: boolean | undefined
}

export interface Client {
  ip: string
  mac: string
  hostname: string
  interface: string
  rxBytes?: number
  txBytes?: number
  leaseActive: boolean
  expiry?: number
}

interface NetworkResponse {
  interfaces: Array<{
    iface: string
    rx_bytes: number
    tx_bytes: number
    rx_bytes_per_second: number
    tx_bytes_per_second: number
    type?: InterfaceStats['type']
    ipv4?: string
    ipv6?: string
    mac?: string
    up?: boolean
  }>
}

interface ClientsResponse {
  leases: Array<{
    expiry: number
    mac: string
    ip: string
    hostname: string
    lease_active: boolean
  }>
}

export const useNetworkStore = defineStore('network', () => {
  const interfaces = ref<InterfaceStats[]>([])
  const rxHistory = ref<number[]>(Array(60).fill(0))
  const txHistory = ref<number[]>(Array(60).fill(0))
  const clients = ref<Client[]>([])
  const clientsAvailable = ref(false)
  const totalRx = ref(0)
  const totalTx = ref(0)
  const liveRxRate = ref(0)
  const liveTxRate = ref(0)
  const trafficAvailable = ref(false)
  const interfaceError = ref<string | null>(null)
  const clientError = ref<string | null>(null)
  const error = computed(() => interfaceError.value ?? clientError.value)

  const wanIface = computed(() =>
    interfaces.value.find(i => i.type === 'wan')
  )

  const wanIp = computed(() => wanIface.value?.ipv4 || '—')

  const totalRxRate = computed(() => liveRxRate.value)
  const totalTxRate = computed(() => liveTxRate.value)

  const clientCount = computed(() => clients.value.filter(c => c.leaseActive).length)

  function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B/s`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB/s`
    return `${(bytes / 1024 / 1024).toFixed(2)} MB/s`
  }

  function updateInterfaces(data: InterfaceStats[]) {
    interfaces.value = data
    const activeInterfaces = data.filter(i => i.type !== 'loopback')
    const rx = activeInterfaces.reduce((s, i) => s + i.rxRate, 0)
    const tx = activeInterfaces.reduce((s, i) => s + i.txRate, 0)
    liveRxRate.value = rx
    liveTxRate.value = tx
    trafficAvailable.value = activeInterfaces.length > 0
    rxHistory.value = [...rxHistory.value.slice(1), rx]
    txHistory.value = [...txHistory.value.slice(1), tx]
    totalRx.value = data.reduce((s, i) => s + i.rxBytes, 0)
    totalTx.value = data.reduce((s, i) => s + i.txBytes, 0)
  }

  function updateClients(data: Client[]) {
    clients.value = data
    clientsAvailable.value = true
  }

  function updateTrafficRates(rx: number, tx: number) {
    liveRxRate.value = rx
    liveTxRate.value = tx
    trafficAvailable.value = true
    rxHistory.value = [...rxHistory.value.slice(1), rx]
    txHistory.value = [...txHistory.value.slice(1), tx]
  }

  function interfaceType(name: string): InterfaceStats['type'] {
    const lower = name.toLowerCase()
    if (lower === 'lo') return 'loopback'
    if (lower.includes('wan') || lower.startsWith('pppoe')) return 'wan'
    if (lower.includes('wlan') || lower.includes('phy') || lower.includes('ap')) return 'wlan'
    if (lower.includes('lan') || lower.startsWith('br-')) return 'lan'
    return 'other'
  }

  async function refreshInterfaces() {
    interfaceError.value = null
    try {
      const response = await apiRequest<NetworkResponse>('/network')
      updateInterfaces(response.interfaces.map((iface) => ({
        name: iface.iface,
        rxBytes: iface.rx_bytes,
        txBytes: iface.tx_bytes,
        rxRate: iface.rx_bytes_per_second,
        txRate: iface.tx_bytes_per_second,
        type: iface.type ?? interfaceType(iface.iface),
        ipv4: iface.ipv4,
        ipv6: iface.ipv6,
        mac: iface.mac,
        up: iface.up,
      })))
    } catch (cause) {
      interfaceError.value = (cause as Error).message
    }
  }

  async function refreshClients() {
    clientError.value = null
    try {
      const response = await apiRequest<ClientsResponse>('/clients')
      updateClients(response.leases.map((lease) => ({
        ip: lease.ip,
        mac: lease.mac,
        hostname: lease.hostname,
        interface: 'DHCP lease',
        leaseActive: lease.lease_active,
        expiry: lease.expiry,
      })))
    } catch (cause) {
      clientError.value = (cause as Error).message
      clientsAvailable.value = false
    }
  }

  return {
    interfaces, rxHistory, txHistory, clients, clientsAvailable, totalRx, totalTx, error, trafficAvailable,
    wanIface, wanIp, totalRxRate, totalTxRate, clientCount,
    formatBytes, updateInterfaces, updateClients, updateTrafficRates, refreshInterfaces, refreshClients,
  }
})
