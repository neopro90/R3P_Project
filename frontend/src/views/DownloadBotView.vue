<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useUiPreferences } from '@/composables/useUiPreferences'

const preferences = useUiPreferences()
const iframeRef = ref<HTMLIFrameElement | null>(null)
const runtimeUrl = ref(`http://${window.location.hostname}:8787`)

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
    <div class="px-3 py-1.5 bg-[#090d16] border-b border-white/10 flex items-center justify-between shrink-0 text-xs">
      <div class="flex items-center gap-2 min-w-0">
        <span class="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
        <span class="font-bold text-gray-200 uppercase tracking-wider text-[11px] truncate">
          ESP32-Download Linux Host Control Plane
        </span>
        <span class="badge badge-blue text-[9.5px] mono hidden sm:inline">{{ runtimeUrl }}</span>
      </div>

      <div class="flex items-center gap-2 shrink-0">
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
          class="btn-primary py-1 px-3 text-[11px] font-bold flex items-center gap-1.5 cursor-pointer shadow-sm"
          :title="`Mở giao diện đầy đủ tại ${runtimeUrl}`"
          @click="openRuntimePort"
        >
          <span>⚡</span>
          <span>Mở Cổng 8787 (Tab Mới)</span>
        </button>
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
