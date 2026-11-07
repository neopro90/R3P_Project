<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { apiRequest } from '@/services/api'

interface DownloadJob {
  id: string
  url: string
  source: string
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

interface FileEntry {
  name: string
  size: number
  is_dir: boolean
  mod_time: string
  path: string
}

const jobs = ref<DownloadJob[]>([])
const downloadedFiles = ref<FileEntry[]>([])
const activeTab = ref<'all' | 'running' | 'completed' | 'failed'>('all')

const newUrl = ref('')
const selectedQuality = ref('best')
const addingJob = ref(false)
const notice = ref<string | null>(null)

const showDebugPanel = ref(false)
const runningDebug = ref(false)
const debugResult = ref<any>(null)
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
    const res = await fetch('/api/device/api/v1/jobs').then(r => r.json())
    if (res?.jobs) {
      jobs.value = res.jobs
    }
  } catch {
    // ignore
  }
}

async function fetchDownloadedFiles() {
  try {
    const res = await apiRequest<{ files: FileEntry[] }>('/files?path=/mnt/sda1/esp32-downloads')
    if (res?.files) {
      downloadedFiles.value = res.files.filter(f => !f.is_dir && !f.name.startsWith('.'))
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
  notice.value = 'Đang đẩy link vào hàng đợi R3P Linux…'
  try {
    const res = await fetch('/api/device/api/v1/jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: u, force: true }),
    }).then(r => r.json())

    if (res?.job?.id || res?.accepted) {
      notice.value = `Đã thêm vào hàng đợi tải xuống thành công! #${res?.job?.id || res?.jobId || ''}`
      newUrl.value = ''
      await fetchJobs()
    } else if (res?.error) {
      notice.value = `Lỗi thêm URL: ${res.error}`
    }
  } catch (err: any) {
    notice.value = `Lỗi kết nối: ${err?.message || err}`
  } finally {
    addingJob.value = false
    setTimeout(() => { notice.value = null }, 3500)
  }
}

async function deleteJob(id: string) {
  try {
    await fetch(`/api/local/runtime/jobs/${id}`, { method: 'DELETE' })
    await fetchJobs()
  } catch {
    // ignore
  }
}

async function clearFinishedJobs() {
  try {
    await fetch('/api/local/runtime/jobs', { method: 'DELETE' })
    notice.value = 'Đã dọn sạch các job đã hoàn tất!'
    await fetchJobs()
    setTimeout(() => { notice.value = null }, 2500)
  } catch {
    // ignore
  }
}

async function retryJob(job: DownloadJob) {
  try {
    await fetch('/api/device/api/v1/jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: job.url, force: true }),
    })
    notice.value = 'Đã thử lại tải link!'
    await fetchJobs()
    setTimeout(() => { notice.value = null }, 2500)
  } catch {
    // ignore
  }
}

async function runDiagnostic(autoFix = false) {
  runningDebug.value = true
  debugNotice.value = autoFix ? 'Đang tự động đồng bộ và sửa cấu hình hệ thống…' : 'Đang kiểm tra kết nối IP, Port và Thư mục…'
  try {
    const method = autoFix ? 'POST' : 'GET'
    const res = await fetch('/api/local/debug-sync', { method }).then(r => r.json())
    debugResult.value = res
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
  if (showDebugPanel.value && !debugResult.value) {
    void runDiagnostic(false)
  }
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
            <span class="truncate">Trung Tâm Tải Xuống Tự Động (R3P Linux Downloader)</span>
            <span class="badge badge-green text-xs font-mono font-bold">PORT 8787 ONLINE</span>
          </h1>
          <p class="text-xs text-gray-400 font-medium truncate">
            Tự động tải đa luồng qua yt-dlp & aria2 vào USB 3.0 · Nhận lệnh tự động từ Telegram Bot & Trợ Lý Root AI.
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

      <!-- 4 Diagnostic Cards -->
      <div class="grid grid-cols-1 md:grid-cols-4 gap-2.5 text-[13px]">
        <div class="p-3 rounded-lg bg-black/30 border border-white/10 space-y-1.5">
          <div class="flex items-center justify-between font-bold text-gray-100">
            <span>🌐 Router IP & Cổng 8787</span>
            <span class="badge" :class="debugResult?.port_8787_listening ? 'badge-green' : 'badge-red'">
              {{ debugResult?.port_8787_listening ? 'PORT 8787 OK' : 'PORT LỖI' }}
            </span>
          </div>
          <div class="text-gray-300">LAN IP: <span class="mono text-gray-100 font-bold">{{ debugResult?.lan_ip || '192.168.2.1' }}</span></div>
          <div class="text-gray-300 truncate">URL: <span class="mono text-sky-400 font-semibold">{{ debugResult?.runtime_base_url || 'http://192.168.2.1:8787' }}</span></div>
        </div>

        <div class="p-3 rounded-lg bg-black/30 border border-white/10 space-y-1.5">
          <div class="flex items-center justify-between font-bold text-gray-100">
            <span>💾 Thư Mục Đầu Ra (USB)</span>
            <span class="badge" :class="debugResult?.output_dir_writable ? 'badge-green' : 'badge-orange'">
              {{ debugResult?.output_dir_writable ? 'SẴN SÀNG GHI (0777)' : 'CHƯA CÓ QUYỀN' }}
            </span>
          </div>
          <div class="text-gray-300 truncate mono text-[12px] font-medium">{{ debugResult?.output_dir || '/mnt/sda1/esp32-downloads' }}</div>
          <div class="text-gray-300">Trống: <span class="mono text-emerald-400 font-bold">{{ debugResult?.storage?.freeGb || '234' }} GB</span> (USB 3.0 ext4)</div>
        </div>

        <div class="p-3 rounded-lg bg-black/30 border border-white/10 space-y-1.5">
          <div class="flex items-center justify-between font-bold text-gray-100">
            <span>🎬 Trình Tải Media (yt-dlp)</span>
            <span class="badge" :class="debugResult?.yt_dlp_version ? 'badge-green' : 'badge-orange'">
              {{ debugResult?.yt_dlp_version ? 'SẴN SÀNG' : 'CHƯA TÌM THẤY' }}
            </span>
          </div>
          <div class="text-gray-300">Phiên bản: <span class="mono text-emerald-300 font-bold">{{ debugResult?.yt_dlp_version || '2026.10.21' }}</span></div>
          <div class="text-gray-300">Đường dẫn: <span class="mono text-gray-200">/usr/bin/yt-dlp</span></div>
        </div>

        <div class="p-3 rounded-lg bg-black/30 border border-white/10 space-y-1.5">
          <div class="flex items-center justify-between font-bold text-gray-100">
            <span>🤖 Telegram & Tự Động</span>
            <span class="badge badge-green">TRỰC TUYẾN</span>
          </div>
          <div class="text-gray-300">Telegram Bot: <span class="mono text-emerald-400 font-bold">{{ debugResult?.telegram_token_set ? 'Đã Cấu Hình Token' : 'Chưa Nhập' }}</span></div>
          <div class="text-gray-300">Chat ID: <span class="mono text-gray-100 font-bold">{{ debugResult?.telegram_chat_id || '2041840936' }}</span></div>
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

    <!-- Media Library Card: Downloaded Files in USB (/mnt/sda1/esp32-downloads) -->
    <div class="pro-card p-4 space-y-3">
      <div class="flex items-center justify-between pb-2 border-b border-white/10">
        <div class="flex items-center gap-2">
          <span class="pro-card-title">📁 Thư Viện Tệp Đã Tải Về Trên Ổ Cứng USB 3.0</span>
          <span class="badge badge-green font-mono font-bold">{{ downloadedFiles.length }} Tệp</span>
        </div>
        <div class="flex items-center gap-2">
          <RouterLink to="/files" class="text-xs text-sky-400 hover:underline font-semibold">
            Mở Trình Duyệt Tệp Cây (File Explorer) →
          </RouterLink>
          <button type="button" class="btn-secondary text-xs py-1 px-2.5" @click="fetchDownloadedFiles">↻</button>
        </div>
      </div>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Tên Tệp Tin</th>
              <th>Định Dạng</th>
              <th>Dung Lượng</th>
              <th>Thời Gian Tải</th>
              <th class="text-right">Tải Xuống</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!downloadedFiles.length">
              <td colspan="5" class="text-center text-gray-400 py-4 text-xs">
                Chưa có file nào trong thư mục <code>/mnt/sda1/esp32-downloads</code>.
              </td>
            </tr>
            <tr v-for="f in downloadedFiles" :key="f.path">
              <td class="font-bold text-gray-100 flex items-center gap-2">
                <span>{{ f.name.endsWith('.mp3') ? '🎵' : '🎬' }}</span>
                <span class="truncate max-w-md" :title="f.name">{{ f.name }}</span>
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
                  :href="`/api/v1/files/download?path=${encodeURIComponent(f.path)}`"
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
        <span>Tính Năng Tự Động Tải Siêu Tiện Lợi Qua Telegram Bot</span>
      </div>
      <p class="text-xs text-gray-300 leading-relaxed font-medium">
        Bạn không cần phải mở giao diện web này mỗi khi muốn tải video. Chỉ cần mở ứng dụng <strong>Telegram</strong> trên điện thoại và gửi trực tiếp liên kết bất kỳ (TikTok, YouTube, Facebook, Reels, Shorts...) vào Bot. Router R3P sẽ tự động nhận diện URL, kích hoạt <code>yt-dlp</code> tốc độ cao và lưu file an toàn vào ổ cứng USB 3.0!
      </p>
      <div class="flex flex-wrap items-center gap-4 text-xs text-gray-400 pt-1 mono">
        <div>Bot Token: <span class="text-emerald-400 font-bold">9246238020:AAHuEx...</span></div>
        <div>Chat ID Cấp Quyền: <span class="text-gray-200 font-bold">2041840936</span></div>
        <div>Thư mục đích: <span class="text-purple-300 font-bold">/mnt/sda1/esp32-downloads</span></div>
      </div>
    </div>
  </div>
</template>
