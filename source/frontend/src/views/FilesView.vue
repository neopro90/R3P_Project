<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { apiRequest, apiURL } from '@/services/api'

interface StorageMount {
  device: string
  mount_point: string
  fstype: string
}

interface StorageResponse {
  usb_mounts: StorageMount[]
}

interface FileEntry {
  name: string
  directory: boolean
  size: number
  modified: string
}

interface DirectoryResponse {
  mount: string
  path: string
  entries: FileEntry[]
  truncated: boolean
}

const mounts = ref<StorageMount[]>([])
const selectedMount = ref('')
const currentPath = ref('')
const entries = ref<FileEntry[]>([])
const truncated = ref(false)
const loading = ref(false)
const error = ref<string | null>(null)

const breadcrumbs = computed(() => {
  if (!currentPath.value) return []
  return currentPath.value.split('/').filter(Boolean)
})

async function loadMounts() {
  error.value = null
  try {
    const response = await apiRequest<StorageResponse>('/storage')
    mounts.value = response.usb_mounts
    if (!mounts.value.some((mount) => mount.mount_point === selectedMount.value)) {
      selectedMount.value = mounts.value[0]?.mount_point ?? ''
      currentPath.value = ''
    }
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not load USB storage.'
    mounts.value = []
    selectedMount.value = ''
    entries.value = []
  }
}

async function loadDirectory() {
  if (!selectedMount.value) {
    entries.value = []
    return
  }
  loading.value = true
  error.value = null
  try {
    const query = new URLSearchParams({
      mount: selectedMount.value,
      path: currentPath.value,
    })
    const response = await apiRequest<DirectoryResponse>(`/files?${query.toString()}`)
    entries.value = response.entries
    truncated.value = response.truncated
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not read this directory.'
    entries.value = []
  } finally {
    loading.value = false
  }
}

function openDirectory(name: string) {
  currentPath.value = currentPath.value ? `${currentPath.value}/${name}` : name
}

function openBreadcrumb(index: number) {
  currentPath.value = breadcrumbs.value.slice(0, index + 1).join('/')
}

function downloadUrl(name: string) {
  const query = new URLSearchParams({
    mount: selectedMount.value,
    path: currentPath.value ? `${currentPath.value}/${name}` : name,
  })
  return `${apiURL('/files/download')}?${query.toString()}`
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MB`
  return `${(bytes / 1024 ** 3).toFixed(2)} GB`
}

watch(selectedMount, (value, previous) => {
  if (previous && value !== previous) currentPath.value = ''
})

watch([selectedMount, currentPath], () => void loadDirectory())
onMounted(loadMounts)
</script>

<template>
  <section class="h-full overflow-y-auto p-5 md:p-6 space-y-5">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="text-xl font-semibold text-white">USB file browser</h2>
        <p class="mt-1 text-sm text-gray-400">Browse and download files from attached removable storage.</p>
      </div>
      <button class="secondary-button" @click="loadMounts">Refresh volumes</button>
    </header>

    <p v-if="error" role="alert" class="glass-card p-4 text-sm text-red-300">{{ error }}</p>

    <div class="glass-card p-4 flex flex-wrap items-center gap-3">
      <label for="storage-volume" class="text-sm text-gray-300">Volume</label>
      <select id="storage-volume" v-model="selectedMount" class="form-control min-w-48">
        <option value="" disabled>Select a mounted USB volume</option>
        <option v-for="mount in mounts" :key="`${mount.device}:${mount.mount_point}`" :value="mount.mount_point">
          {{ mount.device }} · {{ mount.mount_point }} · {{ mount.fstype }}
        </option>
      </select>
      <span class="ml-auto text-xs text-gray-500">Read-only · no upload, delete, or overwrite operations</span>
    </div>

    <div v-if="mounts.length === 0 && !error" class="glass-card p-8 text-center">
      <p class="text-sm text-gray-300">No mounted USB storage volume found.</p>
      <p class="mt-2 text-xs text-gray-500">Connect and mount a supported USB drive to browse its files.</p>
    </div>

    <section v-else class="glass-card overflow-hidden">
      <div class="flex flex-wrap items-center gap-2 border-b border-white/10 px-4 py-3 text-sm">
        <button class="text-sky-300 hover:text-white" @click="currentPath = ''">{{ selectedMount || 'Volume' }}</button>
        <template v-for="(segment, index) in breadcrumbs" :key="`${segment}-${index}`">
          <span class="text-gray-600">/</span>
          <button class="text-sky-300 hover:text-white" @click="openBreadcrumb(index)">{{ segment }}</button>
        </template>
        <button class="secondary-button ml-auto" :disabled="loading" @click="loadDirectory">
          {{ loading ? 'Loading…' : 'Refresh folder' }}
        </button>
      </div>

      <div v-if="loading" class="p-8 text-center text-sm text-gray-400">Reading directory…</div>
      <div v-else-if="entries.length === 0" class="p-8 text-center text-sm text-gray-500">This folder is empty.</div>
      <ul v-else class="divide-y divide-white/5">
        <li v-for="entry in entries" :key="entry.name" class="file-row">
          <button
            v-if="entry.directory"
            class="flex min-w-0 flex-1 items-center gap-3 text-left"
            @click="openDirectory(entry.name)"
          >
            <span aria-hidden="true" class="text-lg">📁</span>
            <span class="truncate text-sm text-white">{{ entry.name }}</span>
          </button>
          <div v-else class="flex min-w-0 flex-1 items-center gap-3">
            <span aria-hidden="true" class="text-lg">📄</span>
            <span class="truncate text-sm text-gray-200">{{ entry.name }}</span>
          </div>
          <span class="hidden text-xs text-gray-500 sm:block">{{ entry.directory ? 'Folder' : formatBytes(entry.size) }}</span>
          <a v-if="!entry.directory" class="secondary-button" :href="downloadUrl(entry.name)">Download</a>
        </li>
      </ul>
      <p v-if="truncated" class="border-t border-white/10 px-4 py-3 text-xs text-amber-300">
        This folder has more than 500 entries; showing the first 500.
      </p>
    </section>
  </section>
</template>
