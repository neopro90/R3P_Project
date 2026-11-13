<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'

interface PluginItem {
  id: string
  name: string
  version: string
  author: string
  description: string
  enabled: boolean
  command: string
  last_output: string
  last_run: string
}

const plugins = ref<PluginItem[]>([])
const loading = ref(false)
const runningId = ref<string | null>(null)
const notice = ref<string | null>(null)
let noticeTimer: ReturnType<typeof setTimeout> | null = null
function showNotice(msg: string, ms = 4000) {
  notice.value = msg
  if (noticeTimer) clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => { notice.value = null }, ms)
}
const selectedOutput = ref<{ name: string; output: string } | null>(null)

const pluginPresets = [
  { id: 'ram_flush', label: 'Flush Memory Cache (drop_caches + free -m)' },
  { id: 'lan_probe', label: 'Gateway & LAN ARP Probe (br-lan + ip neigh)' },
  { id: 'nand_health', label: 'NAND UBI Wear Inspector (/sys/class/ubi/ubi0)' },
  { id: 'dns_reload', label: 'Reload dnsmasq AdBlock Shield' },
  { id: 'disk_usage', label: 'Storage & Partition Check (df -h)' },
  { id: 'uptime_check', label: 'System Load & Uptime Check' },
]

const showCreate = ref(false)
const newPlugin = ref({ name: '', description: '', command: 'ram_flush' })

async function loadPlugins() {
  loading.value = true
  try {
    const res = await apiRequest<{ plugins: PluginItem[] }>('/plugins')
    if (res?.plugins) plugins.value = res.plugins
  } catch (err: any) {
    showNotice('Error loading plugins: ' + (err?.message || err), 5000)
  } finally {
    loading.value = false
  }
}

async function runPlugin(p: PluginItem) {
  runningId.value = p.id
  showNotice(`Running plugin '${p.name}'…`, 10000)
  try {
    const res = await apiRequest<{ plugins: PluginItem[] }>('/plugins', {
      method: 'POST',
      body: JSON.stringify({ action: 'run', id: p.id }),
    })
    if (res?.plugins) {
      plugins.value = res.plugins
      const updated = res.plugins.find((x) => x.id === p.id)
      if (updated) {
        selectedOutput.value = { name: updated.name, output: updated.last_output || 'Completed with no output.' }
      }
    }
    showNotice(`Plugin '${p.name}' executed successfully!`)
  } catch (err: any) {
    showNotice('Execution failed: ' + (err?.message || err), 5000)
  } finally {
    runningId.value = null
  }
}

async function togglePlugin(p: PluginItem) {
  try {
    const res = await apiRequest<{ plugins: PluginItem[] }>('/plugins', {
      method: 'POST',
      body: JSON.stringify({ action: 'toggle', id: p.id }),
    })
    if (res?.plugins) plugins.value = res.plugins
    showNotice(`Plugin '${p.name}' ${!p.enabled ? 'enabled' : 'disabled'}!`)
  } catch (err: any) {
    showNotice('Toggle error: ' + (err?.message || err), 5000)
  }
}

async function createPlugin() {
  if (!newPlugin.value.name || !newPlugin.value.command) return
  try {
    const res = await apiRequest<{ plugins: PluginItem[] }>('/plugins', {
      method: 'POST',
      body: JSON.stringify({
        action: 'create',
        name: newPlugin.value.name,
        description: newPlugin.value.description,
        command: newPlugin.value.command,
      }),
    })
    if (res?.plugins) plugins.value = res.plugins
    showCreate.value = false
    newPlugin.value = { name: '', description: '', command: 'ram_flush' }
    showNotice('Custom plugin created!')
  } catch (err: any) {
    showNotice('Create error: ' + (err?.message || err), 5000)
  }
}

async function deletePlugin(id: string) {
  try {
    const res = await apiRequest<{ plugins: PluginItem[] }>('/plugins', {
      method: 'POST',
      body: JSON.stringify({ action: 'delete', id }),
    })
    if (res?.plugins) plugins.value = res.plugins
    showNotice('Plugin removed!')
  } catch (err: any) {
    showNotice('Delete error: ' + (err?.message || err), 5000)
  }
}

onMounted(() => {
  void loadPlugins()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Floating Toast Notification (Never shifts layout) -->
    <div v-if="notice" class="r3p-toast r3p-toast-info">
      <div class="flex items-start justify-between gap-3">
        <span class="r3p-toast-text">{{ notice }}</span>
        <button type="button" class="text-sky-300 hover:text-white shrink-0 font-bold" @click="notice = null">✕</button>
      </div>
    </div>

    <!-- Plugins Manager Card -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Installed Plugins & Extensions ({{ plugins.length }})</span>
        <div class="flex items-center gap-2">
          <button type="button" class="btn-secondary text-[11px] py-1 min-w-[92px] justify-center" :disabled="loading" @click="loadPlugins">
            {{ loading ? 'Loading…' : 'Refresh ↻' }}
          </button>
          <button type="button" class="btn-primary text-[11px] py-1 min-w-[162px] justify-center" @click="showCreate = !showCreate">
            {{ showCreate ? 'Cancel' : '+ Create Custom Plugin' }}
          </button>
        </div>
      </div>

      <!-- Create Custom Plugin Form -->
      <form v-if="showCreate" class="mb-3 p-2.5 rounded bg-black/40 border border-sky-500/30 grid grid-cols-1 sm:grid-cols-4 gap-2 items-end" @submit.prevent="createPlugin">
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Plugin Name</label>
          <input v-model="newPlugin.name" type="text" required class="w-full pro-input" placeholder="e.g. Custom DDNS" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Description</label>
          <input v-model="newPlugin.description" type="text" class="w-full pro-input" placeholder="Short summary" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Preset Action Hook</label>
          <select v-model="newPlugin.command" required class="w-full pro-input mono">
            <option v-for="preset in pluginPresets" :key="preset.id" :value="preset.id">{{ preset.label }}</option>
          </select>
        </div>
        <button type="submit" class="btn-primary justify-center py-1.5">Install Plugin</button>
      </form>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Plugin Name & ID</th>
              <th>Version</th>
              <th>Status</th>
              <th>Description</th>
              <th>Last Executed</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in plugins" :key="p.id">
              <td>
                <div class="font-semibold text-gray-200 max-w-[180px] truncate" :title="p.name">{{ p.name }}</div>
                <div class="mono text-[10px] text-gray-500 max-w-[180px] truncate">{{ p.id }}</div>
              </td>
              <td class="mono text-sky-400">{{ p.version }}</td>
              <td>
                <span :class="p.enabled ? 'badge badge-green' : 'badge badge-gray'">
                  {{ p.enabled ? 'ENABLED' : 'DISABLED' }}
                </span>
              </td>
              <td class="text-gray-400 max-w-sm truncate" :title="p.command">{{ p.description }}</td>
              <td class="mono text-gray-400">{{ p.last_run || 'Never' }}</td>
              <td>
                <div class="flex items-center gap-1.5">
                  <button
                    type="button"
                    class="btn-primary text-[10.5px] py-0.5 px-2 min-w-[82px] justify-center"
                    :disabled="runningId === p.id || !p.enabled"
                    @click="runPlugin(p)"
                  >
                    {{ runningId === p.id ? 'Running…' : '▶ Run Now' }}
                  </button>
                  <button type="button" class="btn-secondary text-[10.5px] py-0.5 px-2 min-w-[64px] justify-center" @click="togglePlugin(p)">
                    {{ p.enabled ? 'Disable' : 'Enable' }}
                  </button>
                  <button type="button" class="btn-danger text-[10.5px] py-0.5 px-2 min-w-[64px] justify-center" @click="deletePlugin(p.id)">
                    Remove
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Plugin Execution Output Drawer -->
      <div v-if="selectedOutput" class="mt-3 pt-2 border-t border-white/10">
        <div class="flex items-center justify-between text-[11px] text-gray-300 mb-1">
          <span class="font-semibold">Output from: <code class="mono text-sky-400">{{ selectedOutput.name }}</code></span>
          <button type="button" class="text-gray-400 hover:text-white cursor-pointer" @click="selectedOutput = null">Close ×</button>
        </div>
        <pre class="bg-black/60 border border-white/10 rounded p-2.5 mono text-[11px] text-emerald-300 max-h-48 overflow-y-auto whitespace-pre-wrap">{{ selectedOutput.output }}</pre>
      </div>
    </div>
  </div>
</template>
