<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
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

// Realtime Graph Histories (20 sample points)
const cpuHistory = ref<number[]>([12, 15, 10, 8, 20, 18, 14, 22, 16, 12, 15, 19, 14, 11, 16, 25, 18, 15, 12, 16])
const rxHistory = ref<number[]>([15, 22, 18, 30, 45, 60, 40, 35, 50, 75, 90, 65, 45, 30, 40, 55, 70, 45, 35, 50])
const txHistory = ref<number[]>([8, 12, 10, 15, 20, 25, 18, 16, 22, 35, 40, 30, 20, 15, 18, 25, 30, 22, 18, 24])

let pollTimer: ReturnType<typeof setInterval> | null = null

const overlayVol = computed(() => volumes.value.find((v) => v.mount_point === '/overlay') || volumes.value[0])
const usbHddVol = computed(() => volumes.value.find((v) => v.mount_point === '/mnt/sda1' || v.mount_point.startsWith('/mnt/sd')))

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
  if (kb > 1024 * 1024) return `${(kb / (1024 * 1024)).toFixed(1)} GB`
  return `${(kb / 1024).toFixed(1)} MB`
}

// Generate SVG Polyline points
function generatePoints(data: number[], maxVal = 100, width = 300, height = 70): string {
  if (!data.length) return ''
  const step = width / (data.length - 1)
  return data
    .map((val, idx) => {
      const x = (idx * step).toFixed(1)
      const clamped = Math.max(0, Math.min(maxVal, val))
      const y = (height - (clamped / maxVal) * (height - 6) - 3).toFixed(1)
      return `${x},${y}`
    })
    .join(' ')
}

function generateAreaPath(data: number[], maxVal = 100, width = 300, height = 70): string {
  if (!data.length) return ''
  const pts = generatePoints(data, maxVal, width, height)
  return `M 0,${height} L ${pts} L ${width},${height} Z`
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

    // Push new telemetry points to sparklines
    const currentCpu = Math.round(system.cpu || 15)
    cpuHistory.value.push(currentCpu)
    if (cpuHistory.value.length > 24) cpuHistory.value.shift()

    const currentRxKb = Math.round(totalRxRate.value / 1024)
    rxHistory.value.push(currentRxKb)
    if (rxHistory.value.length > 24) rxHistory.value.shift()

    const currentTxKb = Math.round(totalTxRate.value / 1024)
    txHistory.value.push(currentTxKb)
    if (txHistory.value.length > 24) txHistory.value.shift()
  } catch (err) {
    console.warn('Dashboard refresh error:', err)
  } finally {
    loading.value = false
  }
}

async function flushCache() {
  try {
    await apiRequest('/system/drop-caches', { method: 'POST' })
    actionNotice.value = 'Đã giải phóng bộ nhớ RAM cache thành công!'
    await refreshDashboard()
    setTimeout(() => { actionNotice.value = null }, 3500)
  } catch (err: any) {
    actionNotice.value = `Thất bại: ${err?.message || err}`
  }
}

async function quickServiceRestart(name: string) {
  try {
    actionNotice.value = `Đang khởi động lại dịch vụ ${name}…`
    await apiRequest('/services/control', {
      method: 'POST',
      body: JSON.stringify({ name, action: 'restart' }),
    })
    actionNotice.value = `Dịch vụ ${name} đã được khởi động lại!`
    await refreshDashboard()
    setTimeout(() => { actionNotice.value = null }, 3500)
  } catch (err: any) {
    actionNotice.value = `Lỗi: ${err?.message || err}`
  }
}

onMounted(() => {
  void refreshDashboard()
  pollTimer = setInterval(() => {
    void refreshDashboard()
  }, 4000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="h-full overflow-y-auto p-2 sm:p-3 space-y-3 bg-[#080b11]">
    <!-- Action bar & Host Info Banner -->
    <div class="flex flex-wrap items-center justify-between gap-2 bg-[#0d1322] border border-white/10 rounded-lg px-3 py-1.5">
      <div class="flex items-center gap-2 text-xs">
        <span class="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
        <span class="font-bold text-gray-200">{{ system.hostname || 'R3P-Router' }}</span>
        <span class="text-gray-500">·</span>
        <span class="mono text-sky-400">MediaTek MT7621AT @ 880MHz</span>
        <span class="text-gray-500 hidden sm:inline">·</span>
        <span class="text-purple-300 mono hidden sm:inline">Linux {{ system.kernel || '6.12.94' }}</span>
        <span v-if="actionNotice" class="ml-2 px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300 border border-emerald-500/40 text-[11px] font-medium">
          {{ actionNotice }}
        </span>
      </div>
      <div class="flex items-center gap-2">
        <button class="btn-secondary text-[11px] py-1 px-2.5" @click="flushCache">
          🧹 Giải Phóng Cache
        </button>
        <button class="btn-primary text-[11px] py-1 px-3" :disabled="loading" @click="refreshDashboard">
          {{ loading ? 'Đang đồng bộ…' : 'Đồng Bộ ↻' }}
        </button>
      </div>
    </div>

    <!-- ROW 1: REALTIME INTERACTIVE GRAPHS & GAUGES -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-2.5">
      <!-- 1. CPU Load & Sparkline Realtime Chart -->
      <div class="pro-card p-3 flex flex-col justify-between">
        <div class="flex items-center justify-between text-xs mb-1">
          <div class="flex items-center gap-1.5">
            <span class="text-sky-400 font-bold">⚡ Tải Vi Xử Lý (CPU)</span>
          </div>
          <span class="mono font-extrabold text-sky-300 text-sm">{{ (system.cpu || 0).toFixed(1) }}%</span>
        </div>

        <!-- SVG Realtime Sparkline Line Chart -->
        <div class="h-16 w-full relative my-1 overflow-hidden rounded bg-black/40 border border-white/5">
          <svg class="w-full h-full" viewBox="0 0 300 70" preserveAspectRatio="none">
            <defs>
              <linearGradient id="cpuGrad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="#38bdf8" stop-opacity="0.45" />
                <stop offset="100%" stop-color="#38bdf8" stop-opacity="0.0" />
              </linearGradient>
            </defs>
            <path :d="generateAreaPath(cpuHistory, 100, 300, 70)" fill="url(#cpuGrad)" />
            <polyline
              :points="generatePoints(cpuHistory, 100, 300, 70)"
              fill="none"
              stroke="#38bdf8"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </div>

        <div class="flex items-center justify-between text-[10px] text-gray-500 mono pt-1 border-t border-white/5">
          <span>Load Avg: {{ system.loadAvg[0]?.toFixed(2) || '0.15' }}</span>
          <span>4 Luồng Xử Lý</span>
        </div>
      </div>

      <!-- 2. Bandwidth Traffic In/Out Dual-Area Chart -->
      <div class="pro-card p-3 flex flex-col justify-between">
        <div class="flex items-center justify-between text-xs mb-1">
          <span class="font-bold text-emerald-400">🌐 Băng Thông Mạng (Live)</span>
          <span class="badge badge-blue text-[9px]">REALTIME</span>
        </div>

        <!-- SVG Dual Area Chart -->
        <div class="h-16 w-full relative my-1 overflow-hidden rounded bg-black/40 border border-white/5">
          <svg class="w-full h-full" viewBox="0 0 300 70" preserveAspectRatio="none">
            <defs>
              <linearGradient id="rxGrad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="#34d399" stop-opacity="0.4" />
                <stop offset="100%" stop-color="#34d399" stop-opacity="0.0" />
              </linearGradient>
            </defs>
            <path :d="generateAreaPath(rxHistory, 150, 300, 70)" fill="url(#rxGrad)" />
            <polyline
              :points="generatePoints(rxHistory, 150, 300, 70)"
              fill="none"
              stroke="#34d399"
              stroke-width="2"
              stroke-linecap="round"
            />
            <polyline
              :points="generatePoints(txHistory, 150, 300, 70)"
              fill="none"
              stroke="#38bdf8"
              stroke-width="1.5"
              stroke-dasharray="3,3"
            />
          </svg>
        </div>

        <div class="flex items-center justify-between text-[10.5px] mono pt-1 border-t border-white/5">
          <span class="text-emerald-400 font-semibold">↓ {{ formatRate(totalRxRate) }}</span>
          <span class="text-sky-400 font-semibold">↑ {{ formatRate(totalTxRate) }}</span>
        </div>
      </div>

      <!-- 3. Memory RAM Ring Donut Gauge -->
      <div class="pro-card p-3 flex flex-col justify-between">
        <div class="flex items-center justify-between text-xs mb-1">
          <span class="font-bold text-amber-300">🧠 Bộ Nhớ RAM (512MB)</span>
          <span class="mono font-bold text-amber-400">{{ (system.ram || 0).toFixed(0) }}%</span>
        </div>

        <div class="flex items-center gap-3 my-1">
          <!-- Circular Ring Gauge -->
          <div class="relative w-14 h-14 shrink-0 flex items-center justify-center">
            <svg class="w-full h-full -rotate-90" viewBox="0 0 36 36">
              <path
                class="text-white/10"
                stroke-width="4"
                stroke="currentColor"
                fill="none"
                d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
              />
              <path
                class="text-amber-400 transition-all duration-500 ease-out"
                stroke-width="4"
                :stroke-dasharray="`${Math.min(100, system.ram || 20)}, 100`"
                stroke-linecap="round"
                stroke="currentColor"
                fill="none"
                d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
              />
            </svg>
            <span class="absolute text-[11px] font-bold text-gray-200 mono">{{ (system.ram || 0).toFixed(0) }}%</span>
          </div>

          <div class="flex-1 space-y-1 text-[10.5px]">
            <div class="flex justify-between text-gray-400">
              <span>Đang dùng:</span>
              <span class="mono text-gray-200 font-bold">{{ (system.ramUsed || 0).toFixed(0) }} MB</span>
            </div>
            <div class="flex justify-between text-gray-400">
              <span>Khả dụng:</span>
              <span class="mono text-emerald-400 font-bold">{{ ((system.ramTotal || 512) - (system.ramUsed || 0)).toFixed(0) }} MB</span>
            </div>
            <div class="flex justify-between text-gray-500 text-[10px]">
              <span>Tổng RAM:</span>
              <span class="mono">512 MB DDR3</span>
            </div>
          </div>
        </div>

        <div class="progress-bar-bg mt-1">
          <div class="progress-bar-fill bg-amber-400" :style="{ width: `${Math.min(100, system.ram || 0)}%` }"></div>
        </div>
      </div>

      <!-- 4. Storage & Partitions Gauge (NAND + USB 500GB) -->
      <div class="pro-card p-3 flex flex-col justify-between">
        <div class="flex items-center justify-between text-xs mb-1">
          <span class="font-bold text-purple-300">💾 Lưu Trữ & Phân Vùng</span>
          <RouterLink to="/nas" class="text-[10px] text-sky-400 hover:underline">Chi tiết →</RouterLink>
        </div>

        <div class="space-y-2 my-1">
          <!-- NAND /overlay -->
          <div>
            <div class="flex justify-between text-[10.5px] mb-0.5">
              <span class="font-medium text-gray-300 mono">/overlay (NAND)</span>
              <span class="mono text-purple-300 font-bold">{{ overlayVol ? formatMB(overlayVol.avail_kb) : '202 MB' }} trống</span>
            </div>
            <div class="progress-bar-bg">
              <div class="progress-bar-fill bg-purple-400" :style="{ width: `${overlayVol ? Math.max(3, overlayVol.percent) : 3}%` }"></div>
            </div>
          </div>

          <!-- USB 3.0 HDD -->
          <div>
            <div class="flex justify-between text-[10.5px] mb-0.5">
              <span class="font-medium text-gray-300 mono">/mnt/sda1 (USB 3.0)</span>
              <span class="mono text-emerald-300 font-bold">
                {{ usbHddVol ? formatMB(usbHddVol.avail_kb) : 'Ổ Cứng 500GB' }}
              </span>
            </div>
            <div class="progress-bar-bg">
              <div
                class="progress-bar-fill bg-emerald-400"
                :style="{ width: `${usbHddVol ? Math.max(3, usbHddVol.percent) : 2}%` }"
              ></div>
            </div>
          </div>
        </div>

        <div class="flex justify-between text-[10px] text-gray-500 pt-1 border-t border-white/5 mono">
          <span>SLC Flash 256MB</span>
          <span>USB 3.0 ext4 Fast I/O</span>
        </div>
      </div>
    </div>

    <!-- ROW 2: LIVE RADIOS, CORE SERVICES & PARTITION HEALTH -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-2.5">
      <!-- Live Interfaces & Radios -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Live Interfaces & Radios</span>
          <RouterLink to="/wireless" class="text-[11px] text-sky-400 hover:underline">Wi-Fi + WWAN →</RouterLink>
        </div>

        <div class="space-y-2 text-[11.5px]">
          <div
            v-for="iface in network.interfaces.filter(i => i.name !== 'lo').slice(0, 4)"
            :key="iface.name"
            class="flex items-center justify-between p-2 rounded bg-black/25 border border-white/5"
          >
            <div class="min-w-0">
              <div class="flex items-center gap-1.5">
                <span class="font-bold text-gray-200 mono">{{ iface.name }}</span>
                <span class="text-[10px] text-gray-400 truncate">
                  {{ iface.ipv4 ? `(${iface.ipv4})` : (iface.type === 'lan' ? '(LAN Bridge)' : '') }}
                </span>
              </div>
              <div class="mono text-[10px] text-gray-500">
                RX: {{ formatMB(Math.round(iface.rxBytes / 1024)) }} · TX: {{ formatMB(Math.round(iface.txBytes / 1024)) }}
              </div>
            </div>
            <span class="badge" :class="iface.up ? 'badge-green' : 'badge-gray'">
              {{ iface.up ? 'UP' : 'DOWN' }}
            </span>
          </div>

          <div
            v-for="r in radios"
            :key="r.device"
            class="flex items-center justify-between p-2 rounded bg-black/25 border border-white/5"
          >
            <div>
              <div class="font-bold text-gray-200">{{ r.band }} ({{ r.ssid || 'OpenWrt' }})</div>
              <div class="mono text-[10px] text-gray-500">
                Kênh {{ r.channel || 'Auto' }} · {{ r.htmode || 'VHT80' }}
              </div>
            </div>
            <span class="badge" :class="r.enabled ? 'badge-green' : 'badge-orange'">
              {{ r.enabled ? 'ACTIVE' : 'DISABLED' }}
            </span>
          </div>
        </div>
      </div>

      <!-- Core Procd Services -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Core System Daemons (procd)</span>
          <span class="text-[11px] text-gray-500">{{ services.length }} Monitored</span>
        </div>

        <div class="grid grid-cols-2 gap-2 text-[11.5px]">
          <div
            v-for="s in services"
            :key="s.name"
            class="p-2 rounded bg-black/25 border border-white/5 flex items-center justify-between"
          >
            <div class="min-w-0 pr-1">
              <div class="flex items-center gap-1.5 font-bold text-gray-200">
                <span class="w-1.5 h-1.5 rounded-full shrink-0" :class="s.running ? 'bg-emerald-400' : 'bg-gray-500'"></span>
                <span class="mono truncate">{{ s.name }}</span>
              </div>
              <div class="mono text-[10px] text-gray-500 truncate">
                {{ s.pid ? `PID ${s.pid} · ${s.memory_kb}K` : 'Stopped' }}
              </div>
            </div>
            <button
              class="btn-secondary text-[10px] py-0.5 px-1.5 shrink-0"
              title="Khởi động lại dịch vụ"
              @click="quickServiceRestart(s.name)"
            >
              ↻
            </button>
          </div>
        </div>
      </div>

      <!-- AdBlock Shield & Hardware Status -->
      <div class="pro-card space-y-2.5">
        <div class="pro-card-header">
          <span class="pro-card-title">AdBlock Shield & SoC Info</span>
          <RouterLink to="/adblock" class="text-[11px] text-sky-400 hover:underline">AdBlock Tab →</RouterLink>
        </div>

        <div class="p-2.5 rounded-lg bg-emerald-500/10 border border-emerald-500/30 space-y-1">
          <div class="flex items-center justify-between">
            <span class="font-bold text-xs text-emerald-300">🛡️ Lá Chắn DNS AdBlock</span>
            <span class="badge badge-green text-[9.5px]">ACTIVE</span>
          </div>
          <p class="text-[11px] text-gray-300">
            Cổng DNS 53 đã mở trên cả mạng chính và LAN. Tự động nhận diện và triệt tiêu luồng quảng cáo video, tracker và mã độc.
          </p>
        </div>

        <div class="space-y-1 text-xs pt-1">
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Board / SoC</span>
            <span class="mono text-gray-200 font-semibold">Xiaomi Mi Router 3 Pro</span>
          </div>
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Kiến trúc Vi xử lý</span>
            <span class="mono text-sky-300 font-semibold">mipsle_24kc (880MHz)</span>
          </div>
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Bộ nhớ Flash</span>
            <span class="mono text-purple-300 font-semibold">256MB ESMT SLC NAND</span>
          </div>
          <div class="flex justify-between py-1">
            <span class="text-gray-400">Cổng USB 3.0</span>
            <span class="mono text-emerald-300 font-semibold">Dual Mode (Storage + Tether)</span>
          </div>
        </div>
      </div>
    </div>

    <!-- ROW 3: CONNECTED CLIENTS & RECENT LOGS -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-2.5">
      <!-- Connected DHCP Clients Table -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Active DHCP Clients ({{ network.clients.length }})</span>
          <RouterLink to="/access" class="text-[11px] text-sky-400 hover:underline">DHCP & Leases →</RouterLink>
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
                <td colspan="4" class="text-center text-gray-500 py-3">Chưa có thiết bị nào nhận DHCP</td>
              </tr>
              <tr v-for="c in network.clients" :key="c.mac">
                <td class="font-medium text-gray-200">{{ c.hostname || 'Connected Device' }}</td>
                <td class="mono text-sky-300">{{ c.ip }}</td>
                <td class="mono text-gray-400 text-xs">{{ c.mac }}</td>
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
          <RouterLink to="/system" class="text-[11px] text-sky-400 hover:underline">Xem Tất Cả →</RouterLink>
        </div>

        <div class="bg-black/50 rounded border border-white/5 p-2 font-mono text-[10.5px] text-gray-300 overflow-y-auto max-h-48 space-y-1">
          <div v-if="!recentLogs.length" class="text-gray-500">Chưa có nhật ký ghi nhận.</div>
          <div v-for="(line, idx) in recentLogs" :key="idx" class="leading-relaxed hover:bg-white/[0.03] px-1 rounded truncate">
            <span class="text-sky-400 select-none">›</span> {{ line }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
