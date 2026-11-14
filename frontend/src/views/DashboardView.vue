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
    actionNotice.value = res?.message || 'Đã đưa ổ cứng NVMe USB 3.0 vào chế độ nghỉ (Standby/Sleep) thành công!'
    await refreshDashboard(true)
    setTimeout(() => { actionNotice.value = null }, 4000)
  } catch (err: any) {
    actionNotice.value = 'Lỗi chuyển chế độ nghỉ: ' + (err?.message || err)
    setTimeout(() => { actionNotice.value = null }, 4000)
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
    actionNotice.value = res?.message || `Đã đặt thời gian tự động nghỉ là ${val} phút!`
    setTimeout(() => { actionNotice.value = null }, 3500)
  } catch (err: any) {
    actionNotice.value = 'Lỗi lưu cấu hình: ' + (err?.message || err)
    setTimeout(() => { actionNotice.value = null }, 4000)
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
  if (!confirm('Bạn có chắc chắn muốn xóa toàn bộ bộ đệm nhật ký hệ thống (logread & kernel dmesg)?')) return
  clearingLogs.value = true
  try {
    await apiRequest('/logs', { method: 'DELETE' })
    recentLogs.value = []
    actionNotice.value = 'Đã xóa bộ đệm nhật ký hệ thống!'
    setTimeout(() => { actionNotice.value = null }, 3000)
  } catch (err: any) {
    actionNotice.value = 'Lỗi xóa nhật ký: ' + (err?.message || err)
    setTimeout(() => { actionNotice.value = null }, 4000)
  } finally {
    clearingLogs.value = false
  }
}
const loading = ref(false)
const actionNotice = ref<string | null>(null)

// Realtime Graph Histories (20 sample points)
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

const netChartMax = computed(() => Math.max(100, ...rxHistory.value, ...txHistory.value) * 1.15)

function pushTelemetryPoint() {
  const currentCpu = Math.round(system.cpu || 0)
  cpuHistory.value.push(currentCpu)
  if (cpuHistory.value.length > 24) cpuHistory.value.shift()

  const currentRxKb = Math.round(totalRxRate.value / 1024)
  rxHistory.value.push(currentRxKb)
  if (rxHistory.value.length > 24) rxHistory.value.shift()

  const currentTxKb = Math.round(totalTxRate.value / 1024)
  txHistory.value.push(currentTxKb)
  if (txHistory.value.length > 24) txHistory.value.shift()
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

// Generate SVG Polyline points
function generatePoints(data: number[], maxVal = 100, width = 300, height = 70): string {
  if (!data.length) return ''
  if (data.length === 1) {
    const clamped = Math.max(0, Math.min(maxVal, data[0] ?? 0))
    const y = (height - (clamped / maxVal) * (height - 6) - 3).toFixed(1)
    return `0,${y} ${width},${y}`
  }
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
        if (r?.lines) recentLogs.value = r.lines.slice(-12).reverse()
      }),
      apiRequest<{ stats: AdblockStats; enabled?: boolean }>('/adblock/streams').then((r) => {
        if (r?.stats) adStats.value = r.stats
        if (typeof r?.enabled === 'boolean') adblockEnabled.value = r.enabled
        else if ((r?.stats?.analyzed || 0) > 0 || (r?.stats?.blocked || 0) > 0) adblockEnabled.value = true
      }),
    ]
    if (!silent || !system.connected) {
      tasks.unshift(
        system.refresh(),
        network.refreshInterfaces(),
        network.refreshClients(),
      )
    }
    await Promise.all(tasks)
    if (cpuHistory.value.length === 0) {
      pushTelemetryPoint()
    }
  } catch (err) {
    console.warn('Dashboard refresh error:', err)
  } finally {
    if (!silent) loading.value = false
  }
}

async function flushCache() {
  try {
    await apiRequest('/system/drop-caches', { method: 'POST' })
    actionNotice.value = 'Đã giải phóng bộ nhớ RAM cache thành công!'
    await refreshDashboard(true)
    setTimeout(() => { actionNotice.value = null }, 3500)
  } catch (err: any) {
    actionNotice.value = `Thất bại: ${err?.message || err}`
    setTimeout(() => { actionNotice.value = null }, 4000)
  }
}

async function quickServiceRestart(name: string) {
  try {
    actionNotice.value = `Đang khởi động lại dịch vụ ${name}…`
    await apiRequest('/services/control', {
      method: 'POST',
      body: JSON.stringify({ service: name, name, action: 'restart' }),
    })
    actionNotice.value = `Dịch vụ ${name} đã được khởi động lại!`
    await refreshDashboard(true)
    setTimeout(() => { actionNotice.value = null }, 3500)
  } catch (err: any) {
    actionNotice.value = `Lỗi: ${err?.message || err}`
    setTimeout(() => { actionNotice.value = null }, 4000)
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
  }, 20_000)
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="h-full overflow-y-auto p-2 sm:p-3 space-y-3 bg-[#080b11]">
    <!-- Floating Non-Shifting Toast -->
    <div
      v-if="actionNotice"
      class="r3p-toast"
      :class="actionNotice.includes('Lỗi') || actionNotice.includes('Thất bại') ? 'r3p-toast-error' : 'r3p-toast-success'"
    >
      <span class="r3p-toast-text">{{ actionNotice }}</span>
      <button type="button" class="text-xs opacity-80 hover:opacity-100 px-1 cursor-pointer" @click="actionNotice = null">✕</button>
    </div>

    <!-- Action bar & Host Info Banner -->
    <div class="flex flex-wrap items-center justify-between gap-2 bg-[#0d1322] border border-white/10 rounded-lg px-3.5 py-2">
      <div class="flex items-center gap-2.5 text-[13px] min-w-0">
        <span class="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse shrink-0"></span>
        <span class="font-bold text-gray-100 truncate">{{ system.hostname || 'R3P-Router' }}</span>
        <span class="text-gray-500 shrink-0">·</span>
        <span class="mono text-sky-400 font-semibold truncate">{{ system.hardware || 'MT7621' }}</span>
        <span class="text-gray-500 hidden sm:inline shrink-0">·</span>
        <span class="text-purple-300 mono font-semibold hidden sm:inline shrink-0">Linux {{ system.kernel || '—' }}</span>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        <button type="button" class="btn-secondary text-[12px] py-1 px-3" @click="flushCache">
          {{ t('dash_flush_cache') }}
        </button>
        <button type="button" class="btn-primary text-[12px] py-1 px-3.5 min-w-[116px]" :disabled="loading" @click="refreshDashboard(false)">
          {{ loading ? t('btn_syncing') : t('btn_sync') }}
        </button>
      </div>
    </div>

    <!-- ROW 1: REALTIME INTERACTIVE GRAPHS & GAUGES -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-2.5">
      <!-- 1. CPU Load & Sparkline Realtime Chart -->
      <div class="pro-card p-3.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-[13.5px] mb-1">
          <div class="flex items-center gap-1.5">
            <span class="text-sky-400 font-bold">{{ t('dash_cpu_load') }}</span>
            <span class="badge badge-blue text-[10.5px] mono font-bold">
              🌡️ {{ cpuTempItem.temp_celsius > 0 ? `${cpuTempItem.temp_celsius.toFixed(1)}°C` : '—' }}
            </span>
          </div>
          <span class="mono font-extrabold text-sky-300 text-[16px]">{{ (system.cpu || 0).toFixed(1) }}%</span>
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

        <div class="flex items-center justify-between text-[12px] text-gray-300 mono pt-1 border-t border-white/5 font-medium">
          <span>Load: {{ system.loadAvg[0] !== undefined ? system.loadAvg[0].toFixed(2) : '0.00' }}</span>
          <span class="text-emerald-400 font-bold">CPU: {{ cpuTempItem.temp_celsius > 0 ? `${cpuTempItem.temp_celsius.toFixed(1)}°C` : '—' }}</span>
        </div>
      </div>

      <!-- 2. Bandwidth Traffic In/Out Dual-Area Chart -->
      <div class="pro-card p-3.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-[13.5px] mb-1">
          <span class="font-bold text-emerald-400">{{ t('dash_bandwidth') }}</span>
          <span class="badge badge-blue text-[10.5px]">REALTIME</span>
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
            <path :d="generateAreaPath(rxHistory, netChartMax, 300, 70)" fill="url(#rxGrad)" />
            <polyline
              :points="generatePoints(rxHistory, netChartMax, 300, 70)"
              fill="none"
              stroke="#34d399"
              stroke-width="2"
              stroke-linecap="round"
            />
            <polyline
              :points="generatePoints(txHistory, netChartMax, 300, 70)"
              fill="none"
              stroke="#38bdf8"
              stroke-width="1.5"
              stroke-dasharray="3,3"
            />
          </svg>
        </div>

        <div class="flex items-center justify-between text-[12px] mono pt-1 border-t border-white/5">
          <span class="text-emerald-400 font-bold">↓ {{ formatRate(totalRxRate) }}</span>
          <span class="text-sky-400 font-bold">↑ {{ formatRate(totalTxRate) }}</span>
        </div>
      </div>

      <!-- 3. Memory RAM Ring Donut Gauge -->
      <div class="pro-card p-3.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-[13.5px] mb-1">
          <span class="font-bold text-amber-300">{{ t('dash_ram') }} ({{ (system.ramPhysical || system.ramTotal) ? `${system.ramPhysical || system.ramTotal} MB` : '—' }})</span>
          <span class="mono font-bold text-amber-400 text-[14px]">{{ (system.ram || 0).toFixed(0) }}%</span>
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
                :stroke-dasharray="`${Math.min(100, system.ram || 0)}, 100`"
                stroke-linecap="round"
                stroke="currentColor"
                fill="none"
                d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
              />
            </svg>
            <span class="absolute text-[12px] font-extrabold text-amber-300 mono">{{ (system.ram || 0).toFixed(0) }}%</span>
          </div>

          <div class="flex-1 space-y-1 text-[12px]">
            <div class="flex justify-between text-gray-300">
              <span class="font-medium">Đang dùng:</span>
              <span class="mono text-gray-100 font-bold">{{ (system.ramUsed || 0).toFixed(0) }} MB</span>
            </div>
            <div class="flex justify-between text-gray-300">
              <span class="font-medium">Khả dụng:</span>
              <span class="mono text-emerald-400 font-bold">{{ system.ramTotal ? `${Math.max(0, system.ramTotal - (system.ramUsed || 0)).toFixed(0)} MB` : '—' }}</span>
            </div>
            <div class="flex justify-between text-gray-400 text-[11.5px]">
              <span>Vật lý / HĐH:</span>
              <span class="mono font-semibold" :title="system.ramReserved > 0 ? `Kernel & DMA giữ lại ${system.ramReserved} MB` : ''">
                {{ system.ramTotal ? (system.ramReserved > 0 ? `${system.ramPhysical} MB (${system.ramTotal} MB OS)` : `${system.ramTotal} MB`) : '—' }}
              </span>
            </div>
          </div>
        </div>

        <div class="progress-bar-bg mt-1">
          <div class="progress-bar-fill bg-amber-400" :style="{ width: `${Math.min(100, system.ram || 0)}%` }"></div>
        </div>
      </div>

      <!-- 4. Storage & Partitions Gauge -->
      <div class="pro-card p-3.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-[13.5px] mb-1">
          <span class="font-bold text-purple-300">{{ t('dash_storage') }}</span>
          <RouterLink to="/nas" class="text-[12px] text-sky-400 hover:underline font-semibold">→</RouterLink>
        </div>

        <div class="grid grid-cols-2 gap-2.5 my-1">
          <!-- Graph 1: NAND /overlay -->
          <div class="flex items-center gap-2.5 p-2 rounded-lg bg-black/30 border border-white/10">
            <div class="relative w-12 h-12 shrink-0 flex items-center justify-center">
              <svg class="w-full h-full -rotate-90" viewBox="0 0 36 36">
                <path
                  class="text-white/10"
                  stroke-width="3.5"
                  stroke="currentColor"
                  fill="none"
                  d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                />
                <path
                  class="text-purple-400 transition-all duration-500 ease-out"
                  stroke-width="3.5"
                  :stroke-dasharray="`${overlayPercent}, 100`"
                  stroke-linecap="round"
                  stroke="currentColor"
                  fill="none"
                  d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                />
              </svg>
              <span class="absolute text-[12px] font-extrabold text-purple-300 mono">{{ overlayPercent }}%</span>
            </div>
            <div class="min-w-0 flex-1 space-y-0.5">
              <div class="font-bold text-[13px] text-gray-100 truncate">NAND (/overlay)</div>
              <div class="text-purple-300 font-bold text-[12px] truncate">{{ overlayVol ? `${formatMB(overlayVol.avail_kb)} trống` : '—' }}</div>
              <div class="text-gray-300 text-[11.5px] font-medium truncate">Dùng: {{ overlayVol ? formatMB(overlayVol.used_kb) : '—' }}</div>
            </div>
          </div>

          <!-- Graph 2: USB 3.0 HDD /mnt/sda1 -->
          <div class="flex items-center gap-2.5 p-2 rounded-lg bg-black/30 border border-white/10">
            <div class="relative w-12 h-12 shrink-0 flex items-center justify-center">
              <svg class="w-full h-full -rotate-90" viewBox="0 0 36 36">
                <path
                  class="text-white/10"
                  stroke-width="3.5"
                  stroke="currentColor"
                  fill="none"
                  d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                />
                <path
                  class="text-emerald-400 transition-all duration-500 ease-out"
                  stroke-width="3.5"
                  :stroke-dasharray="`${usbHddPercent}, 100`"
                  stroke-linecap="round"
                  stroke="currentColor"
                  fill="none"
                  d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                />
              </svg>
              <span class="absolute text-[12px] font-extrabold text-emerald-300 mono">{{ usbHddPercent }}%</span>
            </div>
            <div class="min-w-0 flex-1 space-y-0.5">
              <div class="font-bold text-[13px] text-gray-100 flex items-center justify-between">
                <span class="truncate">USB Storage</span>
                <span
                  class="text-[10.5px] mono font-bold px-1.5 py-0.5 rounded"
                  :class="diskTempItem?.disk_state === 'standby' ? 'bg-purple-900/40 text-purple-300 border border-purple-500/30' : (diskTempItem?.temp_celsius && diskTempItem.temp_celsius >= 65 ? 'bg-rose-900/40 text-rose-300' : 'bg-emerald-900/40 text-emerald-300 border border-emerald-500/30')"
                >
                  {{ diskTempItem?.disk_state === 'standby' ? '💤 Sleep' : (diskTempItem?.temp_celsius ? `${diskTempItem.temp_celsius.toFixed(0)}°C` : '—') }}
                </span>
              </div>
              <div class="text-emerald-300 font-bold text-[12px] truncate">{{ usbHddVol ? `${formatMB(usbHddVol.avail_kb)} trống` : 'Chưa gắn USB' }}</div>
              <div class="text-gray-300 text-[11.5px] font-medium truncate">Dùng: {{ usbHddVol ? formatMB(usbHddVol.used_kb) : '—' }}</div>
            </div>
          </div>
        </div>

        <div class="flex items-center justify-between text-[11.5px] text-gray-300 pt-1.5 border-t border-white/10 mono font-semibold">
          <div class="flex items-center gap-1.5">
            <button
              type="button"
              class="text-sky-400 hover:text-white transition-colors cursor-pointer text-[11px] font-bold"
              :disabled="settingStandby"
              @click="triggerStandbyNow"
              title="Ngắt truy xuất cho SSD NVMe nghỉ ngay để hạ nhiệt"
            >
              💤 {{ settingStandby ? '…' : t('dash_standby_now') }}
            </button>
          </div>
          <div class="flex items-center gap-1 text-[10.5px]">
            <span class="text-gray-400">{{ t('dash_auto_idle') }}</span>
            <select
              :value="idleTimeoutMins"
              class="bg-black/60 text-emerald-400 font-bold rounded px-1 py-0.5 border border-white/10 text-[10.5px] cursor-pointer"
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

    <!-- ROW 2: LIVE RADIOS, CORE SERVICES & PARTITION HEALTH -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-2.5">
      <!-- Live Interfaces & Radios -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Live Interfaces & Radios</span>
          <RouterLink to="/wireless" class="text-[12px] text-sky-400 hover:underline font-semibold">Wi-Fi + WWAN →</RouterLink>
        </div>

        <div class="space-y-2.5 text-[13px]">
          <div
            v-for="iface in network.interfaces.filter(i => i.name !== 'lo').slice(0, 4)"
            :key="iface.name"
            class="flex items-center justify-between p-2.5 rounded-lg bg-black/30 border border-white/10"
          >
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <span class="font-bold text-[15px] text-gray-100 mono">{{ iface.name }}</span>
                <span class="text-[12.5px] text-gray-300 truncate font-medium">
                  {{ iface.ipv4 ? `(${iface.ipv4})` : (iface.type === 'lan' ? '(LAN Bridge)' : '') }}
                </span>
              </div>
              <div class="mono text-[12px] text-gray-300 font-medium mt-0.5">
                RX: {{ formatMB(Math.round(iface.rxBytes / 1024)) }} · TX: {{ formatMB(Math.round(iface.txBytes / 1024)) }}
              </div>
            </div>
            <span class="badge text-[11.5px] font-bold px-2.5 py-1" :class="iface.up ? 'badge-green' : 'badge-gray'">
              {{ iface.up ? 'UP' : 'DOWN' }}
            </span>
          </div>

          <div
            v-for="r in radios"
            :key="r.device"
            class="flex items-center justify-between p-2.5 rounded-lg bg-black/30 border border-white/10"
          >
            <div class="min-w-0 flex-1 pr-2">
              <div class="font-bold text-[15px] text-gray-100 truncate">{{ r.band }} ({{ r.ssid || 'OpenWrt' }})</div>
              <div class="mono text-[12px] text-gray-300 font-medium mt-0.5 truncate">
                Kênh {{ r.channel || 'Auto' }} · {{ r.htmode || 'VHT80' }}
              </div>
            </div>
            <span class="badge text-[11.5px] font-bold px-2.5 py-1" :class="r.enabled ? 'badge-green' : 'badge-orange'">
              {{ r.enabled ? 'ACTIVE' : 'DISABLED' }}
            </span>
          </div>
        </div>
      </div>

      <!-- Core Procd Services -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">{{ t('dash_services') }}</span>
          <span class="text-[12px] text-gray-300 font-semibold">{{ services.length }} Monitored</span>
        </div>

        <div class="grid grid-cols-2 gap-2 text-[13px]">
          <div
            v-for="s in services"
            :key="s.name"
            class="p-2.5 rounded-lg bg-black/30 border border-white/10 flex items-center justify-between"
          >
            <div class="min-w-0 pr-1">
              <div class="flex items-center gap-1.5 font-bold text-[14px] text-gray-100">
                <span class="w-2.5 h-2.5 rounded-full shrink-0" :class="s.running ? 'bg-emerald-400 shadow-sm' : 'bg-gray-500'"></span>
                <span class="mono truncate">{{ s.name }}</span>
              </div>
              <div class="mono text-[12px] text-gray-300 font-medium truncate mt-0.5">
                {{ s.pid ? `PID ${s.pid} · ${s.memory_kb}K` : 'Stopped' }}
              </div>
            </div>
            <button
              v-if="s.name !== 'r3p-api'"
              type="button"
              class="btn-secondary text-[12px] py-1 px-2.5 shrink-0 font-bold hover:text-white"
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
          <RouterLink to="/adblock" class="text-[12px] text-sky-400 hover:underline font-semibold">AdBlock →</RouterLink>
        </div>

        <div class="p-2.5 rounded-lg bg-emerald-500/10 border border-emerald-500/30 space-y-2">
          <div class="flex items-center justify-between">
            <span class="font-bold text-[14px] text-emerald-300">{{ t('dash_adblock_shield') }}</span>
            <span class="badge text-[11px] font-bold" :class="adblockEnabled ? 'badge-green' : 'badge-gray'">
              {{ adblockEnabled ? t('adblock_active') : t('adblock_disabled') }}
            </span>
          </div>

          <!-- 4 Specific Stat Metrics with Counters -->
          <div class="grid grid-cols-2 gap-2 pt-0.5">
            <div class="p-2.5 rounded-lg bg-black/40 border border-white/5 flex flex-col justify-between">
              <span class="text-[12px] text-gray-200 font-semibold leading-snug">Tổng Số Luồng Đã Phân Tích</span>
              <span class="text-[18px] font-extrabold text-sky-400 mono mt-1">{{ adStats.analyzed.toLocaleString() }}</span>
            </div>
            <div class="p-2.5 rounded-lg bg-black/40 border border-white/5 flex flex-col justify-between">
              <span class="text-[12px] text-gray-200 font-semibold leading-snug">Trình Theo Dõi & Pixel (Trackers)</span>
              <span class="text-[18px] font-extrabold text-amber-400 mono mt-1">{{ adStats.trackers.toLocaleString() }}</span>
            </div>
            <div class="p-2.5 rounded-lg bg-black/40 border border-white/5 flex flex-col justify-between">
              <span class="text-[12px] text-gray-200 font-semibold leading-snug">Luồng Video Ads (YouTube/TikTok)</span>
              <span class="text-[18px] font-extrabold text-rose-400 mono mt-1">{{ adStats.video_ads.toLocaleString() }}</span>
            </div>
            <div class="p-2.5 rounded-lg bg-black/40 border border-white/5 flex flex-col justify-between">
              <span class="text-[12px] text-gray-200 font-semibold leading-snug">Đã Nhận Diện & Tự Động Chặn</span>
              <span class="text-[18px] font-extrabold text-emerald-400 mono mt-1">{{ adStats.blocked.toLocaleString() }}</span>
            </div>
          </div>
        </div>

        <div class="space-y-1.5 text-[13px] pt-1">
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-300 font-medium">Board / SoC</span>
            <span class="mono text-gray-100 font-semibold">Xiaomi Mi Router 3 Pro</span>
          </div>
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-300 font-medium">Kiến trúc Vi xử lý</span>
            <span class="mono text-sky-300 font-semibold">mipsle_24kc (880MHz)</span>
          </div>
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-300 font-medium">Bộ nhớ Flash</span>
            <span class="mono text-purple-300 font-semibold">256MB ESMT SLC NAND</span>
          </div>
          <div class="flex justify-between py-1">
            <span class="text-gray-300 font-medium">Cổng USB 3.0</span>
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
          <span class="pro-card-title">{{ t('dash_active_clients') }} ({{ network.clients.length }})</span>
          <RouterLink to="/access" class="text-[12px] text-sky-400 hover:underline font-semibold">DHCP &amp; Leases →</RouterLink>
        </div>

        <div class="overflow-x-auto h-52">
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
                <td colspan="4" class="text-center text-gray-400 py-4 font-medium">Chưa có thiết bị nào nhận DHCP</td>
              </tr>
              <tr v-for="c in network.clients" :key="c.mac">
                <td class="font-semibold text-gray-100 text-[13.5px]">{{ c.hostname || 'Connected Device' }}</td>
                <td class="mono font-bold text-sky-300 text-[13.5px]">{{ c.ip }}</td>
                <td class="mono text-gray-300 text-[12.5px]">{{ c.mac }}</td>
                <td><span class="badge badge-green text-[11px] font-bold">ACTIVE</span></td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Live Kernel & Daemon Log Feed -->
      <div class="pro-card flex flex-col">
        <div class="pro-card-header">
          <div class="flex items-center gap-2 min-w-0">
            <span class="pro-card-title">Live System Log (logread)</span>
            <span class="text-[12px] text-gray-300 font-medium hidden sm:inline truncate">(Mới nhất ở trên cùng)</span>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            <button
              type="button"
              class="px-3 py-1 min-w-[92px] text-[12px] rounded-lg bg-red-500/20 hover:bg-red-500/30 text-red-200 border border-red-500/40 transition font-bold cursor-pointer"
              :disabled="clearingLogs"
              @click="clearLogs"
            >
              {{ clearingLogs ? 'Đang xóa…' : '🗑 Xóa Log' }}
            </button>
            <RouterLink to="/system" class="text-[12px] text-sky-400 hover:underline font-semibold shrink-0">Xem Tất Cả →</RouterLink>
          </div>
        </div>

        <div class="bg-black/50 rounded-lg border border-white/5 p-2.5 font-mono text-[12px] text-gray-200 overflow-y-auto h-52 space-y-1">
          <div v-if="!recentLogs.length" class="text-gray-400 py-3 text-center">Bộ đệm log trống sạch sẽ.</div>
          <div v-for="(line, idx) in recentLogs" :key="idx" class="leading-relaxed hover:bg-white/[0.04] px-1 rounded truncate">
            <span class="text-sky-400 select-none font-bold">›</span> {{ line }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
