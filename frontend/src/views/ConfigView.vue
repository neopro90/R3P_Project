<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'
import { useUiPreferences } from '@/composables/useUiPreferences'

const { t } = useUiPreferences()

interface ConfigBackupItem {
  file: string
  size: number
  updated_at: string
}

const selectedConfig = ref('network')
const configFiles = ref<string[]>(['network', 'wireless', 'system', 'dropbear', 'dhcp', 'firewall', 'uhttpd', 'ubihealthd'])
const activeText = ref('')
const reloadAfterSave = ref(false)
const hasBackup = ref(false)
const backups = ref<ConfigBackupItem[]>([])
const restoringFile = ref<string | null>(null)
const loading = ref(false)
const saving = ref(false)
const notice = ref<string | null>(null)
let noticeTimer: ReturnType<typeof setTimeout> | null = null
function showNotice(msg: string, ms = 4000) {
  notice.value = msg
  if (noticeTimer) clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => { notice.value = null }, ms)
}

async function loadBackups() {
  try {
    const res = await apiRequest<{ backups: ConfigBackupItem[] }>('/config/backups')
    backups.value = res?.backups || []
  } catch {
    // non-fatal
  }
}

async function loadConfig(cfg: string) {
  selectedConfig.value = cfg
  loading.value = true
  try {
    const res = await apiRequest<{
      file: string
      content: string
      available: string[]
      has_backup?: boolean
    }>(`/config?file=${encodeURIComponent(cfg)}`)
    if (res) {
      activeText.value = res.content || ''
      hasBackup.value = Boolean(res.has_backup)
      if (res.available?.length) {
        configFiles.value = res.available
      }
    }
  } catch (err: any) {
    showNotice('Error reading config: ' + (err?.message || err), 5000)
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  if (!activeText.value.trim() || saving.value) return
  saving.value = true
  try {
    const res = await apiRequest<{ file: string; reloaded?: boolean; message: string }>('/config', {
      method: 'PUT',
      body: JSON.stringify({
        file: selectedConfig.value,
        content: activeText.value,
        reload: reloadAfterSave.value,
      }),
    })
    const reloadSuffix = res.reloaded ? ' (service reloaded)' : ''
    showNotice((res.message || `Saved /etc/config/${selectedConfig.value}!`) + reloadSuffix)
    await Promise.all([loadConfig(selectedConfig.value), loadBackups()])
  } catch (err: any) {
    showNotice('Save failed: ' + (err?.message || err), 5000)
  } finally {
    saving.value = false
  }
}

async function restoreBackup(file: string) {
  if (!confirm(`Khôi phục /etc/config/${file} từ bản sao lưu .bak gần nhất?`)) return
  restoringFile.value = file
  try {
    const res = await apiRequest<{ file: string; reloaded?: boolean; content?: string; message: string }>('/config/restore', {
      method: 'POST',
      body: JSON.stringify({ file, reload: reloadAfterSave.value }),
    })
    showNotice(res.message || `Đã khôi phục /etc/config/${file}!`)
    if (selectedConfig.value === file && typeof res.content === 'string') {
      activeText.value = res.content
    }
    await loadBackups()
  } catch (err: any) {
    showNotice('Error restoring backup: ' + (err?.message || err), 5000)
  } finally {
    restoringFile.value = null
  }
}

onMounted(() => {
  void Promise.all([loadConfig('network'), loadBackups()])
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Floating Toast Notification (Never shifts layout) -->
    <div
      v-if="notice"
      class="r3p-toast"
      :class="notice.startsWith('Error') || notice.startsWith('Save failed') ? 'r3p-toast-error' : 'r3p-toast-success'"
    >
      <div class="flex items-start justify-between gap-3">
        <span class="r3p-toast-text">{{ notice }}</span>
        <button type="button" class="hover:text-white shrink-0 font-bold" @click="notice = null">✕</button>
      </div>
    </div>

    <!-- UCI Configuration Editor -->
    <div class="pro-card">
      <div class="pro-card-header flex-wrap gap-2">
        <span class="pro-card-title">{{ t('cfg_editor_title') }} (/etc/config/{{ selectedConfig }})</span>
        <div class="flex flex-wrap items-center gap-2">
          <label class="flex items-center gap-1.5 text-[11px] text-gray-300 cursor-pointer select-none px-2 py-1 rounded bg-black/25 border border-white/10">
            <input v-model="reloadAfterSave" type="checkbox" class="accent-sky-500" />
            <span>{{ t('cfg_auto_reload') }}</span>
          </label>
          <button
            v-if="hasBackup"
            type="button"
            class="btn-secondary text-[11px]"
            :disabled="restoringFile === selectedConfig || loading"
            @click="restoreBackup(selectedConfig)"
          >
            {{ restoringFile === selectedConfig ? 'Restoring…' : '↩ Restore .bak' }}
          </button>
          <button type="button" class="btn-secondary text-[11px] min-w-[104px] justify-center" :disabled="loading" @click="loadConfig(selectedConfig)">
            {{ loading ? 'Loading…' : 'Reload File ↻' }}
          </button>
          <button type="button" class="btn-primary text-[11px] min-w-[196px] justify-center" :disabled="saving || loading" @click="saveConfig">
            {{ saving ? 'Committing…' : 'Save & Commit (uci commit)' }}
          </button>
        </div>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-3 items-start">
        <!-- Config Files Selector -->
        <div class="space-y-1">
          <div class="text-[10px] font-bold text-gray-500 uppercase tracking-wider mb-2">Router /etc/config/ Files</div>
          <button
            v-for="cfg in configFiles"
            :key="cfg"
            type="button"
            class="w-full text-left px-2.5 py-1.5 rounded text-[12px] font-mono transition flex items-center justify-between cursor-pointer"
            :class="selectedConfig === cfg ? 'bg-sky-500/20 text-sky-300 border border-sky-500/30' : 'text-gray-400 hover:text-gray-200 hover:bg-white/5 border border-transparent'"
            @click="loadConfig(cfg)"
          >
            <span class="truncate">/etc/config/{{ cfg }}</span>
            <span class="text-[10px] text-gray-500 shrink-0">→</span>
          </button>

          <!-- UCI Auto-Backups List -->
          <div v-if="backups.length" class="pt-3 mt-3 border-t border-white/10 space-y-1.5">
            <div class="text-[10px] font-bold text-emerald-400 uppercase tracking-wider">Bản Sao Lưu (.bak)</div>
            <div
              v-for="b in backups"
              :key="b.file"
              class="flex items-center justify-between gap-2 p-2 rounded bg-black/25 border border-white/10 text-[11px]"
            >
              <div class="min-w-0">
                <div class="mono font-semibold text-gray-200 truncate">{{ b.file }}.bak</div>
                <div class="text-[10px] text-gray-400 truncate">{{ b.size }} B · {{ b.updated_at }}</div>
              </div>
              <button
                type="button"
                class="btn-secondary text-[10px] py-0.5 px-2 shrink-0"
                :disabled="restoringFile === b.file"
                @click="restoreBackup(b.file)"
              >
                ↩
              </button>
            </div>
          </div>
        </div>

        <!-- Raw Config Editor -->
        <div class="md:col-span-3 flex flex-col space-y-2 min-w-0">
          <div class="flex items-center justify-between text-[11px] text-gray-400 gap-2">
            <span class="truncate">Live File on Router: <code class="mono text-sky-400">/etc/config/{{ selectedConfig }}</code></span>
            <span class="truncate">Auto-backup: <code class="mono text-gray-500">/etc/r3p/backups/{{ selectedConfig }}.bak</code></span>
          </div>

          <textarea
            v-model="activeText"
            rows="20"
            spellcheck="false"
            class="w-full bg-black/60 rounded border border-white/10 p-2.5 mono text-[11.5px] text-gray-200 outline-none focus:border-sky-500 leading-relaxed resize-y"
          ></textarea>
        </div>
      </div>
    </div>
  </div>
</template>
