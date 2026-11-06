<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'

const selectedConfig = ref('network')
const configFiles = ref<string[]>(['network', 'wireless', 'system', 'dropbear', 'dhcp', 'firewall', 'uhttpd', 'ubihealthd'])
const activeText = ref('')
const loading = ref(false)
const saving = ref(false)
const notice = ref<string | null>(null)

async function loadConfig(cfg: string) {
  selectedConfig.value = cfg
  loading.value = true
  notice.value = null
  try {
    const res = await apiRequest<{
      file: string
      content: string
      available: string[]
    }>(`/config?file=${encodeURIComponent(cfg)}`)
    if (res) {
      activeText.value = res.content || ''
      if (res.available?.length) {
        configFiles.value = res.available
      }
    }
  } catch (err: any) {
    notice.value = 'Error reading config: ' + (err?.message || err)
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  if (!activeText.value.trim() || saving.value) return
  saving.value = true
  notice.value = null
  try {
    const res = await apiRequest<{ file: string; message: string }>('/config', {
      method: 'PUT',
      body: JSON.stringify({
        file: selectedConfig.value,
        content: activeText.value,
      }),
    })
    notice.value = res.message || `Saved /etc/config/${selectedConfig.value}!`
    setTimeout(() => { notice.value = null }, 3500)
  } catch (err: any) {
    notice.value = 'Save failed: ' + (err?.message || err)
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void loadConfig('network')
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- UCI Configuration Editor -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Live UCI Configuration Editor (/etc/config/{{ selectedConfig }})</span>
        <div class="flex items-center gap-2">
          <span v-if="notice" class="text-emerald-400 text-[11.5px] font-medium">{{ notice }}</span>
          <button class="btn-secondary text-[11px]" :disabled="loading" @click="loadConfig(selectedConfig)">
            {{ loading ? 'Loading…' : 'Reload File ↻' }}
          </button>
          <button class="btn-primary text-[11px]" :disabled="saving || loading" @click="saveConfig">
            {{ saving ? 'Committing…' : 'Save & Commit (uci commit)' }}
          </button>
        </div>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-3">
        <!-- Config Files Selector -->
        <div class="space-y-1">
          <div class="text-[10px] font-bold text-gray-500 uppercase tracking-wider mb-2">Router /etc/config/ Files</div>
          <button
            v-for="cfg in configFiles"
            :key="cfg"
            class="w-full text-left px-2.5 py-1.5 rounded text-[12px] font-mono transition flex items-center justify-between"
            :class="selectedConfig === cfg ? 'bg-sky-500/20 text-sky-300 border border-sky-500/30' : 'text-gray-400 hover:text-gray-200 hover:bg-white/5'"
            @click="loadConfig(cfg)"
          >
            <span>/etc/config/{{ cfg }}</span>
            <span class="text-[10px] text-gray-500">→</span>
          </button>
        </div>

        <!-- Raw Config Editor -->
        <div class="md:col-span-3 flex flex-col space-y-2">
          <div class="flex items-center justify-between text-[11px] text-gray-400">
            <span>Live File on Router: <code class="mono text-sky-400">/etc/config/{{ selectedConfig }}</code></span>
            <span>Auto-backup: <code class="mono text-gray-500">/etc/r3p/backups/{{ selectedConfig }}.bak</code></span>
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
