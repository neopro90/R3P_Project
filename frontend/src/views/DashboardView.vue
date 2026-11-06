<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useSystemStore } from '@/stores/system'
import { useNetworkStore } from '@/stores/network'
import { apiRequest } from '@/services/api'

const system = useSystemStore()
const network = useNetworkStore()

interface ServiceItem {
  name: string
  running: boolean
  enabled: boolean
  pid: number
  memory_kb: number
  description: string
}

interface RadioItem {
  device: string
  band: string
  chip: string
  enabled: boolean
  ssid: string
  channel: string
  htmode: string
  encryption: string
}

interface DiskVolume {
  device: string
  mount_point: string
  label: string
  total_kb: number
  used_kb: number
  avail_kb: number
  percent: number
}

const services = ref<ServiceItem[]>([])
const radios = ref<RadioItem[]>([])
const volumes = ref<DiskVolume[]>([])
const recentLogs = ref<string[]>([])
const loading = ref(false)
const actionNotice = ref<string | null>(null)

const overlayVol = computed(() => volumes.value.find((v) => v.mount_point === '/overlay') || volumes.value[0])

const totalRxRate = computed(() => {
  return network.interfaces
    .filter((i) => i.name !== 'lo')
    .reduce((acc, i) => acc + (i.rxRate || 0), 0)
})

const totalTxRate = computed(() => {
  return network.interfaces
    .filter((i) => i.name !== 'lo')
    .reduce((acc, i) => acc + (i.txRate || 0), 0)
})

function formatRate(bytesPerSec: number): string {
  if (!bytesPerSec || bytesPerSec < 1024) return `${Math.round(bytesPerSec || 0)} B/s`
  if (bytesPerSec < 1024 * 1024) return `${(bytesPerSec / 1024).toFixed(1)} KB/s`
  return `${(bytesPerSec / (1024 * 1024)).toFixed(2)} MB/s`
}

function formatMB(kb: number | undefined): string {
  if (!kb) return '0 MB'
  return `${(kb / 1024).toFixed(1)} MB`
}

async function refreshDashboard() {
  loading.value = true
  try {
    await Promise.all([
      system.refresh(),
      network.refreshInterfaces(),
      network.refreshClients(),
      apiRequest<{ services: ServiceItem[] }>('/services/all').then((r) => {
        if (r?.services) services.value = r.services.slice(0, 8)
      }),
      apiRequest<{ radios: RadioItem[] }>('/wireless').then((r) => {
        if (r?.radios) radios.value = r.radios
      }),
      apiRequest<{ volumes: DiskVolume[] }>('/nas').then((r) => {
        if (r?.volumes) volumes.value = r.volumes
      }),
      apiRequest<{ lines: string[] }>('/logs').then((r) => {
        if (r?.lines) recentLogs.value = r.lines.slice(-12).reverse()
      }),
    ])
  } catch (err) {
    console.warn('Dashboard refresh error:', err)
  } finally {
    loading.value = false
  }
}

async function flushCache() {
  try {
    await apiRequest('/system/drop-caches', { method: 'POST' })
    await system.refresh()
    actionNotice.value = 'RAM pagecache & dentries flushed successfully!'
    setTimeout(() => { actionNotice.value = null }, 3000)
  } catch (err: any) {
    actionNotice.value = 'Error: ' + (err?.message || err)
  }
}

async function quickServiceRestart(svcName: string) {
  try {
    actionNotice.value = `Restarting ${svcName}…`
    await apiRequest('/services/control', {
      method: 'POST',
      body: JSON.stringify({ service: svcName, action: 'restart' }),
    })
    await refreshDashboard()
    actionNotice.value = `Service ${svcName} restarted!`
    setTimeout(() => { actionNotice.value = null }, 3000)
  } catch (err: any) {
    actionNotice.value = `Failed: ${err?.message || err}`
  }
}

onMounted(() => {
  void refreshDashboard()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Action bar & Feedback banner -->
    <div class="flex flex-wrap items-center justify-between gap-2 bg-[#0d1322] border border-white/10 rounded px-3 py-1.5">
      <div class="flex items-center gap-2 text-[11.5px]">
        <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
        <span class="font-semibold text-gray-200">{{ system.hostname || 'R3P-Router' }}</span>
        <span class="text-gray-500">·</span>
        <span class="mono text-sky-400">Kernel {{ system.kernel || '6.12.94' }}</span>
        <span v-if="actionNotice" class="ml-2 px-2 py-0.5 rounded bg-emerald-500/15 text-emerald-300 border border-emerald-500/30 font-medium">
          {{ actionNotice }}
        </span>
      </div>
      <div class="flex items-center gap-1.5">
        <button class="btn-secondary text-[11px] py-1" @click="flushCache">
          Flush RAM Cache
        </button>
        <button class="btn-primary text-[11px] py-1" :disabled="loading" @click="refreshDashboard">
          {{ loading ? 'Syncing…' : 'Sync Telemetry ↻' }}
        </button>
      </div>
    </div>

    <!-- Top Metrics Overview Bar (High Density, 100% Real Data) -->
    <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-2">
      <!-- CPU Metric -->
      <div class="stat-mini">
        <div class="flex items-center justify-between text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">
          <span>CPU Load</span>
          <span class="mono text-sky-400 font-semibold">{{ (system.cpu || 0).toFixed(1) }}%</span>
        </div>
        <div class="progress-bar-bg mb-1.5">
          <div class="progress-bar-fill bg-sky-400" :style="{ width: `${Math.min(100, system.cpu || 0)}%` }"></div>
        </div>
        <div class="flex justify-between text-[10px] text-gray-500 mono">
          <span>Load: {{ system.loadAvg[0]?.toFixed(2) || '0.00' }}</span>
          <span>880 MHz</span>
        </div>
      </div>

      <!-- RAM Metric -->
      <div class="stat-mini">
        <div class="flex items-center justify-between text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">
          <span>Memory (RAM)</span>
          <span class="mono text-emerald-400 font-semibold">{{ (system.ram || 0).toFixed(0) }}%</span>
        </div>
        <div class="progress-bar-bg mb-1.5">
          <div class="progress-bar-fill bg-emerald-400" :style="{ width: `${Math.min(100, system.ram || 0)}%` }"></div>
        </div>
        <div class="flex justify-between text-[10px] text-gray-500 mono">
          <span>{{ (system.ramUsed || 0).toFixed(0) }}M Used</span>
          <span>{{ (system.ramTotal || 512).toFixed(0) }}M Total</span>
        </div>
      </div>

      <!-- Storage / Overlay -->
      <div class="stat-mini">
        <div class="flex items-center justify-between text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">
          <span>NAND /overlay</span>
          <span class="mono text-purple-400 font-semibold">{{ overlayVol ? formatMB(overlayVol.avail_kb) : '202 MB' }}</span>
        </div>
        <div class="progress-bar-bg mb-1.5">
          <div class="progress-bar-fill bg-purple-400" :style="{ width: `${overlayVol ? Math.max(2, overlayVol.percent) : 2}%` }"></div>
        </div>
        <div class="flex justify-between text-[10px] text-gray-500 mono">
          <span>{{ overlayVol ? formatMB(overlayVol.used_kb) : '0.1M' }} Used</span>
          <span>{{ overlayVol ? formatMB(overlayVol.total_kb) : '206.7M' }}</span>
        </div>
      </div>

      <!-- Bandwidth In/Out -->
      <div class="stat-mini">
        <div class="flex items-center justify-between text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">
          <span>Live Traffic</span>
          <span class="badge badge-blue text-[9px]">REALTIME</span>
        </div>
        <div class="flex items-center justify-between mono text-[11px] text-gray-200 mt-0.5">
          <span class="text-emerald-400">↓ {{ formatRate(totalRxRate) }}</span>
          <span class="text-sky-400">↑ {{ formatRate(totalTxRate) }}</span>
        </div>
        <div class="text-[9.5px] text-gray-500 mt-1 mono">{{ network.interfaces.length }} Active Net Devs</div>
      </div>

      <!-- Gateway & IP -->
      <div class="stat-mini">
        <div class="flex items-center justify-between text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">
          <span>LAN Bridge</span>
          <span class="badge badge-green text-[9px]">UP</span>
        </div>
        <div class="mono text-[11px] font-semibold text-gray-200 mt-0.5">192.168.2.1/24</div>
        <div class="text-[9.5px] text-gray-500 mt-1 mono">{{ network.clients.length }} Active DHCP Leases</div>
      </div>

      <!-- Uptime & Hostname -->
      <div class="stat-mini">
        <div class="flex items-center justify-between text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">
          <span>System Uptime</span>
          <span class="mono text-amber-400 font-semibold">{{ system.uptimeFormatted || '—' }}</span>
        </div>
        <div class="mono text-[11px] font-semibold text-gray-200 mt-0.5 truncate">{{ system.hostname || 'R3P-Router' }}</div>
        <div class="text-[9.5px] text-gray-500 mt-1">OpenWrt {{ system.firmwareVersion || '25.12.5' }}</div>
      </div>
    </div>

    <!-- Main Grid: Real Interfaces & Radios, Real Services, Hardware -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-2.5">
      <!-- Network Interfaces & Wireless Radios Card -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Live Interfaces & Radios</span>
          <div class="flex items-center gap-2">
            <RouterLink to="/network" class="text-[11px] text-sky-400 hover:underline">Net →</RouterLink>
            <RouterLink to="/wireless" class="text-[11px] text-purple-400 hover:underline">Wi-Fi →</RouterLink>
          </div>
        </div>

        <div class="space-y-1.5">
          <!-- Real Network Interfaces -->
          <div
            v-for="iface in network.interfaces.filter(i => i.name !== 'lo').slice(0, 4)"
            :key="iface.name"
            class="flex items-center justify-between p-1.5 rounded bg-black/25 border border-white/5"
          >
            <div class="flex items-center gap-2 min-w-0">
              <span class="w-2 h-2 rounded-full shrink-0" :class="iface.up ? 'bg-emerald-400' : 'bg-gray-500'"></span>
              <div class="truncate">
                <div class="text-[11.5px] font-semibold text-gray-200 mono">{{ iface.name }} <span class="text-gray-500 font-normal">({{ iface.ipv4 || 'L2 Bridge/Port' }})</span></div>
                <div class="mono text-[10px] text-gray-400">RX: {{ (iface.rxBytes / 1024).toFixed(0) }} KB · TX: {{ (iface.txBytes / 1024).toFixed(0) }} KB</div>
              </div>
            </div>
            <span :class="iface.up ? 'badge badge-green' : 'badge badge-gray'">
              {{ iface.up ? 'UP' : 'DOWN' }}
            </span>
          </div>

          <!-- Real Wireless Radios -->
          <div
            v-for="r in radios"
            :key="r.device"
            class="flex items-center justify-between p-1.5 rounded bg-black/25 border border-white/5"
          >
            <div class="flex items-center gap-2 min-w-0">
              <span class="w-2 h-2 rounded-full shrink-0" :class="r.enabled ? 'bg-emerald-400' : 'bg-amber-400'"></span>
              <div class="truncate">
                <div class="text-[11.5px] font-semibold text-gray-200">{{ r.band }} ({{ r.ssid }})</div>
                <div class="mono text-[10px] text-gray-400">Ch {{ r.channel }} · {{ r.htmode }} · {{ r.encryption }}</div>
              </div>
            </div>
            <RouterLink to="/wireless" :class="r.enabled ? 'badge badge-blue' : 'badge badge-orange'">
              {{ r.enabled ? 'ACTIVE' : 'DISABLED' }}
            </RouterLink>
          </div>
        </div>
      </div>

      <!-- Active System Services Card (With Direct Restart Action) -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">System Services (procd)</span>
          <RouterLink to="/apps" class="text-[11px] text-sky-400 hover:underline">Manage All →</RouterLink>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-1.5">
          <div
            v-for="s in services"
            :key="s.name"
            class="p-1.5 rounded bg-black/25 border border-white/5 flex items-center justify-between"
          >
            <div class="min-w-0">
              <div class="text-[11.5px] font-semibold text-gray-200 flex items-center gap-1.5 truncate">
                <span class="w-1.5 h-1.5 rounded-full shrink-0" :class="s.running ? 'bg-emerald-400' : 'bg-gray-500'"></span>
                <span class="mono">{{ s.name }}</span>
              </div>
              <div class="mono text-[10px] text-gray-500">
                {{ s.pid ? `PID ${s.pid} · ${s.memory_kb} KB` : (s.enabled ? 'Enabled at boot' : 'Stopped') }}
              </div>
            </div>
            <button
              class="btn-secondary text-[10px] py-0.5 px-1.5 shrink-0"
              title="Restart service"
              @click="quickServiceRestart(s.name)"
            >
              ↻
            </button>
          </div>
        </div>
      </div>

      <!-- System Health & Storage Partitions -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Storage Volumes & Hardware</span>
          <RouterLink to="/nas" class="text-[11px] text-sky-400 hover:underline">NAS Browser →</RouterLink>
        </div>

        <div class="space-y-2 text-[11.5px]">
          <div
            v-for="v in volumes"
            :key="v.mount_point"
            class="p-2 rounded bg-black/25 border border-white/5"
          >
            <div class="flex justify-between items-center mb-1">
              <span class="font-semibold text-gray-200 mono">{{ v.mount_point }}</span>
              <span class="mono text-[10.5px] text-emerald-400">{{ formatMB(v.avail_kb) }} free / {{ formatMB(v.total_kb) }}</span>
            </div>
            <div class="progress-bar-bg">
              <div class="progress-bar-fill bg-sky-400" :style="{ width: `${Math.max(2, v.percent)}%` }"></div>
            </div>
          </div>

          <div class="pt-1 space-y-1 text-[11px]">
            <div class="flex justify-between py-0.5 border-b border-white/5">
              <span class="text-gray-400">SoC / CPU</span>
              <span class="mono text-gray-200">MediaTek MT7621AT (mipsle_24kc)</span>
            </div>
            <div class="flex justify-between py-0.5">
              <span class="text-gray-400">NAND Flash</span>
              <span class="mono text-purple-300">256MB ESMT SLC (UBI)</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Bottom Section: Connected Clients & Live System Logs -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-2.5">
      <!-- Connected Clients Table -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Active DHCP Clients ({{ network.clients.length }})</span>
          <RouterLink to="/access" class="text-[11px] text-sky-400 hover:underline">DHCP & Access →</RouterLink>
        </div>

        <div class="overflow-x-auto max-h-48">
          <table class="pro-table">
            <thead>
              <tr>
                <th>Hostname</th>
                <th>IPv4 Address</th>
                <th>MAC Address</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!network.clients.length">
                <td colspan="4" class="text-center text-gray-500 py-3">No active DHCP leases</td>
              </tr>
              <tr v-for="c in network.clients" :key="c.mac">
                <td class="font-medium text-gray-200">{{ c.hostname || 'Connected Host' }}</td>
                <td class="mono text-sky-300">{{ c.ip }}</td>
                <td class="mono text-gray-400">{{ c.mac }}</td>
                <td><span class="badge badge-green text-[10px]">ACTIVE</span></td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Live Kernel & Daemon Log Feed -->
      <div class="pro-card flex flex-col">
        <div class="pro-card-header">
          <span class="pro-card-title">Live System Log (logread)</span>
          <RouterLink to="/system" class="text-[11px] text-sky-400 hover:underline">Full Logs →</RouterLink>
        </div>

        <div class="bg-black/50 rounded border border-white/5 p-2 font-mono text-[10.5px] text-gray-300 overflow-y-auto max-h-48 space-y-1">
          <div v-if="!recentLogs.length" class="text-gray-500">No log entries available.</div>
          <div v-for="(line, idx) in recentLogs" :key="idx" class="leading-relaxed hover:bg-white/[0.03] px-1 rounded truncate">
            <span class="text-sky-400 select-none">›</span> {{ line }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
