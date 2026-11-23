<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useSystemStore } from '@/stores/system'
import { useNetworkStore } from '@/stores/network'
import { apiRequest } from '@/services/api'
import { useUiPreferences } from '@/composables/useUiPreferences'

const system = useSystemStore()
const network = useNetworkStore()
const { t } = useUiPreferences()

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
const clearingLogs = ref(false)

interface AdblockStats {
  analyzed: number
  trackers: number
  video_ads: number
  blocked: number
}

const adStats = ref<AdblockStats>({
  analyzed: 0,
  trackers: 0,
  video_ads: 0,
  blocked: 0,
})
const adblockEnabled = ref(false)

const idleTimeoutMins = ref(10)
const settingStandby = ref(false)

const cpuTempItem = computed(() => {
  return system.temperatures.find(t => t.zone === 'soc') || {
    zone: 'soc',
    type: 'CPU / Bo Mạch (MT7621)',
    temp_celsius: system.temp || 0,
    status: 'Bình Thường',
  }
})

const diskTempItem = computed(() => {
  return system.temperatures.find(t => t.zone === 'usb_disk') || null
})

async function triggerStandbyNow() {
  settingStandby.value = true
  try {
    const res = await apiRequest<{ message: string }>('/usb/standby', {
      method: 'POST',
      body: JSON.stringify({ action: 'standby_now' }),
    })
    actionNotice.value = res?.message || 'Đã kích hoạt chế độ nghỉ cho ổ cứng USB!'
    await refreshDashboard(true)
    setTimeout(() => { actionNotice.value = null }, 3000)
  } catch (err: any) {
    actionNotice.value = 'Lỗi chuyển chế độ nghỉ: ' + (err?.message || err)
    setTimeout(() => { actionNotice.value = null }, 3500)
  } finally {
    settingStandby.value = false
  }
}

async function updateIdleTimeout(val: number) {
  settingStandby.value = true
  idleTimeoutMins.value = val
  try {
    const res = await apiRequest<{ message: string }>('/usb/standby', {
      method: 'POST',
      body: JSON.stringify({ action: 'set_timeout', minutes: val }),
    })
    actionNotice.value = res?.message || `Thời gian nghỉ tự động: ${val} phút`
    setTimeout(() => { actionNotice.value = null }, 3000)
  } catch (err: any) {
    actionNotice.value = 'Lỗi lưu: ' + (err?.message || err)
    setTimeout(() => { actionNotice.value = null }, 3500)
  } finally {
    settingStandby.value = false
  }
}

async function fetchIdleTimeout() {
  try {
    const res = await apiRequest<{ enabled: boolean; minutes: number }>('/usb/standby')
    if (res) {
      idleTimeoutMins.value = res.enabled ? (res.minutes || 10) : 0
    }
  } catch {
    // ignore
  }
}

const clearLogs = async () => {
  if (!confirm('Xóa sạch bộ đệm nhật ký hệ thống (logread & dmesg)?')) return
  clearingLogs.value = true
  try {
    await apiRequest('/logs', { method: 'DELETE' })
    recentLogs.value = []
    actionNotice.value = 'Đã xóa sạch nhật ký hệ thống!'
    setTimeout(() => { actionNotice.value = null }, 2500)
  } catch (err: any) {
    actionNotice.value = 'Lỗi: ' + (err?.message || err)
    setTimeout(() => { actionNotice.value = null }, 3500)
  } finally {
    clearingLogs.value = false
  }
}

const loading = ref(false)
const actionNotice = ref<string | null>(null)

// Realtime Graph Histories
const cpuHistory = ref<number[]>([])
const rxHistory = ref<number[]>([])
const txHistory = ref<number[]>([])

let pollTimer: ReturnType<typeof setInterval> | null = null

const overlayVol = computed(() => {
  return volumes.value.find((v) => v.mount_point === '/overlay' || (v.device && v.device.includes('overlay'))) ||
         volumes.value.find((v) => v.mount_point === '/') ||
         null
})
const usbHddVol = computed(() => volumes.value.find((v) => v.mount_point === '/mnt/sda1' || v.mount_point.startsWith('/mnt/sd')))

const overlayPercent = computed(() => {
  if (!overlayVol.value) return 0
  const p = Number(overlayVol.value.percent)
  if (isNaN(p) || p <= 0) return 0
  return Math.min(100, Math.max(0, Math.round(p)))
})

const usbHddPercent = computed(() => {
  if (!usbHddVol.value) return 0
  const p = Number(usbHddVol.value.percent)
  if (isNaN(p) || p <= 0) return 0
  return Math.min(100, Math.max(0, Math.round(p)))
})

const activeWanStats = computed(() => {
  const ifaceName = system.internet?.wan_iface
  if (ifaceName) {
    const found = network.interfaces.find((i) => i.name === ifaceName)
    if (found) return found
  }
  return network.wanIface || network.interfaces.find((i) => i.name === 'br-lan')
})

const totalRxRate = computed(() => activeWanStats.value?.rxRate || network.totalRxRate || 0)
const totalTxRate = computed(() => activeWanStats.value?.txRate || network.totalTxRate || 0)

const netChartMax = computed(() => Math.max(80, ...rxHistory.value, ...txHistory.value) * 1.15)

function pushTelemetryPoint() {
  const currentCpu = Math.round(system.cpu || 0)
  cpuHistory.value.push(currentCpu)
  if (cpuHistory.value.length > 28) cpuHistory.value.shift()

  const currentRxKb = Math.round(totalRxRate.value / 1024)
  rxHistory.value.push(currentRxKb)
  if (rxHistory.value.length > 28) rxHistory.value.shift()

  const currentTxKb = Math.round(totalTxRate.value / 1024)
  txHistory.value.push(currentTxKb)
  if (txHistory.value.length > 28) txHistory.value.shift()
}

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

function generatePoints(data: number[], maxVal = 100, width = 300, height = 46): string {
  if (!data.length) return ''
  if (data.length === 1) {
    const clamped = Math.max(0, Math.min(maxVal, data[0] ?? 0))
    const y = (height - (clamped / maxVal) * (height - 4) - 2).toFixed(1)
    return `0,${y} ${width},${y}`
  }
  const step = width / (data.length - 1)
  return data
    .map((val, idx) => {
      const x = (idx * step).toFixed(1)
      const clamped = Math.max(0, Math.min(maxVal, val))
      const y = (height - (clamped / maxVal) * (height - 4) - 2).toFixed(1)
      return `${x},${y}`
    })
    .join(' ')
}

function generateAreaPath(data: number[], maxVal = 100, width = 300, height = 46): string {
  if (!data.length) return ''
  const pts = generatePoints(data, maxVal, width, height)
  return `M 0,${height} L ${pts} L ${width},${height} Z`
}

async function refreshDashboard(silent = false) {
  if (silent && document.hidden) return
  if (!silent) loading.value = true
  try {
    const tasks: Promise<unknown>[] = [
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
        if (r?.lines) recentLogs.value = r.lines.slice().reverse().slice(0, 30)
      }),
      apiRequest<{ enabled: boolean; stats: AdblockStats }>('/adblock').then((r) => {
        if (r) {
          adblockEnabled.value = Boolean(r.enabled)
          if (r.stats) adStats.value = r.stats
        }
      }),
      system.refresh(),
      network.refreshInterfaces(),
      network.refreshClients(),
    ]
    await Promise.allSettled(tasks)
    pushTelemetryPoint()
  } finally {
    if (!silent) loading.value = false
  }
}

async function flushCache() {
  try {
    actionNotice.value = 'Đang giải phóng pagecache, dentries & inodes…'
    await apiRequest('/system/drop-caches', { method: 'POST' })
    await system.refresh()
    actionNotice.value = 'Đã giải phóng bộ đệm RAM hệ thống!'
    setTimeout(() => { actionNotice.value = null }, 3000)
  } catch (err: any) {
    actionNotice.value = `Lỗi: ${err?.message || err}`
    setTimeout(() => { actionNotice.value = null }, 3500)
  }
}

async function quickServiceRestart(name: string) {
  try {
    actionNotice.value = `Đang khởi động lại dịch vụ ${name}…`
    await apiRequest('/services/control', {
      method: 'POST',
      body: JSON.stringify({ service: name, name, action: 'restart' }),
    })
    actionNotice.value = `Dịch vụ ${name} đã khởi động lại!`
    await refreshDashboard(true)
    setTimeout(() => { actionNotice.value = null }, 3000)
  } catch (err: any) {
    actionNotice.value = `Lỗi: ${err?.message || err}`
    setTimeout(() => { actionNotice.value = null }, 3500)
  }
}

function handleVisibilityChange() {
  if (!document.hidden) {
    void refreshDashboard(true)
  }
}

watch(
  () => system.lastUpdate,
  () => {
    pushTelemetryPoint()
  },
)

onMounted(() => {
  void refreshDashboard(false)
  void fetchIdleTimeout()
  document.addEventListener('visibilitychange', handleVisibilityChange)
  pollTimer = setInterval(() => {
    void refreshDashboard(true)
  }, 15_000)
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="h-full overflow-y-auto p-2 space-y-2 bg-[#06090e]">
    <!-- Toast Notice -->
    <div
      v-if="actionNotice"
      class="r3p-toast"
      :class="actionNotice.includes('Lỗi') || actionNotice.includes('Thất bại') ? 'r3p-toast-error' : 'r3p-toast-success'"
    >
      <span class="r3p-toast-text">{{ actionNotice }}</span>
      <button type="button" class="text-xs opacity-80 hover:opacity-100 px-1 cursor-pointer" @click="actionNotice = null">✕</button>
    </div>

    <!-- Compact System Telemetry Header -->
    <div class="flex flex-wrap items-center justify-between gap-1.5 bg-[#0a0f1a] border border-white/10 rounded px-2.5 py-1.5 text-xs">
      <div class="flex items-center gap-2 min-w-0">
        <span class="w-2 h-2 rounded-full bg-emerald-400 animate-pulse shrink-0"></span>
        <span class="font-bold text-gray-100 truncate">{{ system.hostname || 'R3P-Router' }}</span>
        <span class="text-gray-500">·</span>
        <span class="mono text-sky-400 font-semibold">{{ system.hardware || 'MT7621AT' }} (880MHz)</span>
        <span class="text-gray-500 hidden sm:inline">·</span>
        <span class="text-purple-300 mono hidden sm:inline">Linux {{ system.kernel || '6.12.94' }}</span>
        <span class="text-gray-500 hidden md:inline">·</span>
        <span class="mono text-gray-400 hidden md:inline">Uptime: {{ Math.floor((system.uptime || 0) / 86400) }}d {{ Math.floor(((system.uptime || 0) % 86400) / 3600) }}h</span>
      </div>
      <div class="flex items-center gap-1.5 shrink-0">
        <span class="mono text-gray-400 text-[11px] pr-1">Load: <b class="text-sky-300">{{ system.loadAvg[0] !== undefined ? system.loadAvg[0].toFixed(2) : '0.00' }}</b></span>
        <button type="button" class="btn-secondary text-[11px] py-0.5 px-2 rounded cursor-pointer" @click="flushCache">
          {{ t('dash_flush_cache') }}
        </button>
        <button type="button" class="btn-primary text-[11px] py-0.5 px-2.5 rounded min-w-[70px] cursor-pointer" :disabled="loading" @click="refreshDashboard(false)">
          {{ loading ? '…' : 'Đồng Bộ ↻' }}
        </button>
      </div>
    </div>

    <!-- ROW 1: 4 COMPACT TELEMETRY GAUGES -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-2">
      <!-- 1. CPU Load & Sparkline -->
      <div class="bg-[#0b101c] border border-white/10 rounded p-2.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-xs mb-1">
          <div class="flex items-center gap-1.5">
            <span class="text-sky-400 font-bold uppercase tracking-wider text-[11px]">CPU Load</span>
            <span class="text-[10px] mono px-1 py-0.2 rounded bg-sky-950/60 text-sky-300 border border-sky-500/20">
              {{ cpuTempItem.temp_celsius > 0 ? `${cpuTempItem.temp_celsius.toFixed(0)}°C` : '—' }}
            </span>
          </div>
          <span class="mono font-bold text-sky-300 text-sm">{{ (system.cpu || 0).toFixed(1) }}%</span>
        </div>

        <div class="h-11 w-full relative my-0.5 overflow-hidden rounded bg-black/50 border border-white/5">
          <svg class="w-full h-full" viewBox="0 0 300 46" preserveAspectRatio="none">
            <defs>
              <linearGradient id="cpuGrad2" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="#38bdf8" stop-opacity="0.35" />
                <stop offset="100%" stop-color="#38bdf8" stop-opacity="0.0" />
              </linearGradient>
            </defs>
            <path :d="generateAreaPath(cpuHistory, 100, 300, 46)" fill="url(#cpuGrad2)" />
            <polyline
              :points="generatePoints(cpuHistory, 100, 300, 46)"
              fill="none"
              stroke="#38bdf8"
              stroke-width="2"
              stroke-linecap="round"
            />
          </svg>
        </div>

        <div class="flex items-center justify-between text-[11px] text-gray-400 mono pt-1 border-t border-white/5">
          <span>Load: {{ system.loadAvg[0] !== undefined ? system.loadAvg[0].toFixed(2) : '0.00' }}</span>
          <span class="text-emerald-400">2C / 4T (880MHz)</span>
        </div>
      </div>

      <!-- 2. Bandwidth Traffic In/Out -->
      <div class="bg-[#0b101c] border border-white/10 rounded p-2.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-xs mb-1">
          <span class="font-bold text-emerald-400 uppercase tracking-wider text-[11px]">Băng Thông WAN</span>
          <span class="mono text-[11px] text-gray-300">
            <span class="text-emerald-400 font-bold">↓ {{ formatRate(totalRxRate) }}</span>
            <span class="text-gray-500 mx-1">·</span>
            <span class="text-sky-400 font-bold">↑ {{ formatRate(totalTxRate) }}</span>
          </span>
        </div>

        <div class="h-11 w-full relative my-0.5 overflow-hidden rounded bg-black/50 border border-white/5">
          <svg class="w-full h-full" viewBox="0 0 300 46" preserveAspectRatio="none">
            <defs>
              <linearGradient id="rxGrad2" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="#34d399" stop-opacity="0.35" />
                <stop offset="100%" stop-color="#34d399" stop-opacity="0.0" />
              </linearGradient>
            </defs>
            <path :d="generateAreaPath(rxHistory, netChartMax, 300, 46)" fill="url(#rxGrad2)" />
            <polyline
              :points="generatePoints(rxHistory, netChartMax, 300, 46)"
              fill="none"
              stroke="#34d399"
              stroke-width="2"
              stroke-linecap="round"
            />
            <polyline
              :points="generatePoints(txHistory, netChartMax, 300, 46)"
              fill="none"
              stroke="#38bdf8"
              stroke-width="1.5"
              stroke-dasharray="3,3"
            />
          </svg>
        </div>

        <div class="flex items-center justify-between text-[11px] mono pt-1 border-t border-white/5 text-gray-400">
          <span>Tải xuống (RX): <b class="text-emerald-300">{{ formatMB(Math.round(network.totalRx / 1024)) }}</b></span>
          <span>Tải lên (TX): <b class="text-sky-300">{{ formatMB(Math.round(network.totalTx / 1024)) }}</b></span>
        </div>
      </div>

      <!-- 3. RAM Memory Gauge -->
      <div class="bg-[#0b101c] border border-white/10 rounded p-2.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-xs mb-1">
          <span class="font-bold text-amber-300 uppercase tracking-wider text-[11px]">RAM ({{ system.ramTotal || 512 }} MB)</span>
          <span class="mono font-bold text-amber-400 text-xs">{{ (system.ram || 0).toFixed(0) }}% Dùng</span>
        </div>

        <div class="flex items-center gap-2.5 my-1">
          <!-- Compact ring gauge -->
          <div class="relative w-11 h-11 shrink-0 flex items-center justify-center">
            <svg class="w-full h-full -rotate-90" viewBox="0 0 36 36">
              <path class="text-white/10" stroke-width="4" stroke="currentColor" fill="none" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
              <path
                class="text-amber-400 transition-all duration-500 ease-out"
                stroke-width="4"
                :stroke-dasharray="`${Math.min(100, system.ram || 0)}, 100`"
                stroke-linecap="round"
                stroke="currentColor"
                fill="none"
                d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
              />
            </svg>
            <span class="absolute text-[10.5px] font-bold text-amber-300 mono">{{ (system.ram || 0).toFixed(0) }}%</span>
          </div>

          <div class="flex-1 space-y-0.5 text-[11px] mono">
            <div class="flex justify-between">
              <span class="text-gray-400">Đang dùng:</span>
              <span class="text-gray-100 font-bold">{{ (system.ramUsed || 0).toFixed(0) }} MB</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-400">Khả dụng:</span>
              <span class="text-emerald-400 font-bold">{{ system.ramTotal ? `${Math.max(0, system.ramTotal - (system.ramUsed || 0)).toFixed(0)} MB` : '—' }}</span>
            </div>
            <div class="flex justify-between text-gray-500 text-[10px]">
              <span>Phân bổ HĐH:</span>
              <span>{{ system.ramPhysical || 512 }}MB DDR3</span>
            </div>
          </div>
        </div>

        <div class="flex items-center justify-between text-[11px] mono pt-1 border-t border-white/5 text-gray-400">
          <span>Phần cứng: <b class="text-amber-200">{{ system.ramPhysical || 512 }}MB DDR3</b></span>
          <span class="text-emerald-400 font-medium">Bộ đệm tối ưu</span>
        </div>
      </div>

      <!-- 4. Storage (NAND & USB 3.0) -->
      <div class="bg-[#0b101c] border border-white/10 rounded p-2.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-xs mb-1">
          <span class="font-bold text-purple-300 uppercase tracking-wider text-[11px]">Ổ Đĩa & Lưu Trữ</span>
          <RouterLink to="/nas" class="text-[11px] text-sky-400 hover:underline mono font-semibold">Chi Tiết NAS →</RouterLink>
        </div>

        <div class="space-y-1.5 my-1">
          <!-- NAND /overlay -->
          <div class="bg-black/30 border border-white/5 rounded px-2 py-1 text-xs">
            <div class="flex items-center justify-between text-[11px] mb-0.5">
              <span class="font-bold text-gray-200">NAND (/overlay)</span>
              <span class="mono text-purple-300 text-[10.5px] font-bold">{{ overlayPercent }}% · {{ overlayVol ? `${formatMB(overlayVol.avail_kb)} trống` : '—' }}</span>
            </div>
            <div class="w-full bg-black/50 h-1.5 rounded-full overflow-hidden">
              <div class="bg-purple-400 h-full rounded-full transition-all" :style="{ width: `${overlayPercent}%` }"></div>
            </div>
          </div>

          <!-- USB 3.0 HDD -->
          <div class="bg-black/30 border border-white/5 rounded px-2 py-1 text-xs">
            <div class="flex items-center justify-between text-[11px] mb-0.5">
              <div class="flex items-center gap-1.5">
                <span class="font-bold text-gray-200">USB 3.0 (/mnt/sda1)</span>
                <span
                  class="text-[9.5px] mono font-bold px-1 py-0.2 rounded"
                  :class="diskTempItem?.disk_state === 'standby' ? 'bg-purple-900/40 text-purple-300' : 'bg-emerald-900/40 text-emerald-300'"
                >
                  {{ diskTempItem?.disk_state === 'standby' ? '💤 Sleep' : (diskTempItem?.temp_celsius ? `${diskTempItem.temp_celsius.toFixed(0)}°C` : 'ext4') }}
                </span>
              </div>
              <span class="mono text-emerald-300 text-[10.5px] font-bold">{{ usbHddPercent }}% · {{ usbHddVol ? `${formatMB(usbHddVol.avail_kb)} trống` : 'Chưa gắn' }}</span>
            </div>
            <div class="w-full bg-black/50 h-1.5 rounded-full overflow-hidden">
              <div class="bg-emerald-400 h-full rounded-full transition-all" :style="{ width: `${usbHddPercent}%` }"></div>
            </div>
          </div>
        </div>

        <div class="flex items-center justify-between text-[10.5px] text-gray-400 pt-1 border-t border-white/5 mono">
          <button
            type="button"
            class="text-sky-400 hover:text-white transition-colors cursor-pointer font-bold"
            :disabled="settingStandby"
            @click="triggerStandbyNow"
          >
            💤 {{ settingStandby ? '…' : 'Sleep Ngay' }}
          </button>
          <div class="flex items-center gap-1">
            <span>Tự nghỉ:</span>
            <select
              :value="idleTimeoutMins"
              class="bg-black/80 text-emerald-400 font-bold rounded px-1 py-0.2 border border-white/10 text-[10px] cursor-pointer"
              :disabled="settingStandby"
              @change="updateIdleTimeout(Number(($event.target as HTMLSelectElement).value))"
            >
              <option :value="5" class="bg-gray-900 text-white">5p</option>
              <option :value="10" class="bg-gray-900 text-white">10p</option>
              <option :value="15" class="bg-gray-900 text-white">15p</option>
              <option :value="30" class="bg-gray-900 text-white">30p</option>
              <option :value="0" class="bg-gray-900 text-white">Tắt</option>
            </select>
          </div>
        </div>
      </div>
    </div>

    <!-- ROW 2: LIVE PORTS & WIRELESS, PROCD SERVICES, HARDWARE/ADBLOCK -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-2">
      <!-- 1. Live Ports & Wireless Radios Matrix -->
      <div class="bg-[#0b101c] border border-white/10 rounded p-2.5 flex flex-col justify-between">
        <div class="flex items-center justify-between pb-1.5 mb-1.5 border-b border-white/10">
          <span class="font-bold text-gray-200 text-xs uppercase tracking-wider flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-sky-400"></span>
            Cổng Mạng & Sóng Wi-Fi
          </span>
          <RouterLink to="/wireless" class="text-[11px] text-sky-400 hover:underline font-semibold">Wi-Fi / WWAN →</RouterLink>
        </div>

        <div class="space-y-1 text-xs">
          <!-- Physical & Virtual Interfaces Table -->
          <div
            v-for="iface in network.interfaces.filter(i => i.name !== 'lo').slice(0, 4)"
            :key="iface.name"
            class="flex items-center justify-between px-2 py-1 rounded bg-black/30 border border-white/5 text-[11.5px]"
          >
            <div class="flex items-center gap-2 min-w-0">
              <span class="font-bold text-gray-100 mono w-12">{{ iface.name }}</span>
              <span class="mono text-gray-400 text-[10.5px] truncate">
                {{ iface.ipv4 || (iface.type === 'lan' ? 'LAN Bridge' : 'Chưa có IP') }}
              </span>
            </div>
            <div class="flex items-center gap-2 shrink-0">
              <span class="mono text-gray-400 text-[10px] hidden sm:inline">
                ↓{{ formatMB(Math.round(iface.rxBytes / 1024)) }} ↑{{ formatMB(Math.round(iface.txBytes / 1024)) }}
              </span>
              <span class="text-[9.5px] font-bold px-1.5 py-0.2 rounded" :class="iface.up ? 'bg-emerald-950 text-emerald-300 border border-emerald-500/30' : 'bg-gray-800 text-gray-400'">
                {{ iface.up ? 'UP' : 'DOWN' }}
              </span>
            </div>
          </div>

          <!-- Radios 2.4G & 5G -->
          <div
            v-for="r in radios"
            :key="r.device"
            class="flex items-center justify-between px-2 py-1 rounded bg-black/30 border border-white/5 text-[11.5px]"
          >
            <div class="flex items-center gap-2 min-w-0">
              <span class="font-bold text-sky-300 mono w-12">{{ r.band }}</span>
              <span class="text-gray-200 text-[11px] truncate">SSID: <b class="text-white">{{ r.ssid || 'OpenWrt' }}</b></span>
            </div>
            <div class="flex items-center gap-2 shrink-0">
              <span class="mono text-gray-400 text-[10px]">CH {{ r.channel || 'Auto' }} · {{ r.htmode || 'VHT80' }}</span>
              <span class="text-[9.5px] font-bold px-1.5 py-0.2 rounded" :class="r.enabled ? 'bg-emerald-950 text-emerald-300 border border-emerald-500/30' : 'bg-amber-950 text-amber-300 border border-amber-500/30'">
                {{ r.enabled ? 'ON' : 'OFF' }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- 2. Core Procd Services -->
      <div class="bg-[#0b101c] border border-white/10 rounded p-2.5 flex flex-col justify-start gap-1">
        <div class="flex items-center justify-between pb-1.5 mb-1.5 border-b border-white/10">
          <span class="font-bold text-gray-200 text-xs uppercase tracking-wider flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
            Tiến Trình Hệ Thống (Procd)
          </span>
          <span class="text-[11px] text-gray-400 font-semibold">{{ services.length }} Dịch vụ</span>
        </div>

        <div class="grid grid-cols-2 gap-1 text-xs">
          <div
            v-for="s in services"
            :key="s.name"
            class="px-2 py-1 rounded bg-black/30 border border-white/5 flex items-center justify-between text-[11px]"
          >
            <div class="min-w-0 flex items-center gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full shrink-0" :class="s.running ? 'bg-emerald-400' : 'bg-gray-500'"></span>
              <div class="truncate">
                <span class="mono font-bold text-gray-200 truncate">{{ s.name }}</span>
                <span class="mono text-[9.5px] text-gray-500 ml-1">{{ s.pid ? `PID ${s.pid}` : 'Off' }}</span>
              </div>
            </div>
            <button
              v-if="s.name !== 'r3p-api'"
              type="button"
              class="text-gray-400 hover:text-white text-[11px] px-1 py-0.2 rounded cursor-pointer transition-colors"
              title="Khởi động lại dịch vụ"
              @click="quickServiceRestart(s.name)"
            >
              ↻
            </button>
          </div>
        </div>
      </div>

      <!-- 3. Hardware Specs & AdBlock Shield -->
      <div class="bg-[#0b101c] border border-white/10 rounded p-2.5 flex flex-col justify-between">
        <div class="flex items-center justify-between pb-1.5 mb-1.5 border-b border-white/10">
          <span class="font-bold text-gray-200 text-xs uppercase tracking-wider flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-purple-400"></span>
            Phần Cứng & AdBlock Shield
          </span>
          <RouterLink to="/adblock" class="text-[11px] text-sky-400 hover:underline font-semibold">Cài Đặt →</RouterLink>
        </div>

        <!-- AdBlock Status Widget -->
        <div class="bg-black/30 border border-white/5 rounded px-2.5 py-1.5 mb-1.5 flex items-center justify-between text-xs">
          <div class="flex items-center gap-2">
            <span class="font-bold text-gray-200 text-[11.5px]">Lá Chắn DNS AdBlock</span>
            <span class="text-[9.5px] font-bold px-1.5 py-0.2 rounded" :class="adblockEnabled ? 'bg-emerald-950 text-emerald-300 border border-emerald-500/30' : 'bg-gray-800 text-gray-400'">
              {{ adblockEnabled ? 'ĐANG BẬT' : 'ĐÃ TẮT' }}
            </span>
          </div>
          <span class="mono text-[11px] text-emerald-300 font-bold">
            {{ adStats.blocked > 0 ? `${adStats.blocked.toLocaleString()} Đã chặn` : '85,000 Quy tắc' }}
          </span>
        </div>

        <!-- Hardware Specs Matrix -->
        <div class="space-y-1 text-[11.5px] mono">
          <div class="flex justify-between py-0.5 border-b border-white/5">
            <span class="text-gray-400">Bo Mạch (SoC):</span>
            <span class="text-gray-100 font-bold">Xiaomi R3P (MT7621AT)</span>
          </div>
          <div class="flex justify-between py-0.5 border-b border-white/5">
            <span class="text-gray-400">Kiến Trúc / Xung:</span>
            <span class="text-sky-300 font-bold">mipsle_24kc (880MHz 2C/4T)</span>
          </div>
          <div class="flex justify-between py-0.5 border-b border-white/5">
            <span class="text-gray-400">Bộ Nhớ Flash:</span>
            <span class="text-purple-300 font-bold">256MB ESMT SLC NAND</span>
          </div>
          <div class="flex justify-between py-0.5">
            <span class="text-gray-400">Cổng USB 3.0:</span>
            <span class="text-emerald-300 font-bold">Dual Mode (Storage + 4G Tether)</span>
          </div>
        </div>
      </div>
    </div>

    <!-- ROW 3: ACTIVE CLIENTS & LIVE SYSTEM LOG -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-2">
      <!-- Connected DHCP Clients Table -->
      <div class="bg-[#0b101c] border border-white/10 rounded p-2.5 flex flex-col justify-between">
        <div class="flex items-center justify-between pb-1.5 mb-1.5 border-b border-white/10">
          <span class="font-bold text-gray-200 text-xs uppercase tracking-wider flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
            Thiết Bị Đang Kết Nối ({{ network.clients.length }})
          </span>
          <RouterLink to="/access" class="text-[11px] text-sky-400 hover:underline font-semibold">DHCP & Leases →</RouterLink>
        </div>

        <div class="overflow-x-auto h-44">
          <table class="w-full text-left text-xs border-collapse">
            <thead>
              <tr class="border-b border-white/10 text-[10.5px] uppercase text-gray-400 font-bold tracking-wider">
                <th class="py-1 px-2">Hostname</th>
                <th class="py-1 px-2">IPv4 Address</th>
                <th class="py-1 px-2">MAC Address</th>
                <th class="py-1 px-2 text-right">Trạng Thái</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-white/5 text-[11.5px] mono">
              <tr v-if="!network.clients.length">
                <td colspan="4" class="text-center text-gray-500 py-3 font-medium">Chưa có thiết bị nào nhận DHCP</td>
              </tr>
              <tr v-for="c in network.clients" :key="c.mac" class="hover:bg-white/[0.03]">
                <td class="py-1 px-2 font-semibold text-gray-200">{{ c.hostname || 'Unknown' }}</td>
                <td class="py-1 px-2 font-bold text-sky-300">{{ c.ip }}</td>
                <td class="py-1 px-2 text-gray-400 text-[11px]">{{ c.mac }}</td>
                <td class="py-1 px-2 text-right">
                  <span class="text-[9.5px] font-bold px-1.5 py-0.2 rounded bg-emerald-950 text-emerald-300 border border-emerald-500/30">ONLINE</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Live Kernel & Daemon Log Feed -->
      <div class="bg-[#0b101c] border border-white/10 rounded p-2.5 flex flex-col justify-between">
        <div class="flex items-center justify-between pb-1.5 mb-1.5 border-b border-white/10">
          <div class="flex items-center gap-2 min-w-0">
            <span class="font-bold text-gray-200 text-xs uppercase tracking-wider flex items-center gap-1.5">
              <span class="w-2 h-2 rounded-full bg-sky-400"></span>
              Live System Log (logread)
            </span>
          </div>
          <div class="flex items-center gap-1.5 shrink-0">
            <button
              type="button"
              class="px-2 py-0.5 text-[10.5px] rounded bg-red-950/60 hover:bg-red-900/80 text-red-300 border border-red-500/30 transition font-bold cursor-pointer"
              :disabled="clearingLogs"
              @click="clearLogs"
            >
              {{ clearingLogs ? '…' : '🗑 Xóa Log' }}
            </button>
            <RouterLink to="/system" class="text-[11px] text-sky-400 hover:underline font-semibold shrink-0">Xem Tất Cả →</RouterLink>
          </div>
        </div>

        <div class="bg-black/60 rounded border border-white/5 p-2 font-mono text-[11px] text-gray-300 overflow-y-auto h-44 space-y-0.5">
          <div v-if="!recentLogs.length" class="text-gray-500 py-3 text-center">Bộ đệm log trống sạch sẽ.</div>
          <div v-for="(line, idx) in recentLogs" :key="idx" class="leading-snug hover:bg-white/[0.04] px-1 rounded truncate">
            <span class="text-sky-400 select-none font-bold">›</span> {{ line }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
