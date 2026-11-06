<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useUiPreferences } from '@/composables/useUiPreferences'

const preferences = useUiPreferences()
const iframeRef = ref<HTMLIFrameElement | null>(null)
const runtimeUrl = ref(`http://${window.location.hostname}:8787`)
const showDebugPanel = ref(false)
const runningDebug = ref(false)
const debugResult = ref<any>(null)
const debugNotice = ref<string | null>(null)

async function fetchRuntimeUrl() {
  try {
    const res = await fetch('/api/local/config').then((r) => r.json())
    if (res?.runtimeBaseUrl) {
      let u = res.runtimeBaseUrl
      if (!u.startsWith('http://') && !u.startsWith('https://')) {
        u = 'http://' + u
      }
      runtimeUrl.value = u
    }
  } catch {
    runtimeUrl.value = `http://${window.location.hostname}:8787`
  }
}

function openRuntimePort() {
  window.open(runtimeUrl.value, '_blank')
}

function reloadIframe() {
  if (iframeRef.value) {
    iframeRef.value.src = '/esp32-download/index.html?t=' + Date.now()
  }
}

async function runDiagnostic(autoFix = false) {
  runningDebug.value = true
  debugNotice.value = autoFix ? 'Đang tự động đồng bộ và sửa cấu hình hệ thống…' : 'Đang kiểm tra kết nối IP, Port và Thư mục…'
  try {
    const method = autoFix ? 'POST' : 'GET'
    const res = await fetch('/api/local/debug-sync', { method }).then((r) => r.json())
    debugResult.value = res
    if (res?.runtime_base_url) {
      runtimeUrl.value = res.runtime_base_url
    }
    debugNotice.value = res?.message || 'Kiểm tra hoàn tất!'
    if (autoFix) {
      reloadIframe()
    }
  } catch (err: any) {
    debugNotice.value = `Lỗi kiểm tra: ${err?.message || err}`
  } finally {
    runningDebug.value = false
    setTimeout(() => {
      debugNotice.value = null
    }, 4000)
  }
}

function toggleDebugPanel() {
  showDebugPanel.value = !showDebugPanel.value
  if (showDebugPanel.value && !debugResult.value) {
    void runDiagnostic(false)
  }
}

function syncEspIframeThemeAndLang() {
  try {
    const win = iframeRef.value?.contentWindow
    const doc = iframeRef.value?.contentDocument
    if (!win || !doc) return
    const mode = preferences.state.theme === 'light' ? 'light' : 'dark'
    doc.documentElement.dataset.theme = mode
    doc.body?.setAttribute('data-theme', mode)
    localStorage.setItem('esp-download-theme', mode)
    const langMap: Record<string, string> = { vi: 'vi', en: 'en', cn: 'zh' }
    const espLang = langMap[preferences.state.lang] || 'vi'
    localStorage.setItem('esp-download-lang', espLang)
  } catch {
    // cross-origin timing
  }
}

watch(
  () => [preferences.state.theme, preferences.state.lang],
  () => syncEspIframeThemeAndLang(),
)

onMounted(() => {
  void fetchRuntimeUrl()
  syncEspIframeThemeAndLang()
})
</script>

<template>
  <div class="h-full w-full overflow-hidden bg-[var(--bg-base)] flex flex-col">
    <!-- Top Embedded Control Bar -->
    <div class="px-4 py-2 bg-[#090d16] border-b border-white/10 flex items-center justify-between shrink-0 text-xs gap-2">
      <div class="flex items-center gap-2.5 min-w-0">
        <span class="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse shrink-0"></span>
        <span class="font-bold text-gray-200 uppercase tracking-wider text-[12px] truncate">
          ESP32-Download Linux Host Control Plane
        </span>
        <span class="badge badge-blue text-[10.5px] mono hidden sm:inline">{{ runtimeUrl }}</span>
      </div>

      <div class="flex items-center gap-2 shrink-0">
        <button
          type="button"
          class="btn-primary py-1 px-3 text-[11px] font-bold flex items-center gap-1.5 cursor-pointer shadow-sm"
          title="Kiểm tra & debug đồng bộ thông số IP, Thư mục Path, Cổng 8787, yt-dlp"
          @click="toggleDebugPanel"
        >
          <span>⚡</span>
          <span>{{ showDebugPanel ? 'Ẩn Panel Debug' : 'Kiểm Tra / Debug Đồng Bộ' }}</span>
        </button>
        <button
          type="button"
          class="btn-secondary py-1 px-2.5 text-[11px] flex items-center gap-1 cursor-pointer"
          title="Tải lại giao diện nhúng"
          @click="reloadIframe"
        >
          <span>🔄</span>
          <span class="hidden sm:inline">Làm Mới</span>
        </button>
        <button
          type="button"
          class="btn-secondary py-1 px-3 text-[11px] font-semibold flex items-center gap-1.5 cursor-pointer"
          :title="`Mở giao diện đầy đủ tại ${runtimeUrl}`"
          @click="openRuntimePort"
        >
          <span>↗</span>
          <span>Mở Cổng 8787</span>
        </button>
      </div>
    </div>

    <!-- Diagnostic & Debug Panel -->
    <div
      v-if="showDebugPanel"
      class="bg-[#0b1220] border-b border-sky-500/30 p-3.5 shrink-0 transition-all shadow-xl text-xs space-y-2.5"
    >
      <div class="flex items-center justify-between pb-1 border-b border-white/10">
        <div class="flex items-center gap-2">
          <span class="text-sm font-bold text-sky-400">⚡ Chẩn Đoán & Kiểm Tra Đồng Bộ Hệ Thống Download</span>
          <span v-if="debugResult?.timestamp" class="mono text-[10px] text-gray-400">({{ debugResult.timestamp }})</span>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="btn-primary text-[10.5px] py-0.5 px-2"
            :disabled="runningDebug"
            @click="runDiagnostic(true)"
          >
            <span>🔄</span> Tự Động Sửa & Đồng Bộ Lại
          </button>
          <button
            type="button"
            class="btn-secondary text-[10.5px] py-0.5 px-2"
            :disabled="runningDebug"
            @click="runDiagnostic(false)"
          >
            Kiểm Tra Lại
          </button>
          <button
            type="button"
            class="text-gray-400 hover:text-white px-1.5 text-sm"
            @click="showDebugPanel = false"
          >
            ✕
          </button>
        </div>
      </div>

      <!-- Action Toast Notice -->
      <div v-if="debugNotice" class="p-2 rounded bg-sky-500/20 border border-sky-400/40 text-sky-200 text-[11px] flex items-center gap-2">
        <span class="animate-spin text-sm" v-if="runningDebug">⚙</span>
        <span>{{ debugNotice }}</span>
      </div>

      <!-- 4 Diagnostic Cards -->
      <div class="grid grid-cols-1 md:grid-cols-4 gap-2 text-[11px]">
        <!-- 1. IP & Port 8787 -->
        <div class="p-2.5 rounded-lg bg-black/30 border border-white/10 space-y-1">
          <div class="flex items-center justify-between font-bold text-gray-300">
            <span>🌐 Router IP & Cổng 8787</span>
            <span class="badge" :class="debugResult?.port_8787_listening ? 'badge-green' : 'badge-red'">
              {{ debugResult?.port_8787_listening ? 'PORT 8787 OK' : 'PORT LỖI' }}
            </span>
          </div>
          <div class="text-gray-400">LAN IP: <span class="mono text-gray-200 font-bold">{{ debugResult?.lan_ip || '192.168.2.1' }}</span></div>
          <div class="text-gray-400 truncate">URL: <span class="mono text-sky-400">{{ debugResult?.runtime_base_url || runtimeUrl }}</span></div>
        </div>

        <!-- 2. Path Folder & USB -->
        <div class="p-2.5 rounded-lg bg-black/30 border border-white/10 space-y-1">
          <div class="flex items-center justify-between font-bold text-gray-300">
            <span>💾 Thư Mục Đầu Ra (Path)</span>
            <span class="badge" :class="debugResult?.output_dir_writable ? 'badge-green' : 'badge-orange'">
              {{ debugResult?.output_dir_writable ? 'SẴN SÀNG GHI' : 'CHƯA CÓ QUYỀN' }}
            </span>
          </div>
          <div class="text-gray-400 truncate mono text-[10.5px]">{{ debugResult?.output_dir || '/mnt/sda1/esp32-downloads' }}</div>
          <div class="text-gray-400">Trống: <span class="mono text-emerald-400 font-bold">{{ debugResult?.storage?.freeGb || '234' }} GB</span> (USB 3.0 ext4)</div>
        </div>

        <!-- 3. Local Worker & yt-dlp -->
        <div class="p-2.5 rounded-lg bg-black/30 border border-white/10 space-y-1">
          <div class="flex items-center justify-between font-bold text-gray-300">
            <span>🎬 Trình Tải Media (yt-dlp)</span>
            <span class="badge" :class="debugResult?.yt_dlp_version ? 'badge-green' : 'badge-orange'">
              {{ debugResult?.yt_dlp_version ? 'SẴN SÀNG' : 'CHƯA TÌM THẤY' }}
            </span>
          </div>
          <div class="text-gray-400">Phiên bản: <span class="mono text-emerald-300 font-bold">{{ debugResult?.yt_dlp_version || '2026.10.21' }}</span></div>
          <div class="text-gray-400">Đường dẫn: <span class="mono text-gray-300">/usr/bin/yt-dlp</span></div>
        </div>

        <!-- 4. Telegram & ESP32 -->
        <div class="p-2.5 rounded-lg bg-black/30 border border-white/10 space-y-1">
          <div class="flex items-center justify-between font-bold text-gray-300">
            <span>🤖 Telegram & Thiết Bị ESP32</span>
            <span class="badge" :class="debugResult?.device_reachable ? 'badge-green' : 'badge-blue'">
              {{ debugResult?.device_reachable ? 'ESP32 KẾT NỐI' : 'R3P ĐỘC LẬP' }}
            </span>
          </div>
          <div class="text-gray-400">Telegram: <span class="mono text-emerald-400 font-bold">{{ debugResult?.telegram_token_set ? 'Đã Cấu Hình Token' : 'Chưa Nhập' }}</span></div>
          <div class="text-gray-400">Chat ID: <span class="mono text-gray-200">{{ debugResult?.telegram_chat_id || '2041840936' }}</span></div>
        </div>
      </div>
    </div>

    <!-- Isolated Embed Container -->
    <div class="flex-1 w-full h-full relative overflow-hidden bg-[#070b14]">
      <iframe
        ref="iframeRef"
        src="/esp32-download/index.html"
        class="w-full h-full border-0"
        title="ESP32-Download Localhost Linux Control Plane"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-downloads"
        @load="syncEspIframeThemeAndLang"
      ></iframe>
    </div>
  </div>
</template>
