<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useUiPreferences } from '@/composables/useUiPreferences'

const preferences = useUiPreferences()
const iframeRef = ref<HTMLIFrameElement | null>(null)

function syncEspIframeThemeAndLang() {
  try {
    const win = iframeRef.value?.contentWindow
    const doc = iframeRef.value?.contentDocument
    if (!win || !doc) return
    const mode = preferences.state.theme === 'light' ? 'light' : 'dark'
    doc.documentElement.dataset.theme = mode
    doc.body?.setAttribute('data-theme', mode)
    localStorage.setItem('esp_download_theme', mode)
    const langMap: Record<string, string> = { vi: 'vi', en: 'en', cn: 'zh' }
    const espLang = langMap[preferences.state.lang] || 'vi'
    localStorage.setItem('esp_download_lang', espLang)
  } catch {
    // ignore cross-frame timing
  }
}

watch(
  () => [preferences.state.theme, preferences.state.lang],
  () => syncEspIframeThemeAndLang(),
)

onMounted(() => {
  syncEspIframeThemeAndLang()
})
</script>

<template>
  <div class="h-full w-full overflow-hidden bg-[var(--bg-base)] flex flex-col">
    <iframe
      ref="iframeRef"
      src="/esp32-download/"
      class="w-full flex-1 border-0"
      title="ESP32-Download Localhost Linux Control Plane"
      @load="syncEspIframeThemeAndLang"
    ></iframe>
  </div>
</template>
