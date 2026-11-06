<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { apiRequest, apiURL } from '@/services/api'

interface StorageMount {
  device: string
  mount_point: string
  fstype: string
}

interface FileEntry {
  name: string
  directory: boolean
  size: number
  modified: string
}

interface DirectoryResponse {
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

const breadcrumbs = computed(() => currentPath.value.split('/').filter(Boolean))
const photos = computed(() => entries.value.filter((entry) => !entry.directory && /\.(jpe?g|png|gif|webp|bmp|avif)$/i.test(entry.name)))
const folders = computed(() => entries.value.filter((entry) => entry.directory))

async function loadMounts() {
  error.value = null
  try {
    const response = await apiRequest<{ usb_mounts: StorageMount[] }>('/storage')
    mounts.value = response.usb_mounts
    if (!mounts.value.some((mount) => mount.mount_point === selectedMount.value)) {
      selectedMount.value = mounts.value[0]?.mount_point ?? ''
      currentPath.value = ''
    }
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not load USB volumes.'
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
    const query = new URLSearchParams({ mount: selectedMount.value, path: currentPath.value })
    const response = await apiRequest<DirectoryResponse>(`/files?${query.toString()}`)
    entries.value = response.entries
    truncated.value = response.truncated
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not read this folder.'
    entries.value = []
  } finally {
    loading.value = false
  }
}

function openFolder(name: string) {
  currentPath.value = currentPath.value ? `${currentPath.value}/${name}` : name
}

function openBreadcrumb(index: number) {
  currentPath.value = breadcrumbs.value.slice(0, index + 1).join('/')
}

function previewURL(name: string) {
  const query = new URLSearchParams({
    mount: selectedMount.value,
    path: currentPath.value ? `${currentPath.value}/${name}` : name,
  })
  return `${apiURL('/files/preview')}?${query.toString()}`
}

function downloadURL(name: string) {
  const query = new URLSearchParams({
    mount: selectedMount.value,
    path: currentPath.value ? `${currentPath.value}/${name}` : name,
  })
  return `${apiURL('/files/download')}?${query.toString()}`
}

function formatBytes(bytes: number) {
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(0)} KB`
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`
}

watch([selectedMount, currentPath], () => void loadDirectory())
onMounted(loadMounts)
</script>

<template>
  <section class="h-full overflow-y-auto p-5 md:p-6 space-y-5">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-sky-300">NAS · media</p>
        <h2 class="mt-1 text-xl font-semibold text-white">Photo Library</h2>
        <p class="mt-1 text-sm text-gray-400">Stream supported image files directly from mounted USB storage; no thumbnails are generated on the router.</p>
      </div>
      <button class="secondary-button" :disabled="loading" @click="loadMounts">Refresh volumes</button>
    </header>

    <p v-if="error" role="alert" class="glass-card p-4 text-sm text-red-300">{{ error }}</p>

    <div class="glass-card flex flex-wrap items-center gap-3 p-4">
      <label for="photo-volume" class="text-sm text-gray-300">USB volume</label>
      <select id="photo-volume" v-model="selectedMount" class="form-control min-w-48">
        <option value="" disabled>Select a mounted volume</option>
        <option v-for="mount in mounts" :key="`${mount.device}:${mount.mount_point}`" :value="mount.mount_point">
          {{ mount.device }} · {{ mount.mount_point }}
        </option>
      </select>
      <span class="ml-auto text-xs text-gray-500">Read-only · JPEG, PNG, GIF, WebP, BMP, AVIF</span>
    </div>

    <div v-if="mounts.length === 0 && !error" class="glass-card p-8 text-center text-sm text-gray-400">
      No mounted USB volume found. Connect a removable drive to browse its photo folders.
    </div>

    <section v-else-if="selectedMount" class="space-y-4">
      <div class="flex flex-wrap items-center gap-2 text-sm">
        <button class="text-sky-300 hover:text-white" @click="currentPath = ''">{{ selectedMount }}</button>
        <template v-for="(segment, index) in breadcrumbs" :key="`${segment}-${index}`">
          <span class="text-gray-600">/</span>
          <button class="text-sky-300 hover:text-white" @click="openBreadcrumb(index)">{{ segment }}</button>
        </template>
        <button class="secondary-button ml-auto" :disabled="loading" @click="loadDirectory">{{ loading ? 'Loading…' : 'Refresh folder' }}</button>
      </div>

      <div v-if="loading" class="glass-card p-8 text-center text-sm text-gray-400">Reading USB directory…</div>

      <section v-if="folders.length" class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
        <button
          v-for="folder in folders"
          :key="folder.name"
          class="glass-card flex min-w-0 items-center gap-3 p-3 text-left"
          @click="openFolder(folder.name)"
        >
          <span aria-hidden="true">📁</span>
          <span class="truncate text-sm text-white">{{ folder.name }}</span>
        </button>
      </section>

      <p v-if="!loading && photos.length === 0 && folders.length === 0" class="glass-card p-8 text-center text-sm text-gray-400">
        No supported photos in this folder.
      </p>

      <section v-if="photos.length" class="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-5">
        <article v-for="photo in photos" :key="photo.name" class="glass-card min-w-0 overflow-hidden">
          <a :href="previewURL(photo.name)" target="_blank" rel="noopener noreferrer" :aria-label="`Open ${photo.name}`">
            <img class="photo-library-image" :src="previewURL(photo.name)" :alt="photo.name" loading="lazy" decoding="async" />
          </a>
          <div class="flex items-center justify-between gap-2 p-3">
            <div class="min-w-0">
              <p class="truncate text-xs text-white" :title="photo.name">{{ photo.name }}</p>
              <p class="mt-1 text-[0.65rem] text-gray-500">{{ formatBytes(photo.size) }}</p>
            </div>
            <a class="secondary-button shrink-0" :href="downloadURL(photo.name)">Save</a>
          </div>
        </article>
      </section>

      <p v-if="truncated" class="text-xs text-amber-300">This folder exceeds 500 entries; only the first 500 are shown.</p>
    </section>
  </section>
</template>
