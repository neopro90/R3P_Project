<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { ApiError, apiRequest } from '@/services/api'

interface StorageMount {
  device: string
  mount_point: string
  fstype: string
}

interface TelegramStatus {
  configured: boolean
  enabled: boolean
  running: boolean
  connected: boolean
  chat_id?: string
  mount?: string
  path?: string
  last_error?: string
}

interface DownloadItem {
  gid: string
  status: string
  totalLength: string
  completedLength: string
  downloadSpeed: string
  files: Array<{ path: string }>
}

const mounts = ref<StorageMount[]>([])
const selectedMount = ref('')
const destinationPath = ref('Downloads')
const botToken = ref('')
const chatId = ref('')
const status = ref<TelegramStatus | null>(null)
const downloads = ref<DownloadItem[]>([])
const manualURL = ref('')
const loading = ref(false)
const saving = ref(false)
const error = ref<string | null>(null)
const notice = ref<string | null>(null)
let refreshTimer: ReturnType<typeof setInterval> | null = null

async function refresh() {
  loading.value = true
  error.value = null
  try {
    const [storage, telegram] = await Promise.all([
      apiRequest<{ usb_mounts: StorageMount[] }>('/storage'),
      apiRequest<TelegramStatus>('/telegram'),
    ])
    mounts.value = storage.usb_mounts
    status.value = telegram
    chatId.value = telegram.chat_id ?? chatId.value
    if (telegram.mount && mounts.value.some((mount) => mount.mount_point === telegram.mount)) {
      selectedMount.value = telegram.mount
      destinationPath.value = telegram.path ?? ''
    } else if (!mounts.value.some((mount) => mount.mount_point === selectedMount.value)) {
      selectedMount.value = mounts.value[0]?.mount_point ?? ''
    }
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not read download integration status.'
    status.value = null
  }
  try {
    const response = await apiRequest<{ downloads: DownloadItem[] }>('/downloads')
    downloads.value = response.downloads
  } catch (cause) {
    if (cause instanceof ApiError && cause.status !== 503) {
      error.value = cause.message
    }
    downloads.value = []
  } finally {
    loading.value = false
  }
}

async function saveTelegram() {
  if (!selectedMount.value) {
    error.value = 'Select a mounted USB destination first.'
    return
  }
  saving.value = true
  error.value = null
  notice.value = null
  try {
    status.value = await apiRequest<TelegramStatus>('/telegram', {
      method: 'PUT',
      body: JSON.stringify({
        bot_token: botToken.value,
        chat_id: chatId.value.trim(),
        mount: selectedMount.value,
        path: destinationPath.value.trim(),
        enabled: true,
      }),
    })
    botToken.value = ''
    chatId.value = status.value.chat_id ?? ''
    notice.value = 'Telegram URL intake saved. The bot token is stored only on the router.'
    await refresh()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not save Telegram settings.'
  } finally {
    saving.value = false
  }
}

async function disableTelegram() {
  saving.value = true
  error.value = null
  notice.value = null
  try {
    status.value = await apiRequest<TelegramStatus>('/telegram', { method: 'DELETE' })
    botToken.value = ''
    chatId.value = ''
    notice.value = 'Telegram integration and its stored bot token were removed.'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not remove Telegram settings.'
  } finally {
    saving.value = false
  }
}

async function addDownload() {
  if (!selectedMount.value) {
    error.value = 'Select a mounted USB destination first.'
    return
  }
  error.value = null
  notice.value = null
  try {
    await apiRequest<{ gid: string }>('/downloads', {
      method: 'POST',
      body: JSON.stringify({
        url: manualURL.value.trim(),
        mount: selectedMount.value,
        path: destinationPath.value.trim(),
      }),
    })
    manualURL.value = ''
    notice.value = 'URL was added to the aria2 queue.'
    await refresh()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not queue this URL.'
  }
}

function percent(item: DownloadItem) {
  const total = Number(item.totalLength)
  const completed = Number(item.completedLength)
  if (!Number.isFinite(total) || total <= 0 || !Number.isFinite(completed)) return '—'
  return `${Math.min(100, Math.round((completed / total) * 100))}%`
}

function displayName(item: DownloadItem) {
  const file = item.files[0]?.path
  return file ? file.split(/[\\/]/).pop() || file : item.gid
}

onMounted(() => {
  void refresh()
  refreshTimer = setInterval(() => void refresh(), 15_000)
})

onUnmounted(() => {
  if (refreshTimer !== null) clearInterval(refreshTimer)
})
</script>

<template>
  <section class="h-full overflow-y-auto p-5 md:p-6 space-y-5">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-sky-300">Mini-server · automation</p>
        <h2 class="mt-1 text-xl font-semibold text-white">Auto Download</h2>
        <p class="mt-1 text-sm text-gray-400">Telegram messages from one allowlisted chat are parsed for links and queued in aria2.</p>
      </div>
      <button class="secondary-button" :disabled="loading || saving" @click="refresh">{{ loading ? 'Refreshing…' : 'Refresh queue' }}</button>
    </header>

    <p v-if="error" role="alert" class="glass-card p-4 text-sm text-red-300">{{ error }}</p>
    <p v-if="notice" role="status" class="glass-card p-4 text-sm text-emerald-300">{{ notice }}</p>

    <section class="glass-card space-y-4 p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="font-semibold text-white">Telegram URL intake</h3>
          <p class="mt-1 text-xs text-gray-400">The token is never returned by the API or stored in browser preferences.</p>
        </div>
        <span class="status-pill" :class="status?.connected ? 'status-pill-online' : 'status-pill-muted'">
          {{ status?.connected ? 'Telegram connected' : status?.configured ? 'Configured · checking' : 'Not configured' }}
        </span>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <label class="form-label">
          Bot token
          <input
            v-model="botToken"
            class="form-control"
            type="password"
            autocomplete="new-password"
            spellcheck="false"
            :placeholder="status?.configured ? 'Leave blank to keep saved token' : 'Paste token from BotFather'"
          />
        </label>
        <label class="form-label">
          Allowlisted Telegram chat ID
          <input v-model="chatId" class="form-control" inputmode="numeric" autocomplete="off" placeholder="e.g. 123456789" />
        </label>
        <label class="form-label">
          Download volume
          <select v-model="selectedMount" class="form-control">
            <option value="" disabled>Select USB volume</option>
            <option v-for="mount in mounts" :key="`${mount.device}:${mount.mount_point}`" :value="mount.mount_point">
              {{ mount.device }} · {{ mount.mount_point }}
            </option>
          </select>
        </label>
        <label class="form-label">
          Existing folder on volume
          <input v-model="destinationPath" class="form-control" autocomplete="off" placeholder="Downloads" />
        </label>
      </div>

      <div class="flex flex-wrap items-center gap-3">
        <button class="primary-button" :disabled="saving || !selectedMount" @click="saveTelegram">
          {{ saving ? 'Saving…' : status?.configured ? 'Save & enable' : 'Connect Telegram' }}
        </button>
        <button v-if="status?.configured" class="secondary-button" :disabled="saving" @click="disableTelegram">Remove integration</button>
      </div>
      <ul class="list-disc space-y-1 pl-5 text-xs text-gray-400">
        <li>Use HTTPS and a private Telegram chat; only the configured chat ID can submit URLs.</li>
        <li>Disable any webhook for this bot; the router uses Telegram long polling.</li>
        <li>Supported links: HTTP, HTTPS, and magnet. Links are limited to a mounted USB destination.</li>
        <li>Redirects are disabled in aria2. Do not configure this bot in an untrusted group.</li>
      </ul>
      <p v-if="status?.last_error" role="status" class="text-xs text-amber-300">{{ status.last_error }}</p>
    </section>

    <section class="glass-card space-y-4 p-5">
      <div>
        <h3 class="font-semibold text-white">Add URL manually</h3>
        <p class="mt-1 text-xs text-gray-400">Queues directly to aria2; destination is confined to the selected mounted USB folder.</p>
      </div>
      <div class="flex flex-col gap-3 sm:flex-row">
        <input v-model="manualURL" class="form-control min-w-0 flex-1" autocomplete="off" placeholder="https://example.com/file.zip or magnet:?…" />
        <button class="primary-button" :disabled="!manualURL.trim() || !selectedMount" @click="addDownload">Queue download</button>
      </div>
    </section>

    <section class="space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="font-semibold text-white">aria2 queue</h3>
        <span class="text-xs text-gray-500">{{ downloads.length }} item(s)</span>
      </div>
      <p v-if="downloads.length === 0" class="glass-card p-5 text-sm text-gray-400">No download items reported. aria2 may be unavailable or the queue is empty.</p>
      <article v-for="item in downloads" :key="item.gid" class="glass-card flex flex-wrap items-center justify-between gap-3 p-4">
        <div class="min-w-0">
          <h4 class="truncate text-sm font-medium text-white">{{ displayName(item) }}</h4>
          <p class="mt-1 text-xs text-gray-500">GID {{ item.gid }} · {{ item.status }} · {{ percent(item) }}</p>
        </div>
        <span class="text-xs text-gray-400">{{ item.downloadSpeed }} B/s</span>
      </article>
    </section>
  </section>
</template>
