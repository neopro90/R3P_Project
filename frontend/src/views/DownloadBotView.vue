<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { apiRequest } from '@/services/api'

interface DownloadJob {
  id: string
  url: string
  source: string
  quality?: string
  status: string // "RUNNING" | "DOWNLOADING" | "QUEUED" | "COMPLETED" | "FAILED"
  progress: number
  speed: string
  eta: string
  outputPath: string
  outputBytes: number
  error: string
  createdAt: string
  finishedAt: string
}

interface DownloadFileItem {
  name: string
  sub_path: string
  full_path: string
  size: number
  mod_time: string
  category: string
}

interface TelegramLogItem {
  id: string
  updateId: number
  chatId: string
  url: string
  decision: string
  reason: string
  jobId: string
  jobStatus: string
  receivedAt: string
}

const jobs = ref<DownloadJob[]>([])
const downloadedFiles = ref<DownloadFileItem[]>([])
const activeTab = ref<'all' | 'running' | 'completed' | 'failed'>('all')

const outputDir = ref('/mnt/sda1/downloads')
const savingOutputDir = ref(false)
const showEditOutputDir = ref(false)

const newUrl = ref('')
const selectedQuality = ref('best')
const addingJob = ref(false)
const notice = ref<string | null>(null)

const showDebugPanel = ref(false)
const runningDebug = ref(false)
const testingTelegram = ref(false)
const debugResult = ref<any>(null)
const telegramLogs = ref<TelegramLogItem[]>([])
const debugNotice = ref<string | null>(null)

let pollTimer: ReturnType<typeof setInterval> | null = null

const filteredJobs = computed(() => {
  if (activeTab.value === 'all') return jobs.value
  if (activeTab.value === 'running') {
    return jobs.value.filter(j => j.status === 'RUNNING' || j.status === 'DOWNLOADING' || j.status === 'QUEUED')
  }
  if (activeTab.value === 'completed') {
    return jobs.value.filter(j => j.status === 'COMPLETED')
  }
  if (activeTab.value === 'failed') {
    return jobs.value.filter(j => j.status === 'FAILED' || j.status.startsWith('ERR'))
  }
  return jobs.value
})

const runningJobsCount = computed(() => {
  return jobs.value.filter(j => j.status === 'RUNNING' || j.status === 'DOWNLOADING' || j.status === 'QUEUED').length
})

const completedJobsCount = computed(() => {
  return jobs.value.filter(j => j.status === 'COMPLETED').length
})

const failedJobsCount = computed(() => {
  return jobs.value.filter(j => j.status === 'FAILED' || j.status.startsWith('ERR')).length
})

async function fetchJobs() {
  try {
    const res = await apiRequest<{ jobs: DownloadJob[] }>('/download/jobs')
    if (res?.jobs) {
      jobs.value = res.jobs
    }
  } catch {
    // ignore
  }
}

async function fetchDownloadSettings() {
  try {
    const res = await apiRequest<{ output_directory: string }>('/download/settings')
    if (res?.output_directory) {
      outputDir.value = res.output_directory
    }
  } catch {
    // ignore
  }
}

async function saveOutputDir() {
  const p = outputDir.value.trim()
  if (!p) return
  savingOutputDir.value = true
  try {
    const res = await apiRequest<{ message: string; output_directory: string }>('/download/settings', {
      method: 'POST',
      body: JSON.stringify({ output_dir: p }),
    })
    notice.value = res?.message || `Đã đổi thư mục lưu trữ thành: ${p}`
    showEditOutputDir.value = false
    await fetchDownloadedFiles()
    setTimeout(() => { notice.value = null }, 3500)
  } catch (err: any) {
    notice.value = 'Lỗi lưu thư mục: ' + (err?.message || err)
    setTimeout(() => { notice.value = null }, 4000)
  } finally {
    savingOutputDir.value = false
  }
}

async function fetchDownloadedFiles() {
  try {
    const res = await apiRequest<{ output_directory: string; total: number; files: DownloadFileItem[] }>('/download/files')
    if (res?.files) {
      downloadedFiles.value = res.files
      if (res.output_directory) {
        outputDir.value = res.output_directory
      }
    }
  } catch {
    // USB might not have files yet
  }
}

async function addDownloadJob() {
  const u = newUrl.value.trim()
  if (!u) {
    notice.value = 'Vui lòng nhập liên kết video/âm thanh hoặc file cần tải!'
    return
  }
  addingJob.value = true
  notice.value = 'Đang đẩy link vào hàng đợi R3P Local Downloader…'
  try {
    const res = await apiRequest<{ duplicate?: boolean; job?: { id: string; url: string; quality?: string; status: string } }>('/download/jobs', {
      method: 'POST',
      body: JSON.stringify({
        url: u,
        quality: selectedQuality.value,
        force: false,
      }),
    })

    if (res?.job?.id) {
      notice.value = `Đã thêm vào hàng đợi tải xuống thành công! #${res.job.id}`
      newUrl.value = ''
      await fetchJobs()
    }
  } catch (err: any) {
    notice.value = `Lỗi thêm URL: ${err?.message || err}`
  } finally {
    addingJob.value = false
    setTimeout(() => { notice.value = null }, 3500)
  }
}

async function deleteJob(id: string) {
  try {
    await apiRequest(`/download/jobs/${encodeURIComponent(id)}`, { method: 'DELETE' })
    await fetchJobs()
  } catch (err: any) {
    notice.value = `Lỗi xóa job: ${err?.message || err}`
    setTimeout(() => { notice.value = null }, 3500)
  }
}

async function clearFinishedJobs() {
  try {
    await apiRequest('/download/jobs', { method: 'DELETE' })
    notice.value = 'Đã dọn sạch các job đã hoàn tất!'
    await fetchJobs()
    setTimeout(() => { notice.value = null }, 2500)
  } catch (err: any) {
    notice.value = `Lỗi dọn hàng đợi: ${err?.message || err}`
    setTimeout(() => { notice.value = null }, 3500)
  }
}

async function retryJob(job: DownloadJob) {
  try {
    await apiRequest(`/download/jobs/${encodeURIComponent(job.id)}/retry`, {
      method: 'POST',
    })
    notice.value = 'Đã thử lại tải link!'
    await fetchJobs()
    setTimeout(() => { notice.value = null }, 2500)
  } catch (err: any) {
    notice.value = `Không thể thử lại job: ${err?.message || err}`
    setTimeout(() => { notice.value = null }, 3500)
  }
}

async function fetchTelegramLogs() {
  try {
    const res = await apiRequest<{ entries: TelegramLogItem[] }>('/download/telegram/log')
    if (Array.isArray(res?.entries)) {
      telegramLogs.value = res.entries
    }
  } catch {
    // ignore
  }
}

async function clearTelegramLogs() {
  try {
    await apiRequest('/download/telegram/log', { method: 'DELETE' })
    telegramLogs.value = []
    debugNotice.value = 'Đã xoá nhật ký phân tích Telegram.'
    setTimeout(() => { debugNotice.value = null }, 3000)
  } catch (err: any) {
    debugNotice.value = `Lỗi xoá log: ${err?.message || err}`
  }
}

async function sendTelegramTest() {
  testingTelegram.value = true
  debugNotice.value = 'Đang gửi tin nhắn thử tới Telegram...'
  try {
    const res = await apiRequest<{ sent: boolean; chat_id?: string }>('/download/telegram/test', {
      method: 'POST',
      body: JSON.stringify({}),
    })
    debugNotice.value = `✅ Đã gửi tin thử thành công tới Chat ID ${res?.chat_id || 'đã cấu hình'}!`
    await runDiagnostic(false)
  } catch (err: any) {
    debugNotice.value = `❌ Lỗi gửi Telegram: ${err?.message || err}`
  } finally {
    testingTelegram.value = false
    setTimeout(() => { debugNotice.value = null }, 5000)
  }
}

async function runDiagnostic(autoFix = false) {
  runningDebug.value = true
  debugNotice.value = autoFix ? 'Đang tự động đồng bộ và sửa cấu hình hệ thống…' : 'Đang kiểm tra kết nối IP, Telegram, Dịch vụ R3P và Thư mục…'
  try {
    const method = autoFix ? 'POST' : 'GET'
    const res = await apiRequest<any>('/download/debug', { method })
    debugResult.value = res
    if (Array.isArray(res?.telegram_logs)) {
      telegramLogs.value = res.telegram_logs
    } else {
      await fetchTelegramLogs()
    }
    debugNotice.value = res?.message || 'Kiểm tra hoàn tất!'
    if (autoFix) {
      await fetchJobs()
      await fetchDownloadedFiles()
    }
  } catch (err: any) {
    debugNotice.value = `Lỗi kiểm tra: ${err?.message || err}`
  } finally {
    runningDebug.value = false
    setTimeout(() => { debugNotice.value = null }, 4000)
  }
}

function toggleDebugPanel() {
  showDebugPanel.value = !showDebugPanel.value
  if (showDebugPanel.value) {
    void runDiagnostic(false)
  }
}

function telegramStatusBadge(res: any): string {
  const st = res?.telegram_polling_state
  if (st === 'polling') return 'badge-green'
  if (st === 'conflict' || st === 'auth_error' || st === 'error') return 'badge-red'
  if (st === 'disabled' || st === 'unconfigured') return 'badge-orange'
  return 'badge-blue'
}

function telegramStatusLabel(res: any): string {
  const st = res?.telegram_polling_state
  if (st === 'polling') return 'ĐANG POLLING'
  if (st === 'conflict') return 'XUNG ĐỘT 409'
  if (st === 'auth_error') return 'SAI TOKEN'
  if (st === 'error') return 'LỖI KẾT NỐI'
  if (st === 'disabled') return 'ĐÃ TẮT'
  if (st === 'unconfigured') return 'CHƯA CẤU HÌNH'
  return 'ĐANG KHỞI TẠO'
}

function logDecisionClass(decision: string): string {
  if (decision === 'ACCEPTED') return 'badge-green'
  if (decision === 'AI_AGENT') return 'badge-blue'
  if (decision === 'HELP') return 'badge-purple'
  if (decision === 'DUPLICATE') return 'badge-orange'
  return 'badge-red'
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`
}

function statusBadgeClass(st: string): string {
  if (st === 'COMPLETED') return 'badge-green'
  if (st === 'RUNNING' || st === 'DOWNLOADING') return 'badge-blue animate-pulse'
  if (st === 'QUEUED') return 'badge-orange'
  return 'badge-red'
}

function statusLabel(st: string): string {
  if (st === 'COMPLETED') return 'Hoàn Tất'
  if (st === 'RUNNING' || st === 'DOWNLOADING') return 'Đang Tải'
  if (st === 'QUEUED') return 'Chờ Tải'
  return 'Lỗi'
}

onMounted(() => {
  void fetchJobs()
  void fetchDownloadedFiles()
  void fetchDownloadSettings()
  void runDiagnostic(false)
  pollTimer = setInterval(() => {
    void fetchJobs()
  }, 2500)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="p-4 space-y-4 max-w-7xl mx-auto">
    <!-- Floating Non-Shifting Notice Toast -->
    <div
      v-if="notice"
      class="r3p-toast"
      :class="notice.includes('Lỗi') ? 'r3p-toast-error' : 'r3p-toast-success'"
    >
      <span class="r3p-toast-text">{{ notice }}</span>
      <button type="button" class="text-xs opacity-75 hover:opacity-100 px-1 cursor-pointer" @click="notice = null">✕</button>
    </div>

    <!-- Top Telemetry & Control Bar -->
    <div class="pro-card p-3.5 flex flex-wrap items-center justify-between gap-3 shadow-md border-sky-500/20">
      <div class="flex items-center gap-3 min-w-0">
        <span class="w-3 h-3 rounded-full bg-emerald-400 animate-pulse shrink-0"></span>
        <div class="min-w-0">
          <h1 class="text-[16px] font-bold text-gray-100 flex items-center gap-2">
            <span class="truncate">Trình Tải Video yt-dlp Trực Tiếp Trên R3P (Native Downloader)</span>
            <span class="badge badge-green text-xs font-mono font-bold">NATIVE YT-DLP</span>
          </h1>
          <p class="text-xs text-gray-400 font-medium truncate">
            R3P nhận link trực tiếp và giao cho yt-dlp tải về ổ cứng USB 3.0 (/mnt/sda1) · Tự động nhận link qua Telegram Bot & Web.
          </p>
        </div>
      </div>

      <div class="flex items-center gap-2 shrink-0">
        <button
          type="button"
          class="btn-primary py-1.5 px-3 min-w-[205px] text-xs font-bold flex items-center gap-1.5 cursor-pointer shadow-sm"
          @click="toggleDebugPanel"
        >
          <span>⚡</span>
          <span>{{ showDebugPanel ? 'Ẩn Panel Debug' : 'Kiểm Tra / Debug Đồng Bộ' }}</span>
        </button>
        <button
          type="button"
          class="btn-secondary py-1.5 px-3 text-xs font-semibold flex items-center gap-1.5 cursor-pointer"
          @click="fetchJobs(); fetchDownloadedFiles()"
        >
          <span>🔄</span>
          <span>Làm Mới</span>
        </button>
      </div>
    </div>

    <!-- Diagnostic & Debug Panel -->
    <div
      v-if="showDebugPanel"
      class="bg-[#0b1220] border border-sky-500/30 rounded-xl p-3.5 shadow-xl text-xs space-y-2.5"
    >
      <div class="flex items-center justify-between pb-1.5 border-b border-white/10">
        <div class="flex items-center gap-2">
          <span class="text-sm font-bold text-sky-400">⚡ Chẩn Đoán & Đồng Bộ Thông Số Hệ Thống Download</span>
          <span v-if="debugResult?.timestamp" class="mono text-[10.5px] text-gray-400">({{ debugResult.timestamp }})</span>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="btn-secondary text-xs py-1 px-2.5"
            :disabled="testingTelegram"
            @click="sendTelegramTest"
          >
            <span>📨</span> {{ testingTelegram ? 'Đang gửi...' : 'Gửi Tin Thử Telegram' }}
          </button>
          <button
            type="button"
            class="btn-primary text-xs py-1 px-2.5"
            :disabled="runningDebug"
            @click="runDiagnostic(true)"
          >
            <span>🔄</span> Tự Động Sửa & Đồng Bộ Lại
          </button>
          <button
            type="button"
            class="text-gray-400 hover:text-white px-1 text-sm"
            @click="showDebugPanel = false"
          >
            ✕
          </button>
        </div>
      </div>

      <!-- Action Toast Notice -->
      <div v-if="debugNotice" class="p-2 rounded bg-sky-500/20 border border-sky-400/40 text-sky-200 text-xs flex items-center gap-2">
        <span class="animate-spin text-sm" v-if="runningDebug">⚙</span>
        <span>{{ debugNotice }}</span>
      </div>

      <!-- Telegram Error Banner -->
      <div
        v-if="debugResult?.telegram_last_error"
        class="p-2.5 rounded-lg bg-red-500/15 border border-red-500/40 text-red-200 text-xs space-y-1"
      >
        <div class="font-bold text-red-300">⚠️ Lỗi nhận tin nhắn Telegram ({{ telegramStatusLabel(debugResult) }}):</div>
        <div class="mono break-all text-[11px]">{{ debugResult.telegram_last_error }}</div>
      </div>

      <!-- 4 Diagnostic Cards -->
      <div class="grid grid-cols-1 md:grid-cols-4 gap-2.5 text-[13px]">
        <div class="p-3 rounded-lg bg-black/30 border border-white/10 space-y-1.5">
          <div class="flex items-center justify-between font-bold text-gray-100">
            <span>🌐 R3P Local Runtime (Cục Bộ)</span>
            <span class="badge" :class="debugResult?.service_online !== false ? 'badge-green' : 'badge-red'">
              {{ debugResult?.service_online !== false ? `HTTPS :${debugResult?.https_port || 8443} OK` : 'DỊCH VỤ LỖI' }}
            </span>
          </div>
          <div class="text-gray-300">LAN IP: <span class="mono text-gray-100 font-bold">{{ debugResult?.lan_ip || '192.168.2.1' }}</span></div>
          <div class="text-gray-300 truncate">URL: <span class="mono text-sky-400 font-semibold">{{ debugResult?.service_url || `https://${debugResult?.lan_ip || '192.168.2.1'}:8443/downloads` }}</span></div>
        </div>

        <div class="p-3 rounded-lg bg-black/30 border border-white/10 space-y-1.5">
          <div class="flex items-center justify-between font-bold text-gray-100">
            <span>💾 Thư Mục Đầu Ra (USB)</span>
            <span class="badge" :class="debugResult?.output_dir_writable ? 'badge-green' : 'badge-orange'">
              {{ debugResult?.output_dir_writable ? 'SẴN SÀNG GHI' : 'CHƯA CÓ QUYỀN' }}
            </span>
          </div>
          <div class="text-gray-300 truncate mono text-[12px] font-medium">{{ debugResult?.output_dir || '/mnt/sda1/downloads' }}</div>
          <div class="text-gray-300">Trống: <span class="mono text-emerald-400 font-bold">{{ debugResult?.storage?.freeGb || '0' }} GB</span></div>
        </div>

        <div class="p-3 rounded-lg bg-black/30 border border-white/10 space-y-1.5">
          <div class="flex items-center justify-between font-bold text-gray-100">
            <span>🎬 Trình Tải Media (yt-dlp)</span>
            <span class="badge" :class="debugResult?.yt_dlp_version ? 'badge-green' : 'badge-orange'">
              {{ debugResult?.yt_dlp_version ? 'SẴN SÀNG' : 'CHƯA TÌM THẤY' }}
            </span>
          </div>
          <div class="text-gray-300">Phiên bản: <span class="mono text-emerald-300 font-bold">{{ debugResult?.yt_dlp_version || 'N/A' }}</span></div>
          <div class="text-gray-300">Đường dẫn: <span class="mono text-gray-200">/usr/bin/yt-dlp</span></div>
        </div>

        <div class="p-3 rounded-lg bg-black/30 border border-white/10 space-y-1.5">
          <div class="flex items-center justify-between font-bold text-gray-100">
            <span>🤖 Telegram & Tự Động</span>
            <span class="badge" :class="telegramStatusBadge(debugResult)">
              {{ telegramStatusLabel(debugResult) }}
            </span>
          </div>
          <div class="text-gray-300 truncate">
            Bot:
            <span class="mono text-emerald-400 font-bold">
              {{ debugResult?.telegram_token_set ? ((debugResult?.telegram_bot_username ? '@' + debugResult.telegram_bot_username + ' ' : '') + '(****' + (debugResult?.telegram_token_last4 || '') + ')') : 'Chưa Nhập' }}
            </span>
          </div>
          <div class="text-gray-300 truncate">Chat ID: <span class="mono text-gray-100 font-bold">{{ debugResult?.telegram_chat_id || 'Chưa Cấu Hình' }}</span></div>
          <div class="text-[11px] text-gray-400 mono truncate">
            Offset: {{ debugResult?.telegram_update_offset || 0 }} · Webhook: {{ debugResult?.telegram_webhook_url ? 'Có (Sẽ tự xoá)' : 'Trống' }}
          </div>
        </div>
      </div>

      <!-- Telegram Routing Log Table -->
      <div class="p-3 rounded-lg bg-black/30 border border-white/10 space-y-2">
        <div class="flex items-center justify-between">
          <span class="font-bold text-sky-300 text-xs">
            🧭 Nhật Ký Phân Tích Tin Nhắn Telegram (Chỉ URL → Downloader · Tin Thường → AI)
          </span>
          <button
            v-if="telegramLogs.length > 0"
            type="button"
            class="text-[11px] text-gray-400 hover:text-red-300 cursor-pointer"
            @click="clearTelegramLogs"
          >
            🗑️ Xoá Nhật Ký
          </button>
        </div>
        <div v-if="telegramLogs.length === 0" class="text-[11.5px] text-gray-400 py-2">
          Chưa có sự kiện phân tích tin nhắn Telegram nào từ lúc khởi động.
        </div>
        <div v-else class="overflow-x-auto max-h-48 overflow-y-auto">
          <table class="w-full text-left text-[11.5px] border-collapse">
            <thead class="text-gray-400 border-b border-white/10">
              <tr>
                <th class="py-1 pr-2">Thời Gian</th>
                <th class="py-1 pr-2">Chat</th>
                <th class="py-1 pr-2">Định Tuyến</th>
                <th class="py-1 pr-2">Nội Dung / URL</th>
                <th class="py-1 pr-2">Chi Tiết</th>
                <th class="py-1">Trạng Thái</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-white/5 text-gray-200">
              <tr v-for="item in telegramLogs" :key="item.id">
                <td class="py-1 pr-2 mono text-gray-400 whitespace-nowrap">{{ item.receivedAt }}</td>
                <td class="py-1 pr-2 mono whitespace-nowrap">{{ item.chatId }}</td>
                <td class="py-1 pr-2 whitespace-nowrap">
                  <span class="badge text-[10px]" :class="logDecisionClass(item.decision)">{{ item.decision }}</span>
                </td>
                <td class="py-1 pr-2 max-w-[220px] truncate mono text-sky-300" :title="item.url">{{ item.url }}</td>
                <td class="py-1 pr-2 text-gray-300">{{ item.reason }}</td>
                <td class="py-1 mono text-emerald-300 whitespace-nowrap">{{ item.jobStatus }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Quick Add Download URL Card -->
    <div class="pro-card p-4 space-y-3">
      <div class="flex items-center justify-between pb-2 border-b border-white/10">
        <span class="pro-card-title">📥 Thêm Tác Vụ Tải Xuống Mới (Direct URL / Video / Audio)</span>
        <span class="text-[12.5px] text-gray-300 font-medium">Hỗ trợ: YouTube, TikTok, Facebook, Douyin, X, Bilibili, HTTP/HTTPS direct</span>
      </div>

      <form class="flex flex-col sm:flex-row items-center gap-2.5" @submit.prevent="addDownloadJob">
        <div class="relative flex-1 w-full">
          <input
            v-model="newUrl"
            type="text"
            required
            class="w-full pro-input text-[14px] font-mono py-2.5 px-3.5 text-sky-300 placeholder-gray-400"
            placeholder="Dán liên kết video, audio hoặc file tải xuống vào đây…"
          />
        </div>

        <div class="flex items-center gap-2 shrink-0 w-full sm:w-auto">
          <select v-model="selectedQuality" class="pro-input text-[13px] py-2 px-3 font-medium text-gray-100">
            <option value="best">🎬 Video Tốt Nhất (Best)</option>
            <option value="1080p">📺 1080p Full HD</option>
            <option value="720p">📱 720p Tiết Kiệm</option>
            <option value="audio">🎵 Chỉ Tải Âm Thanh (MP3)</option>
          </select>

          <button
            type="submit"
            class="btn-primary py-2 px-4 min-w-[144px] text-[13px] font-bold shrink-0 cursor-pointer shadow-md flex items-center gap-1.5"
            :disabled="addingJob"
          >
            <span>⚡</span>
            <span>{{ addingJob ? 'Đang thêm…' : 'Tải Về Ổ Cứng' }}</span>
          </button>
        </div>
      </form>
    </div>

    <!-- Jobs Queue & Progress Table -->
    <div class="pro-card p-4 space-y-3.5">
      <div class="flex flex-wrap items-center justify-between gap-2 pb-2.5 border-b border-white/10">
        <div class="flex items-center gap-2">
          <span class="pro-card-title">Danh Sách Hàng Đợi Tải Xuống (Queue Jobs)</span>
          <span class="badge badge-blue font-mono font-bold">{{ jobs.length }} Jobs</span>
        </div>

        <!-- Filter Tabs & Actions -->
        <div class="flex items-center gap-2 text-[12.5px]">
          <div class="flex items-center bg-black/30 border border-white/10 rounded-lg p-0.5 gap-0.5">
            <button
              type="button"
              class="px-3 py-1.5 rounded-md font-semibold transition cursor-pointer"
              :class="activeTab === 'all' ? 'bg-sky-600 text-white' : 'text-gray-300 hover:text-white'"
              @click="activeTab = 'all'"
            >
              Tất Cả ({{ jobs.length }})
            </button>
            <button
              type="button"
              class="px-3 py-1.5 rounded-md font-semibold transition cursor-pointer"
              :class="activeTab === 'running' ? 'bg-sky-600 text-white' : 'text-gray-300 hover:text-white'"
              @click="activeTab = 'running'"
            >
              Đang Tải ({{ runningJobsCount }})
            </button>
            <button
              type="button"
              class="px-3 py-1.5 rounded-md font-semibold transition cursor-pointer"
              :class="activeTab === 'completed' ? 'bg-sky-600 text-white' : 'text-gray-300 hover:text-white'"
              @click="activeTab = 'completed'"
            >
              Hoàn Tất ({{ completedJobsCount }})
            </button>
            <button
              type="button"
              class="px-3 py-1.5 rounded-md font-semibold transition cursor-pointer"
              :class="activeTab === 'failed' ? 'bg-sky-600 text-white' : 'text-gray-300 hover:text-white'"
              @click="activeTab = 'failed'"
            >
              Lỗi ({{ failedJobsCount }})
            </button>
          </div>

          <button
            v-if="completedJobsCount > 0"
            type="button"
            class="btn-secondary py-1.5 px-3 text-[12px] font-semibold cursor-pointer"
            title="Dọn dẹp các job đã tải xong"
            @click="clearFinishedJobs"
          >
            🗑️ Dọn Xong
          </button>
        </div>
      </div>

      <!-- Jobs Table -->
      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th class="w-16">Mã Job</th>
              <th>Liên Kết / Tên Tệp</th>
              <th>Nguồn</th>
              <th>Trạng Thái</th>
              <th>Tiến Độ</th>
              <th>Thời Gian</th>
              <th class="text-right">Thao Tác</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!filteredJobs.length">
              <td colspan="7" class="text-center text-gray-400 py-6 text-xs">
                Chưa có tác vụ tải xuống nào trong mục này. Dán URL ở khung phía trên để bắt đầu!
              </td>
            </tr>
            <tr v-for="j in filteredJobs" :key="j.id">
              <td class="mono font-bold text-sky-400">#{{ j.id }}</td>
              <td class="max-w-md">
                <div class="font-bold text-gray-100 truncate" :title="j.url">{{ j.url }}</div>
                <div v-if="j.error" class="text-xs text-red-400 truncate mt-0.5" :title="j.error">
                  ⚠️ {{ j.error }}
                </div>
                <div v-else-if="j.outputPath" class="text-xs text-gray-400 truncate font-mono mt-0.5">
                  📁 {{ j.outputPath }}
                </div>
              </td>
              <td>
                <span class="badge" :class="j.source === 'telegram' ? 'badge-blue' : (j.source === 'ai' ? 'badge-purple' : 'badge-gray')">
                  {{ j.source === 'telegram' ? '✈️ Telegram' : (j.source === 'ai' ? '🤖 Root AI' : '🌐 Web Dashboard') }}
                </span>
              </td>
              <td>
                <span class="badge" :class="statusBadgeClass(j.status)">
                  {{ statusLabel(j.status) }}
                </span>
              </td>
              <td>
                <div class="w-28 space-y-1">
                  <div class="flex items-center justify-between text-[11px] font-mono">
                    <span class="font-bold text-gray-200">{{ (j.progress || 0).toFixed(0) }}%</span>
                    <span v-if="j.speed" class="text-emerald-400">{{ j.speed }}</span>
                  </div>
                  <div class="bar-track">
                    <div
                      class="bar-fill"
                      :class="j.status === 'COMPLETED' ? 'bg-emerald-400' : (j.status === 'FAILED' ? 'bg-red-400' : 'bg-sky-400')"
                      :style="{ width: `${Math.max(3, Math.min(100, j.progress || 0))}%` }"
                    ></div>
                  </div>
                </div>
              </td>
              <td class="text-xs mono text-gray-400 font-medium">
                {{ j.createdAt ? j.createdAt.replace('T', ' ').slice(5, 19) : '—' }}
              </td>
              <td class="text-right space-x-1.5 whitespace-nowrap">
                <button
                  v-if="j.status === 'FAILED' || j.status.startsWith('ERR')"
                  type="button"
                  class="btn-secondary text-xs py-1 px-2 font-semibold cursor-pointer"
                  title="Thử tải lại link này"
                  @click="retryJob(j)"
                >
                  🔄 Thử lại
                </button>
                <button
                  type="button"
                  class="btn-danger text-xs py-1 px-2 font-semibold cursor-pointer"
                  title="Xóa job này khỏi hàng đợi"
                  @click="deleteJob(j.id)"
                >
                  ✕ Xóa
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Media Library Card: Downloaded Files in USB -->
    <div class="pro-card p-4 space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-2 pb-2 border-b border-white/10">
        <div class="flex items-center gap-2">
          <span class="pro-card-title">📁 Thư Viện Tệp Đã Tải Về Trên Ổ Cứng USB 3.0</span>
          <span class="badge badge-green font-mono font-bold">{{ downloadedFiles.length }} Tệp</span>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="btn-secondary text-xs py-1 px-2.5 font-semibold flex items-center gap-1 cursor-pointer"
            @click="showEditOutputDir = !showEditOutputDir"
          >
            ⚙️ {{ showEditOutputDir ? 'Đóng Cài Đặt Thư Mục' : 'Đổi Thư Mục Lưu Trữ' }}
          </button>
          <RouterLink to="/files" class="text-xs text-sky-400 hover:underline font-semibold">
            Mở Trình Duyệt Tệp Cây →
          </RouterLink>
          <button type="button" class="btn-secondary text-xs py-1 px-2.5" @click="fetchDownloadedFiles">↻</button>
        </div>
      </div>

      <!-- Output Directory Settings Panel -->
      <div v-if="showEditOutputDir" class="p-3 rounded-lg bg-[#0d1627] border border-sky-500/30 space-y-2 text-xs">
        <div class="font-bold text-sky-300 flex items-center justify-between">
          <span>⚙️ Cấu Hình Thư Mục Lưu Trữ Video Tải Về</span>
          <span class="text-gray-400 font-mono text-[11px]">Đường dẫn tuyệt đối trên R3P</span>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <input
            v-model="outputDir"
            type="text"
            class="pro-input mono flex-1 min-w-[280px]"
            placeholder="/mnt/sda1/downloads"
          />
          <button
            type="button"
            class="btn-primary text-xs py-1.5 px-3 font-bold"
            :disabled="savingOutputDir"
            @click="saveOutputDir"
          >
            {{ savingOutputDir ? 'Đang lưu…' : '✓ Lưu Thư Mục Mới' }}
          </button>
        </div>
        <p class="text-[11px] text-gray-400">
          Khuyên dùng các đường dẫn nằm trên ổ cứng USB 3.0 (ví dụ: <code class="mono text-sky-300">/mnt/sda1/downloads</code> hoặc <code class="mono text-sky-300">/mnt/sda1/videos</code>) để tránh đầy bộ nhớ Flash nội bộ NAND.
        </p>
      </div>

      <!-- Current Directory Indicator -->
      <div class="flex items-center justify-between text-xs text-gray-400 bg-black/30 px-3 py-1.5 rounded border border-white/5 mono">
        <div class="truncate">
          <span>Thư mục hiện tại: </span>
          <span class="text-emerald-400 font-bold select-all">{{ outputDir }}</span>
        </div>
        <span class="shrink-0 text-gray-500">Đã quét cả thư mục con (Youtube, TikTok, Facebook)</span>
      </div>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Nguồn</th>
              <th>Tên Tệp Tin & Thư Mục Con</th>
              <th>Định Dạng</th>
              <th>Dung Lượng</th>
              <th>Thời Gian Tải</th>
              <th class="text-right">Tải Xuống</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!downloadedFiles.length">
              <td colspan="6" class="text-center text-gray-400 py-6 text-xs">
                Chưa có file nào trong thư mục <code>{{ outputDir }}</code>. Gửi link vào Telegram bot hoặc dán URL ở trên để tải về!
              </td>
            </tr>
            <tr v-for="f in downloadedFiles" :key="f.sub_path">
              <td>
                <span
                  class="badge text-[10.5px] font-bold"
                  :class="f.category === 'YouTube' ? 'badge-red' : (f.category === 'TikTok' ? 'badge-blue' : (f.category === 'Facebook' ? 'badge-purple' : 'badge-gray'))"
                >
                  {{ f.category }}
                </span>
              </td>
              <td class="font-bold text-gray-100">
                <div class="flex items-center gap-1.5">
                  <span>{{ f.name.endsWith('.mp3') ? '🎵' : '🎬' }}</span>
                  <span class="truncate max-w-sm" :title="f.name">{{ f.name }}</span>
                </div>
                <div class="text-[11px] text-gray-400 mono font-normal truncate mt-0.5">
                  📁 {{ f.sub_path }}
                </div>
              </td>
              <td class="mono font-bold text-sky-400 uppercase text-xs">
                {{ f.name.split('.').pop() || 'FILE' }}
              </td>
              <td class="mono font-semibold text-emerald-400 text-xs">
                {{ formatBytes(f.size) }}
              </td>
              <td class="mono text-gray-400 text-xs font-medium">
                {{ f.mod_time ? f.mod_time.slice(0, 19).replace('T', ' ') : '—' }}
              </td>
              <td class="text-right">
                <a
                  :href="`/api/v1/download/file?sub_path=${encodeURIComponent(f.sub_path)}`"
                  download
                  class="btn-primary text-xs py-1 px-2.5 font-semibold inline-flex items-center gap-1"
                >
                  ⬇️ Tải Về Máy
                </a>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Telegram Bot Automation Instructions Card -->
    <div class="pro-card p-4 bg-gradient-to-r from-blue-950/20 to-sky-950/20 border border-sky-500/20 space-y-2.5">
      <div class="flex items-center gap-2 text-sky-400 font-bold text-sm">
        <span>✈️</span>
        <span>Tự Động Phân Tích & Điều Hướng Tin Nhắn Telegram (Downloader + Root AI)</span>
      </div>
      <p class="text-xs text-gray-300 leading-relaxed font-medium">
        Mở <strong>Telegram</strong> và nhắn cho Bot bất cứ lúc nào: nếu tin nhắn <strong>chỉ chứa URL</strong> (TikTok, YouTube, Facebook, Reels, Shorts...), hệ thống tự chuyển cho module <code>yt-dlp</code> tải về ổ cứng USB; nếu là <strong>tin nhắn thông thường</strong>, câu hỏi (kể cả có nhắc link) hoặc lệnh <code>!&lt;lệnh&gt;</code>, hệ thống tự chuyển cho <strong>Trợ Lý AI Root R3P</strong> xử lý và phản hồi!
      </p>
      <div class="flex flex-wrap items-center gap-4 text-xs text-gray-400 pt-1 mono">
        <div>
          Bot Token:
          <span class="text-emerald-400 font-bold">
            {{ debugResult?.telegram_token_set ? ((debugResult?.telegram_bot_username ? '@' + debugResult.telegram_bot_username + ' · ' : '') + '****' + (debugResult?.telegram_token_last4 || '')) : 'Chưa cấu hình' }}
          </span>
        </div>
        <div>Chat ID Cấp Quyền: <span class="text-gray-200 font-bold">{{ debugResult?.telegram_chat_id || 'Chưa cấu hình' }}</span></div>
        <div>Thư mục đích: <span class="text-purple-300 font-bold">{{ outputDir }}</span></div>
      </div>
    </div>
  </div>
</template>
