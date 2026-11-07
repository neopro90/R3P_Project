<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'

interface CronEntry {
  index: number
  schedule: string
  command: string
  description: string
  enabled: boolean
}

const jobs = ref<CronEntry[]>([])
const loading = ref(false)
const notice = ref<string | null>(null)
let noticeTimer: ReturnType<typeof setTimeout> | null = null
function showNotice(msg: string, ms = 4000) {
  notice.value = msg
  if (noticeTimer) clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => { notice.value = null }, ms)
}
const runOutput = ref<string | null>(null)
const runningCmd = ref<string | null>(null)

const showAddCron = ref(false)
const newCron = ref({
  schedule: '0 4 * * *',
  command: 'sync; echo 3 > /proc/sys/vm/drop_caches',
  description: 'Flush memory cache',
})

const telegramToken = ref('')
const telegramChatId = ref('')
const telegramEnabled = ref(true)
const telegramConfigured = ref(false)
const telegramTesting = ref(false)

async function loadAutomation() {
  loading.value = true
  try {
    const res = await apiRequest<{
      jobs: CronEntry[]
      telegram: { configured: boolean; enabled: boolean; chat_id?: string }
    }>('/automation/cron')
    if (res) {
      jobs.value = res.jobs || []
      if (res.telegram) {
        telegramConfigured.value = res.telegram.configured
        telegramEnabled.value = res.telegram.enabled
        if (res.telegram.chat_id) telegramChatId.value = res.telegram.chat_id
      }
    }
  } catch (err: any) {
    showNotice('Error loading automation: ' + (err?.message || err), 5000)
  } finally {
    loading.value = false
  }
}

async function addCron() {
  if (!newCron.value.schedule || !newCron.value.command) return
  try {
    const res = await apiRequest<{ jobs: CronEntry[] }>('/automation/cron', {
      method: 'POST',
      body: JSON.stringify(newCron.value),
    })
    if (res?.jobs) jobs.value = res.jobs
    showAddCron.value = false
    showNotice('Scheduled task saved to /etc/crontabs/root!')
  } catch (err: any) {
    showNotice('Add cron error: ' + (err?.message || err), 5000)
  }
}

async function toggleCron(c: CronEntry, idx: number) {
  try {
    const res = await apiRequest<{ jobs: CronEntry[] }>('/automation/cron', {
      method: 'PATCH',
      body: JSON.stringify({ index: idx, enabled: !c.enabled }),
    })
    if (res?.jobs) jobs.value = res.jobs
    showNotice(`Scheduled job #${idx + 1} ${!c.enabled ? 'enabled' : 'disabled'}!`)
  } catch (err: any) {
    showNotice('Toggle error: ' + (err?.message || err), 5000)
  }
}

async function deleteCron(idx: number) {
  try {
    const res = await apiRequest<{ jobs: CronEntry[] }>(`/automation/cron?index=${idx}`, {
      method: 'DELETE',
    })
    if (res?.jobs) jobs.value = res.jobs
    showNotice('Cron job removed!')
  } catch (err: any) {
    showNotice('Delete error: ' + (err?.message || err), 5000)
  }
}

async function runNow(command: string) {
  runningCmd.value = command
  runOutput.value = `Running: ${command} …`
  try {
    const res = await apiRequest<{ command: string; output: string; error: string }>('/automation/run', {
      method: 'POST',
      body: JSON.stringify({ command }),
    })
    runOutput.value = `$ ${res.command}\n${res.output || '(Command completed with exit status 0)'}${res.error ? '\nError: ' + res.error : ''}`
  } catch (err: any) {
    runOutput.value = 'Execution failed: ' + (err?.message || err)
  } finally {
    runningCmd.value = null
  }
}

async function saveTelegram() {
  try {
    await apiRequest('/telegram', {
      method: 'PUT',
      body: JSON.stringify({
        bot_token: telegramToken.value.trim(),
        chat_id: telegramChatId.value.trim(),
        mount: '/overlay',
        path: 'downloads',
        enabled: telegramEnabled.value,
      }),
    })
    telegramConfigured.value = true
    showNotice('Telegram Bot configuration saved!')
  } catch (err: any) {
    showNotice('Telegram error: ' + (err?.message || err), 5000)
  }
}

async function testTelegram() {
  telegramTesting.value = true
  try {
    const res = await apiRequest<{ message: string }>('/telegram/test', {
      method: 'POST',
      body: JSON.stringify({
        bot_token: telegramToken.value.trim(),
        chat_id: telegramChatId.value.trim(),
      }),
    })
    showNotice(res.message || 'Test ping sent!')
  } catch (err: any) {
    showNotice('Telegram test error: ' + (err?.message || err), 5000)
  } finally {
    telegramTesting.value = false
  }
}

onMounted(() => {
  void loadAutomation()
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

    <!-- Automation Cron Jobs Table -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Scheduled Cron Jobs (/etc/crontabs/root)</span>
        <div class="flex items-center gap-2">
          <button type="button" class="btn-secondary text-[11px] py-1 min-w-[92px] justify-center" :disabled="loading" @click="loadAutomation">
            {{ loading ? 'Loading…' : 'Refresh ↻' }}
          </button>
          <button type="button" class="btn-primary text-[11px] py-1 min-w-[154px] justify-center" @click="showAddCron = !showAddCron">
            {{ showAddCron ? 'Cancel' : '+ Add Scheduled Task' }}
          </button>
        </div>
      </div>

      <!-- Add Cron Form -->
      <form v-if="showAddCron" class="mb-3 p-2.5 rounded bg-black/40 border border-sky-500/30 grid grid-cols-1 sm:grid-cols-4 gap-2 items-end" @submit.prevent="addCron">
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Cron Schedule (5 fields)</label>
          <input v-model="newCron.schedule" type="text" required class="w-full pro-input mono" placeholder="*/15 * * * *" />
        </div>
        <div class="sm:col-span-2">
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Shell Command</label>
          <input v-model="newCron.command" type="text" required class="w-full pro-input mono" placeholder="/usr/bin/r3p-task.sh" />
        </div>
        <div class="flex gap-2">
          <input v-model="newCron.description" type="text" class="w-full pro-input" placeholder="Description" />
          <button type="submit" class="btn-primary shrink-0 min-w-[68px] justify-center">Save</button>
        </div>
      </form>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Cron Expression</th>
              <th>Command Executable</th>
              <th>Description</th>
              <th>Status</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!jobs.length">
              <td colspan="5" class="text-center text-gray-500 py-3">No cron jobs configured in /etc/crontabs/root</td>
            </tr>
            <tr v-for="(c, idx) in jobs" :key="idx">
              <td class="mono font-semibold text-sky-400">{{ c.schedule }}</td>
              <td class="mono text-gray-200 max-w-sm truncate" :title="c.command">{{ c.command }}</td>
              <td class="text-gray-400 max-w-[220px] truncate" :title="c.description">{{ c.description }}</td>
              <td>
                <span :class="c.enabled ? 'badge badge-green' : 'badge badge-gray'">
                  {{ c.enabled ? 'ACTIVE' : 'DISABLED' }}
                </span>
              </td>
              <td>
                <div class="flex items-center gap-1.5">
                  <button
                    type="button"
                    class="btn-primary text-[10px] py-0.5 px-2 min-w-[82px] justify-center"
                    :disabled="runningCmd === c.command"
                    @click="runNow(c.command)"
                  >
                    {{ runningCmd === c.command ? 'Running…' : '▶ Run Now' }}
                  </button>
                  <button type="button" class="btn-secondary text-[10px] py-0.5 px-2 min-w-[64px] justify-center" @click="toggleCron(c, idx)">
                    {{ c.enabled ? 'Disable' : 'Enable' }}
                  </button>
                  <button type="button" class="btn-danger text-[10px] py-0.5 px-2 min-w-[58px] justify-center" @click="deleteCron(idx)">
                    Delete
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="runOutput" class="mt-3">
        <div class="flex items-center justify-between text-[10.5px] text-gray-400 mb-1">
          <span>Task Execution Output</span>
          <button type="button" class="text-gray-400 hover:text-white cursor-pointer" @click="runOutput = null">Clear ×</button>
        </div>
        <pre class="bg-black/60 border border-white/10 rounded p-2 mono text-[11px] text-emerald-300 overflow-x-auto whitespace-pre-wrap">{{ runOutput }}</pre>
      </div>
    </div>

    <!-- Integrations Grid: Telegram Bot & Quick System Scripts -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-3 items-start">
      <!-- Telegram Notifications -->
      <form class="pro-card" @submit.prevent="saveTelegram">
        <div class="pro-card-header">
          <span class="pro-card-title">Telegram Bot Integration</span>
          <span :class="telegramConfigured ? 'badge badge-green' : 'badge badge-gray'">
            {{ telegramConfigured ? 'CONFIGURED' : 'NOT CONFIGURED' }}
          </span>
        </div>

        <div class="space-y-2.5 text-[12px]">
          <div>
            <label class="block text-gray-400 mb-1 text-[11px] uppercase tracking-wider font-semibold">Bot API Token</label>
            <input
              v-model="telegramToken"
              type="password"
              class="w-full pro-input mono"
              :placeholder="telegramConfigured ? '•••••••••••••••• (Saved, enter to replace)' : 'e.g. 123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ'"
            />
          </div>
          <div>
            <label class="block text-gray-400 mb-1 text-[11px] uppercase tracking-wider font-semibold">Numeric Chat ID</label>
            <input
              v-model="telegramChatId"
              type="text"
              required
              class="w-full pro-input mono"
              placeholder="e.g. 987654321 or -100123456789"
            />
          </div>

          <div class="flex justify-end gap-2 pt-1">
            <button
              type="button"
              :disabled="telegramTesting"
              class="btn-secondary text-[11px] min-w-[152px] justify-center"
              @click="testTelegram"
            >
              {{ telegramTesting ? 'Sending…' : 'Send Test Notification' }}
            </button>
            <button type="submit" class="btn-primary text-[11px] min-w-[116px] justify-center">Save Bot Config</button>
          </div>
        </div>
      </form>

      <!-- One-Click System Automation Scripts -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">One-Click Maintenance & Diagnostic Scripts</span>
          <span class="badge badge-purple">INSTANT EXEC</span>
        </div>

        <div class="space-y-2 text-[12px]">
          <div class="p-2 rounded bg-black/25 border border-white/5 flex items-center justify-between gap-2">
            <div class="min-w-0">
              <div class="font-medium text-gray-200 truncate">Flush Linux Memory Cache</div>
              <div class="mono text-[10.5px] text-gray-400 truncate">sync; echo 3 > /proc/sys/vm/drop_caches; free -m</div>
            </div>
            <button
              type="button"
              class="btn-secondary text-[11px] min-w-[88px] justify-center shrink-0"
              :disabled="runningCmd === 'sync; echo 3 > /proc/sys/vm/drop_caches; free -m'"
              @click="runNow('sync; echo 3 > /proc/sys/vm/drop_caches; free -m')"
            >
              {{ runningCmd === 'sync; echo 3 > /proc/sys/vm/drop_caches; free -m' ? 'Running…' : '▶ Execute' }}
            </button>
          </div>

          <div class="p-2 rounded bg-black/25 border border-white/5 flex items-center justify-between gap-2">
            <div class="min-w-0">
              <div class="font-medium text-gray-200 truncate">Inspect Active Listening Sockets</div>
              <div class="mono text-[10.5px] text-gray-400 truncate">netstat -tulnp</div>
            </div>
            <button
              type="button"
              class="btn-secondary text-[11px] min-w-[88px] justify-center shrink-0"
              :disabled="runningCmd === 'netstat -tulnp 2>/dev/null || ss -tuln'"
              @click="runNow('netstat -tulnp 2>/dev/null || ss -tuln')"
            >
              {{ runningCmd === 'netstat -tulnp 2>/dev/null || ss -tuln' ? 'Running…' : '▶ Execute' }}
            </button>
          </div>

          <div class="p-2 rounded bg-black/25 border border-white/5 flex items-center justify-between gap-2">
            <div class="min-w-0">
              <div class="font-medium text-gray-200 truncate">Reload DNS & DHCP Server</div>
              <div class="mono text-[10.5px] text-gray-400 truncate">/etc/init.d/dnsmasq reload</div>
            </div>
            <button
              type="button"
              class="btn-secondary text-[11px] min-w-[88px] justify-center shrink-0"
              :disabled="runningCmd === '/etc/init.d/dnsmasq reload && echo dnsmasq reloaded cleanly'"
              @click="runNow('/etc/init.d/dnsmasq reload && echo dnsmasq reloaded cleanly')"
            >
              {{ runningCmd === '/etc/init.d/dnsmasq reload && echo dnsmasq reloaded cleanly' ? 'Running…' : '▶ Execute' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
