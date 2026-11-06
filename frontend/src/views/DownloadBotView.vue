<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { apiRequest } from '@/services/api'

interface EspRuntimeJob {
  id: string
  espJobId?: number
  chatId?: string
  url: string
  platform: string
  mediaType: string
  espStatus: string
  runtimeStatus: string
  progressPercent: number
  receivedAt: string
  startedAt?: string | null
  finishedAt?: string | null
  errorMessage?: string
  outputPath?: string
  outputBytes?: number
  workerId?: string
}

interface EspScheduledItem {
  id: string
  url: string
  scheduledAt: string
  recurrence: string
  status: string
  createdAt: string
  errorMessage?: string
}

interface DownloadTask {
  gid: string
  name: string
  url: string
  dir: string
  file_path: string
  status: 'active' | 'waiting' | 'paused' | 'complete' | 'error'
  totalLength: number
  completedLength: number
  downloadSpeed: number
  error_msg?: string
}

const activeTab = ref<'esp32_integrated' | 'esp32_webui' | 'direct_aria2'>('esp32_integrated')

// ESP32-Download Linux Runtime State
const espJobs = ref<EspRuntimeJob[]>([])
const espScheduled = ref<EspScheduledItem[]>([])
const espStatus = ref<any>({
  workerId: 'R3P-Router-linux-runtime',
  ytDlpInstalled: true,
  ffmpegInstalled: true,
  outputDirectoryExists: true,
  outputFreeBytes: 0,
  completedLast30DaysCount: 0,
  completedLast30DaysBytes: 0,
  telegramConfigured: true,
  telegramEnabled: true,
  deviceReachable: false,
})
const espConfig = ref<any>({
  deviceBaseUrl: 'http://192.168.1.5',
  runtimeBaseUrl: 'http://192.168.2.1:8787',
  telegramBotToken: '',
  telegramAllowedChatId: '',
  telegramEnabled: true,
  outputDirectory: '/mnt/sda1/esp32-downloads',
  ytDlpPath: '/usr/bin/yt-dlp',
  ffmpegPath: '/usr/bin/ffmpeg',
})
const pairingInfo = ref<any>(null)
const showEspConfig = ref(false)
const showQrModal = ref(false)

const mediaUrl = ref('')
const scheduleTime = ref('')
const scheduleRecurrence = ref('once')
const queueingMedia = ref(false)

// Direct / Aria2 State
const tasks = ref<DownloadTask[]>([])
const newUrl = ref('')
const adding = ref(false)
const globalDownloadSpeed = ref(0)
const downloadDir = ref('/mnt/sda1/downloads')
const maxConcurrent = ref(3)

const notice = ref<string | null>(null)
let pollTimer: ReturnType<typeof setInterval> | null = null

async function fetchEspRuntime() {
  try {
    const [stResp, qResp, cfgResp, schResp] = await Promise.all([
      fetch('/api/local/status', { credentials: 'include' }).then((r) => (r.ok ? r.json() : null)).catch(() => null),
      fetch('/api/runtime/jobs', { credentials: 'include' }).then((r) => (r.ok ? r.json() : null)).catch(() => null),
      fetch('/api/local/config', { credentials: 'include' }).then((r) => (r.ok ? r.json() : null)).catch(() => null),
      fetch('/api/local/scheduled-downloads', { credentials: 'include' }).then((r) => (r.ok ? r.json() : null)).catch(() => null),
    ])
    if (stResp) espStatus.value = stResp
    if (qResp?.jobs) espJobs.value = qResp.jobs
    if (cfgResp) espConfig.value = { ...espConfig.value, ...cfgResp }
    if (schResp?.items) espScheduled.value = schResp.items
  } catch (e) {
    console.warn('ESP32 runtime fetch error:', e)
  }
}

async function queueYtDlpJob() {
  const url = mediaUrl.value.trim()
  if (!url || queueingMedia.value) return
  queueingMedia.value = true
  notice.value = null
  try {
    if (scheduleTime.value) {
      const resp = await fetch('/api/local/scheduled-downloads', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          url,
          scheduledAt: new Date(scheduleTime.value).toISOString(),
          recurrence: scheduleRecurrence.value,
        }),
      })
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
      notice.value = 'Đã thêm lịch hẹn giờ tải video vào Linux Runtime!'
      scheduleTime.value = ''
    } else {
      const resp = await fetch('/api/runtime/jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ url, force: true }),
      })
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
      notice.value = 'Đã đưa link vào tiến trình yt-dlp trên Router! Đang tải về /mnt/sda1/esp32-downloads…'
    }
    mediaUrl.value = ''
    await fetchEspRuntime()
  } catch (err: any) {
    notice.value = 'Lỗi khi thêm link: ' + (err?.message || err)
  } finally {
    queueingMedia.value = false
  }
}

async function espJobAction(id: string, action: 'run' | 'retry' | 'cancel' | 'delete') {
  try {
    if (action === 'delete') {
      await fetch(`/api/runtime/jobs/${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
    } else {
      await fetch(`/api/runtime/jobs/${encodeURIComponent(id)}/${action}`, { method: 'POST', credentials: 'include' })
    }
    await fetchEspRuntime()
  } catch (err: any) {
    notice.value = 'Lỗi thao tác: ' + (err?.message || err)
  }
}

async function clearEspHistory() {
  try {
    await fetch('/api/runtime/jobs/clear', { method: 'POST', credentials: 'include' })
    notice.value = 'Đã xóa lịch sử các tác vụ đã hoàn tất!'
    await fetchEspRuntime()
  } catch (err: any) {
    notice.value = 'Lỗi: ' + (err?.message || err)
  }
}

async function deleteScheduled(id: string) {
  await fetch(`/api/local/scheduled-downloads/${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  await fetchEspRuntime()
}

async function saveEspConfig() {
  try {
    const resp = await fetch('/api/local/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(espConfig.value),
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    notice.value = 'Đã lưu cấu hình ESP32-Download Linux Host!'
    showEspConfig.value = false
    await fetchEspRuntime()
  } catch (err: any) {
    notice.value = 'Lỗi lưu cấu hình: ' + (err?.message || err)
  }
}

async function openQrPairing() {
  showQrModal.value = true
  try {
    const resp = await fetch('/api/local/pairing-info', { credentials: 'include' })
    if (resp.ok) {
      pairingInfo.value = await resp.json()
    }
  } catch {
    // ignore
  }
}

// Direct / Aria2 Downloader Functions
async function fetchDownloads() {
  try {
    const res = await apiRequest<{
      tasks: DownloadTask[]
      downloadSpeed: number
      settings: { default_dir: string; max_concurrent: number }
    }>('/downloads')
    if (res) {
      tasks.value = res.tasks || []
      globalDownloadSpeed.value = res.downloadSpeed || 0
      if (res.settings?.default_dir) {
        downloadDir.value = res.settings.default_dir
        maxConcurrent.value = res.settings.max_concurrent || 3
      }
    }
  } catch (err) {
    console.warn('Download fetch error:', err)
  }
}

async function addUrl() {
  if (!newUrl.value.trim() || adding.value) return
  adding.value = true
  notice.value = null
  try {
    await apiRequest('/downloads', {
      method: 'POST',
      body: JSON.stringify({ url: newUrl.value.trim(), dir: downloadDir.value }),
    })
    newUrl.value = ''
    notice.value = 'Download task queued and started!'
    await fetchDownloads()
  } catch (err: any) {
    notice.value = 'Error: ' + (err?.message || err)
  } finally {
    adding.value = false
  }
}

async function taskAction(gid: string, action: 'pause' | 'resume' | 'retry' | 'remove') {
  try {
    await apiRequest('/downloads/action', {
      method: 'POST',
      body: JSON.stringify({ gid, action }),
    })
    await fetchDownloads()
  } catch (err: any) {
    notice.value = 'Action failed: ' + (err?.message || err)
  }
}

async function saveSettings() {
  try {
    await apiRequest('/downloads', {
      method: 'PUT',
      body: JSON.stringify({
        default_dir: downloadDir.value,
        max_concurrent: maxConcurrent.value,
        speed_limit_kbs: 0,
      }),
    })
    notice.value = 'Download settings saved!'
  } catch (err: any) {
    notice.value = 'Error: ' + (err?.message || err)
  }
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

onMounted(() => {
  void fetchEspRuntime()
  void fetchDownloads()
  pollTimer = setInterval(() => {
    void fetchEspRuntime()
    if (activeTab.value === 'direct_aria2') {
      void fetchDownloads()
    }
  }, 2500)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Top Mode Switcher Bar -->
    <div class="pro-card p-2.5 flex flex-wrap items-center justify-between gap-2">
      <div class="flex flex-wrap items-center gap-1.5">
        <button
          class="px-3 py-1.5 rounded text-xs font-semibold transition cursor-pointer"
          :class="activeTab === 'esp32_integrated' ? 'bg-sky-500 text-white shadow' : 'bg-white/5 text-gray-300 hover:bg-white/10'"
          @click="activeTab = 'esp32_integrated'"
        >
          🤖 ESP32-Download Linux Runtime (yt-dlp + Telegram Bot)
        </button>
        <button
          class="px-3 py-1.5 rounded text-xs font-semibold transition cursor-pointer"
          :class="activeTab === 'esp32_webui' ? 'bg-purple-500 text-white shadow' : 'bg-white/5 text-gray-300 hover:bg-white/10'"
          @click="activeTab = 'esp32_webui'"
        >
          🖥️ Giao Diện Gốc ESP32-Download (Port 8787 Embedded)
        </button>
        <button
          class="px-3 py-1.5 rounded text-xs font-semibold transition cursor-pointer"
          :class="activeTab === 'direct_aria2' ? 'bg-emerald-500 text-white shadow' : 'bg-white/5 text-gray-300 hover:bg-white/10'"
          @click="activeTab = 'direct_aria2'"
        >
          ⚡ Direct HTTP / Magnet (Aria2 Engine)
        </button>
      </div>

      <div class="flex items-center gap-1.5">
        <button class="btn-secondary text-[11px] py-1" @click="openQrPairing">
          📱 QR App Android & APK
        </button>
        <button class="btn-secondary text-[11px] py-1" @click="showEspConfig = !showEspConfig">
          ⚙ Cấu Hình Runtime
        </button>
        <a
          href="http://192.168.2.1:8787"
          target="_blank"
          class="btn-primary text-[11px] py-1"
        >
          ↗ Mở Cổng :8787
        </a>
      </div>
    </div>

    <div v-if="notice" class="px-3 py-1.5 rounded bg-sky-500/15 border border-sky-500/30 text-sky-300 text-xs font-medium">
      {{ notice }}
    </div>

    <!-- QR Pairing & Android APK Modal -->
    <div v-if="showQrModal" class="pro-card border border-sky-500/40 p-3.5">
      <div class="flex items-center justify-between mb-2">
        <span class="text-xs font-bold text-sky-300 uppercase">Kết Nối App Android (ESP-Download Companion) & Firmware ESP32 CYD</span>
        <button class="text-xs text-gray-400 hover:text-white" @click="showQrModal = false">✕ Đóng</button>
      </div>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-3 items-center text-xs">
        <div class="flex flex-col items-center p-3 rounded bg-black/40 border border-white/10">
          <img v-if="pairingInfo?.qrDataUrl" :src="pairingInfo.qrDataUrl" alt="QR Pairing" class="w-36 h-36 rounded bg-white p-1 mb-2" />
          <span class="mono text-[11px] text-emerald-400">{{ pairingInfo?.runtimeBaseUrl || 'http://192.168.2.1:8787' }}</span>
          <span class="text-[10px] text-gray-400 mt-0.5">Quét mã QR từ ứng dụng Android để đồng bộ</span>
        </div>
        <div class="md:col-span-2 space-y-2">
          <div class="p-2.5 rounded bg-black/30 border border-white/5 space-y-1">
            <div class="font-semibold text-gray-200">Tải Ứng Dụng Android & Firmware ESP32 Trực Tiếp Từ Router</div>
            <div class="flex flex-wrap gap-2 pt-1">
              <a href="/api/local/download-apk" class="btn-primary text-[11px]">⬇ Tải File esp-download.apk (Android)</a>
              <a href="/api/local/firmware/download?model=ESP32-WROOM-32" class="btn-secondary text-[11px]">⬇ Firmware ESP32-WROOM-32</a>
              <a href="/api/local/firmware/download?model=ESP32-S3" class="btn-secondary text-[11px]">⬇ Firmware ESP32-S3</a>
              <a href="/api/local/firmware/download?model=ESP32-C3" class="btn-secondary text-[11px]">⬇ Firmware ESP32-C3</a>
            </div>
          </div>
          <div class="p-2.5 rounded bg-black/30 border border-white/5 mono text-[11px] text-gray-300 space-y-1">
            <div><b>Linux Worker:</b> {{ espStatus.workerId }} (OpenWrt MIPSLE Native)</div>
            <div><b>Output Storage:</b> {{ espConfig.outputDirectory }} (500GB USB 3.0 HDD)</div>
            <div><b>Telegram Bot Polling:</b> {{ espStatus.telegramEnabled ? 'ACTIVE (Auto-detect URLs)' : 'DISABLED' }}</div>
          </div>
        </div>
      </div>
    </div>

    <!-- Collapsible ESP32-Download Linux Runtime Configuration -->
    <form v-if="showEspConfig" class="pro-card border border-sky-500/40 p-3.5 space-y-2.5 text-xs" @submit.prevent="saveEspConfig">
      <div class="flex items-center justify-between">
        <span class="font-bold text-sky-300 uppercase">Cấu Hình ESP32-Download Linux Runtime Trên Router R3P</span>
        <button type="button" class="text-gray-400 hover:text-white" @click="showEspConfig = false">✕ Đóng</button>
      </div>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-2.5">
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Thư Mục Lưu Video (Trên HDD 500GB)</label>
          <input v-model="espConfig.outputDirectory" class="pro-input w-full mono" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Địa Chỉ Mạch ESP32 (Tùy chọn)</label>
          <input v-model="espConfig.deviceBaseUrl" class="pro-input w-full mono" placeholder="http://192.168.1.5" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Runtime Base URL (LAN)</label>
          <input v-model="espConfig.runtimeBaseUrl" class="pro-input w-full mono" placeholder="http://192.168.2.1:8787" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Telegram Bot Token</label>
          <input v-model="espConfig.telegramBotToken" class="pro-input w-full mono" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Telegram Allowed Chat ID</label>
          <input v-model="espConfig.telegramAllowedChatId" class="pro-input w-full mono" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Đường dẫn yt-dlp & ffmpeg trên Linux</label>
          <div class="flex gap-1">
            <input v-model="espConfig.ytDlpPath" class="pro-input w-1/2 mono" placeholder="/usr/bin/yt-dlp" />
            <input v-model="espConfig.ffmpegPath" class="pro-input w-1/2 mono" placeholder="/usr/bin/ffmpeg" />
          </div>
        </div>
      </div>
      <div class="flex items-center justify-between pt-2 border-t border-white/10">
        <label class="flex items-center gap-2 cursor-pointer">
          <input v-model="espConfig.telegramEnabled" type="checkbox" />
          <span>Tự động nhận link video từ Telegram Bot 24/7 ngay trên Router Linux (không cần bật PC Windows)</span>
        </label>
        <button type="submit" class="btn-primary">Lưu Cấu Hình Runtime</button>
      </div>
    </form>

    <!-- MODE 1: ESP32-DOWNLOAD INTEGRATED LINUX DASHBOARD -->
    <template v-if="activeTab === 'esp32_integrated'">
      <!-- Status Telemetry Cards -->
      <div class="grid grid-cols-2 md:grid-cols-5 gap-2">
        <div class="stat-mini">
          <div class="text-[10px] text-gray-400 uppercase">Linux Host Runtime</div>
          <div class="mono text-xs font-bold text-emerald-400 flex items-center gap-1.5 mt-0.5">
            <span class="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
            PORT 8787 ONLINE
          </div>
          <div class="text-[10px] text-gray-500 truncate">{{ espStatus.workerId }}</div>
        </div>

        <div class="stat-mini">
          <div class="text-[10px] text-gray-400 uppercase">yt-dlp & FFmpeg Engine</div>
          <div class="mono text-xs font-bold" :class="espStatus.ytDlpInstalled ? 'text-sky-400' : 'text-amber-400'">
            {{ espStatus.ytDlpInstalled ? 'yt-dlp READY' : 'INSTALLING…' }}
            · {{ espStatus.ffmpegInstalled ? 'FFmpeg OK' : 'No FFmpeg' }}
          </div>
          <div class="text-[10px] text-gray-500">YouTube / TikTok / FB / IG / X</div>
        </div>

        <div class="stat-mini">
          <div class="text-[10px] text-gray-400 uppercase">Telegram Bot Poller</div>
          <div class="mono text-xs font-bold" :class="espStatus.telegramEnabled ? 'text-emerald-400' : 'text-gray-400'">
            {{ espStatus.telegramEnabled ? 'POLLING 24/7' : 'PAUSED' }}
          </div>
          <div class="text-[10px] text-gray-500">Chat ID: {{ espConfig.telegramAllowedChatId || 'All' }}</div>
        </div>

        <div class="stat-mini">
          <div class="text-[10px] text-gray-400 uppercase">500GB HDD Output</div>
          <div class="mono text-xs font-bold text-purple-300 truncate">{{ espConfig.outputDirectory }}</div>
          <div class="text-[10px] text-gray-500">Free: {{ formatBytes(espStatus.outputFreeBytes || 492000000000) }}</div>
        </div>

        <div class="stat-mini">
          <div class="text-[10px] text-gray-400 uppercase">30-Day Completed</div>
          <div class="mono text-xs font-bold text-amber-400">
            {{ espStatus.completedLast30DaysCount || 0 }} Videos ({{ formatBytes(espStatus.completedLast30DaysBytes || 0) }})
          </div>
          <div class="text-[10px] text-gray-500">Queue: {{ espJobs.length }} items</div>
        </div>
      </div>

      <!-- Queue Video / Social URL via yt-dlp -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Tải Video Mạng Xã Hội Bằng yt-dlp (YouTube, TikTok, Facebook, Instagram, Douyin, X…)</span>
          <span class="mono text-[10.5px] text-emerald-400">Lưu trực tiếp vào {{ espConfig.outputDirectory }}</span>
        </div>
        <form class="grid grid-cols-1 md:grid-cols-12 gap-2" @submit.prevent="queueYtDlpJob">
          <input
            v-model="mediaUrl"
            type="url"
            required
            class="pro-input md:col-span-6 mono text-xs"
            placeholder="Dán link YouTube, TikTok, Facebook Reel, Instagram, Twitter/X, Playlist…"
          />
          <input
            v-model="scheduleTime"
            type="datetime-local"
            class="pro-input md:col-span-3 mono text-xs"
            title="Hẹn giờ tải (Để trống nếu muốn tải ngay lập tức)"
          />
          <select v-model="scheduleRecurrence" class="pro-input md:col-span-1 text-xs">
            <option value="once">1 Lần</option>
            <option value="daily">Hàng ngày</option>
            <option value="weekly">Hàng tuần</option>
          </select>
          <button
            type="submit"
            class="btn-primary md:col-span-2 text-xs font-semibold"
            :disabled="queueingMedia || !mediaUrl.trim()"
          >
            {{ queueingMedia ? 'Đang xử lý…' : (scheduleTime ? '⏰ Hẹn Giờ Tải' : '⬇ Tải Ngay (yt-dlp)') }}
          </button>
        </form>
      </div>

      <!-- Scheduled Downloads List (if any) -->
      <div v-if="espScheduled.length > 0" class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Lịch Hẹn Giờ Tải Tự Động ({{ espScheduled.length }})</span>
        </div>
        <div class="overflow-x-auto">
          <table class="pro-table">
            <thead>
              <tr>
                <th>URL</th>
                <th>Thời Gian Hẹn</th>
                <th>Lặp Lại</th>
                <th>Trạng Thái</th>
                <th>Thao Tác</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in espScheduled" :key="s.id">
                <td class="mono text-xs text-sky-300 max-w-md truncate">{{ s.url }}</td>
                <td class="mono text-xs text-gray-200">{{ s.scheduledAt }}</td>
                <td><span class="badge badge-purple uppercase">{{ s.recurrence }}</span></td>
                <td><span class="badge badge-blue">{{ s.status }}</span></td>
                <td>
                  <button class="btn-danger text-[10px] py-0.5 px-2" @click="deleteScheduled(s.id)">Xóa</button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- ESP32-Download Runtime Queue & History Table -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Hàng Đợi & Lịch Sử Tải Video (ESP32-Download Linux Queue · {{ espJobs.length }})</span>
          <div class="flex items-center gap-1.5">
            <button class="btn-secondary text-[11px] py-0.5" @click="fetchEspRuntime"> Làm Mới ↻</button>
            <button class="btn-danger text-[11px] py-0.5" @click="clearEspHistory">Xóa Lịch Sử Đã Xong</button>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="pro-table">
            <thead>
              <tr>
                <th>Nền Tảng</th>
                <th>URL & File Đầu Ra</th>
                <th>Trạng Thái</th>
                <th>Tiến Độ</th>
                <th>Dung Lượng</th>
                <th>Thao Tác</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!espJobs.length">
                <td colspan="6" class="text-center text-gray-500 py-5">
                  Chưa có tác vụ nào trong hàng đợi. Dán link video ở trên hoặc gửi link qua Telegram Bot để Router tự động tải về ổ cứng 500GB!
                </td>
              </tr>
              <tr v-for="job in espJobs" :key="job.id">
                <td>
                  <span class="badge badge-blue uppercase mono text-[10px]">{{ job.platform || 'VIDEO' }}</span>
                </td>
                <td class="max-w-md">
                  <div class="mono text-xs text-sky-300 truncate" :title="job.url">{{ job.url }}</div>
                  <div v-if="job.outputPath" class="mono text-[10.5px] text-emerald-400 truncate">📁 {{ job.outputPath }}</div>
                  <div v-if="job.errorMessage" class="text-[10.5px] text-red-400 truncate">⚠ {{ job.errorMessage }}</div>
                </td>
                <td>
                  <span
                    class="badge text-[10px]"
                    :class="job.runtimeStatus === 'COMPLETED'
                      ? 'badge-green'
                      : job.runtimeStatus === 'DOWNLOADING'
                        ? 'badge-blue'
                        : job.runtimeStatus === 'PENDING'
                          ? 'badge-orange'
                          : 'badge-red'"
                  >
                    {{ job.runtimeStatus }}
                  </span>
                </td>
                <td class="w-36">
                  <div class="flex items-center gap-2">
                    <div class="progress-bar-bg flex-1">
                      <div
                        class="progress-bar-fill bg-sky-400"
                        :style="{ width: `${Math.min(100, Math.max(0, job.progressPercent || 0))}%` }"
                      ></div>
                    </div>
                    <span class="mono text-[10px] text-gray-300">{{ Math.round(job.progressPercent || 0) }}%</span>
                  </div>
                </td>
                <td class="mono text-xs text-gray-300">
                  {{ job.outputBytes ? formatBytes(job.outputBytes) : '—' }}
                </td>
                <td>
                  <div class="flex items-center gap-1">
                    <button
                      v-if="job.runtimeStatus !== 'DOWNLOADING'"
                      class="btn-primary text-[10px] py-0.5 px-2"
                      @click="espJobAction(job.id, 'run')"
                    >
                      Tải Lại
                    </button>
                    <button
                      v-if="job.runtimeStatus === 'DOWNLOADING'"
                      class="btn-secondary text-[10px] py-0.5 px-2"
                      @click="espJobAction(job.id, 'cancel')"
                    >
                      Hủy
                    </button>
                    <a
                      v-if="job.outputPath"
                      :href="`/api/v1/nas/download?path=${encodeURIComponent(job.outputPath)}`"
                      class="btn-secondary text-[10px] py-0.5 px-2"
                    >
                      Lưu Về PC
                    </a>
                    <button
                      class="btn-danger text-[10px] py-0.5 px-2"
                      @click="espJobAction(job.id, 'delete')"
                    >
                      Xóa
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>

    <!-- MODE 2: EMBEDDED ORIGINAL ESP32-DOWNLOAD WEB UI -->
    <template v-else-if="activeTab === 'esp32_webui'">
      <div class="pro-card p-0 overflow-hidden h-[calc(100vh-8.5rem)]">
        <iframe
          src="/esp32-download/"
          class="w-full h-full border-0 bg-[#070b14]"
          title="ESP32-Download Embedded Dashboard"
        ></iframe>
      </div>
    </template>

    <!-- MODE 3: DIRECT HTTP / HTTPS / MAGNET (ARIA2) DOWNLOADER -->
    <template v-else>
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Add Direct HTTP / HTTPS / Magnet Download Task</span>
          <span class="mono text-xs text-sky-400">Speed: ↓ {{ formatBytes(globalDownloadSpeed) }}/s</span>
        </div>
        <div class="flex flex-col sm:flex-row items-center gap-2">
          <input
            v-model="newUrl"
            type="url"
            class="pro-input flex-1 w-full mono text-[12px]"
            placeholder="Enter direct file HTTP / HTTPS URL or Magnet link…"
            @keyup.enter="addUrl"
          />
          <select v-model="downloadDir" class="pro-input w-full sm:w-64 mono text-[11.5px]" @change="saveSettings">
            <option value="/mnt/sda1/downloads">/mnt/sda1/downloads (USB 3.0 500GB HDD)</option>
            <option value="/mnt/sda1/share">/mnt/sda1/share (USB 500GB Shared)</option>
            <option value="/overlay/downloads">/overlay/downloads (NAND 202MB)</option>
            <option value="/tmp/downloads">/tmp/downloads (RAM 216MB)</option>
          </select>
          <button
            :disabled="adding || !newUrl.trim()"
            class="btn-primary shrink-0 w-full sm:w-auto disabled:opacity-50"
            @click="addUrl"
          >
            {{ adding ? 'Starting…' : '+ Start Download' }}
          </button>
        </div>
      </div>

      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Direct Download Queue ({{ tasks.length }})</span>
          <button class="btn-secondary text-[11px] py-1" @click="fetchDownloads">Refresh Now ↻</button>
        </div>

        <div class="overflow-x-auto">
          <table class="pro-table">
            <thead>
              <tr>
                <th>File Name & Target</th>
                <th>Status</th>
                <th>Progress</th>
                <th>Size</th>
                <th>Speed</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!tasks.length">
                <td colspan="6" class="text-center text-gray-500 py-4">
                  Direct queue is empty.
                </td>
              </tr>
              <tr v-for="t in tasks" :key="t.gid">
                <td class="max-w-xs">
                  <div class="font-semibold text-gray-200 truncate" :title="t.url">{{ t.name }}</div>
                  <div class="mono text-[10px] text-gray-500 truncate">{{ t.file_path }}</div>
                </td>
                <td>
                  <span class="badge text-[10px]" :class="t.status === 'complete' ? 'badge-green' : t.status === 'active' ? 'badge-blue' : 'badge-orange'">
                    {{ t.status.toUpperCase() }}
                  </span>
                </td>
                <td class="w-40">
                  <div class="progress-bar-bg">
                    <div
                      class="progress-bar-fill bg-sky-400"
                      :style="{ width: `${t.totalLength > 0 ? Math.min(100, (t.completedLength / t.totalLength) * 100) : (t.status === 'complete' ? 100 : 0)}%` }"
                    ></div>
                  </div>
                </td>
                <td class="mono text-gray-300">{{ formatBytes(t.completedLength) }} / {{ formatBytes(t.totalLength) }}</td>
                <td class="mono text-emerald-400">{{ t.downloadSpeed ? formatBytes(t.downloadSpeed) + '/s' : '—' }}</td>
                <td>
                  <div class="flex items-center gap-1">
                    <button v-if="t.status === 'active'" class="btn-secondary text-[10px] py-0.5 px-2" @click="taskAction(t.gid, 'pause')">Pause</button>
                    <button v-if="t.status === 'paused'" class="btn-primary text-[10px] py-0.5 px-2" @click="taskAction(t.gid, 'resume')">Resume</button>
                    <a v-if="t.status === 'complete'" :href="`/api/v1/downloads/file?gid=${t.gid}`" class="btn-primary text-[10px] py-0.5 px-2">Save</a>
                    <button class="btn-danger text-[10px] py-0.5 px-2" @click="taskAction(t.gid, 'remove')">Delete</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>
