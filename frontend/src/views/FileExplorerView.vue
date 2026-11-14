<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { apiRequest } from '@/services/api'
import { useUiPreferences } from '@/composables/useUiPreferences'

const { t } = useUiPreferences()

interface ExplorerEntry {
  name: string
  path: string
  is_dir: boolean
  size: number
  mode: string
  mod_time: string
  ext: string
  is_editable: boolean
  is_image?: boolean
  is_video?: boolean
  is_audio?: boolean
}

interface QuickFolder {
  label: string
  path: string
  icon: string
  desc: string
}

const currentPath = ref('/mnt/sda1')
const parentPath = ref('/')
const entries = ref<ExplorerEntry[]>([])
const quickFolders = ref<QuickFolder[]>([])
const loading = ref(false)
const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

// Editor state
const editingFile = ref<{
  path: string
  name: string
  content: string
  originalContent: string
  mode: string
  size: number
} | null>(null)
const savingFile = ref(false)
const createBackup = ref(true)

// Media Preview Modal state (Images, Streaming Videos, Audio)
const previewMedia = ref<{
  name: string
  path: string
  size: number
  type: 'image' | 'video' | 'audio'
  url: string
} | null>(null)

function openMediaPreview(entry: ExplorerEntry) {
  const type: 'image' | 'video' | 'audio' = entry.is_image ? 'image' : entry.is_video ? 'video' : 'audio'
  const url = `/api/v1/explorer/raw?path=${encodeURIComponent(entry.path)}`
  previewMedia.value = {
    name: entry.name,
    path: entry.path,
    size: entry.size,
    type,
    url,
  }
}

function closeMediaPreview() {
  previewMedia.value = null
}

// Create modal state
const showCreateModal = ref(false)
const createType = ref<'file' | 'dir'>('file')
const createName = ref('')
const createContent = ref('')

const searchQuery = ref('')

const filteredEntries = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return entries.value
  return entries.value.filter((e) => e.name.toLowerCase().includes(q))
})

const pathSegments = computed(() => {
  const parts = currentPath.value.split('/').filter(Boolean)
  const segments: Array<{ name: string; fullPath: string }> = []
  let build = ''
  for (const p of parts) {
    build += '/' + p
    segments.push({ name: p, fullPath: build })
  }
  return segments
})

function formatBytes(bytes: number) {
  if (bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

function getFileIcon(entry: ExplorerEntry) {
  if (entry.is_dir) return '📁'
  const ext = entry.ext.toLowerCase()
  if (['.sh', '.bash'].includes(ext)) return '🐚'
  if (['.conf', '.config', '.ini'].includes(ext)) return '⚙️'
  if (['.json', '.yaml', '.yml'].includes(ext)) return '📋'
  if (['.mp4', '.mkv', '.webm', '.avi'].includes(ext)) return '🎬'
  if (['.mp3', '.m4a', '.aac', '.flac'].includes(ext)) return '🎵'
  if (['.jpg', '.jpeg', '.png', '.webp', '.gif'].includes(ext)) return '🖼️'
  if (['.tar', '.gz', '.zip', '.bin', '.ipk'].includes(ext)) return '📦'
  if (['.log', '.txt', '.md'].includes(ext)) return '📄'
  return '📝'
}

async function loadDirectory(path: string) {
  loading.value = true
  errorMsg.value = null
  try {
    const res = await apiRequest<{
      current_path: string
      parent_path: string
      entries: ExplorerEntry[]
      quick_folders: QuickFolder[]
    }>(`/explorer/list?path=${encodeURIComponent(path)}`)
    if (res) {
      currentPath.value = res.current_path
      parentPath.value = res.parent_path
      entries.value = res.entries || []
      quickFolders.value = res.quick_folders || []
    }
  } catch (err: any) {
    errorMsg.value = err.message || 'Lỗi kết nối khi duyệt thư mục'
  } finally {
    loading.value = false
  }
}

async function openEntry(entry: ExplorerEntry) {
  if (entry.is_dir) {
    await loadDirectory(entry.path)
  } else if (entry.is_image || entry.is_video || entry.is_audio) {
    openMediaPreview(entry)
  } else if (entry.is_editable) {
    await openEditor(entry.path)
  } else {
    downloadFile(entry.path)
  }
}

async function openEditor(path: string) {
  loading.value = true
  errorMsg.value = null
  try {
    const res = await apiRequest<{
      path: string
      name: string
      content: string
      mode: string
      size: number
    }>(`/explorer/read?path=${encodeURIComponent(path)}`)
    if (res) {
      editingFile.value = {
        path: res.path,
        name: res.name,
        content: res.content,
        originalContent: res.content,
        mode: res.mode,
        size: res.size,
      }
    }
  } catch (err: any) {
    errorMsg.value = err.message || 'Lỗi khi mở tệp để chỉnh sửa'
  } finally {
    loading.value = false
  }
}

async function saveCurrentFile() {
  if (!editingFile.value) return
  savingFile.value = true
  errorMsg.value = null
  successMsg.value = null
  try {
    const res = await apiRequest<{ saved: boolean; message: string }>(
      '/explorer/write',
      {
        method: 'POST',
        body: JSON.stringify({
          path: editingFile.value.path,
          content: editingFile.value.content,
          backup: createBackup.value,
        }),
      }
    )
    successMsg.value = res?.message || 'Đã lưu tệp thành công!'
    editingFile.value.originalContent = editingFile.value.content
    setTimeout(() => (successMsg.value = null), 4000)
    void loadDirectory(currentPath.value)
  } catch (err: any) {
    errorMsg.value = err.message || 'Lỗi khi lưu tệp'
  } finally {
    savingFile.value = false
  }
}

function closeEditor() {
  if (
    editingFile.value &&
    editingFile.value.content !== editingFile.value.originalContent
  ) {
    if (!confirm('Tệp có thay đổi chưa lưu. Bạn có chắc muốn đóng?')) {
      return
    }
  }
  editingFile.value = null
}

async function createNewItem() {
  const name = createName.value.trim()
  if (!name) return
  loading.value = true
  errorMsg.value = null
  try {
    await apiRequest('/explorer/create', {
      method: 'POST',
      body: JSON.stringify({
        parent_path: currentPath.value,
        name,
        type: createType.value,
        content: createContent.value,
      }),
    })
    showCreateModal.value = false
    createName.value = ''
    createContent.value = ''
    await loadDirectory(currentPath.value)
  } catch (err: any) {
    errorMsg.value = err.message || 'Lỗi kết nối khi tạo mục mới'
  } finally {
    loading.value = false
  }
}

async function deleteItem(entry: ExplorerEntry) {
  const label = entry.is_dir ? 'thư mục' : 'tệp'
  if (!confirm(`Bạn có chắc chắn muốn XÓA vĩnh viễn ${label} "${entry.name}"?`)) {
    return
  }
  loading.value = true
  errorMsg.value = null
  try {
    await apiRequest(
      `/explorer/delete?path=${encodeURIComponent(entry.path)}`,
      { method: 'DELETE' }
    )
    if (editingFile.value?.path === entry.path) {
      editingFile.value = null
    }
    await loadDirectory(currentPath.value)
  } catch (err: any) {
    errorMsg.value = err.message || 'Lỗi kết nối khi xóa'
  } finally {
    loading.value = false
  }
}

function downloadFile(path: string) {
  window.open(`/api/v1/explorer/raw?download=1&path=${encodeURIComponent(path)}`, '_blank')
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    if (previewMedia.value) closeMediaPreview()
    else if (editingFile.value) closeEditor()
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  void loadDirectory('/mnt/sda1')
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="h-full w-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Floating Toast Notifications (Never shift layout) -->
    <div v-if="errorMsg" class="r3p-toast r3p-toast-error">
      <div class="flex items-start justify-between gap-3">
        <span class="r3p-toast-text">⚠️ {{ errorMsg }}</span>
        <button type="button" class="text-rose-300 hover:text-white shrink-0 font-bold" @click="errorMsg = null">✕</button>
      </div>
    </div>
    <div v-else-if="successMsg" class="r3p-toast r3p-toast-success">
      <div class="flex items-start justify-between gap-3">
        <span class="r3p-toast-text">✅ {{ successMsg }}</span>
        <button type="button" class="text-emerald-300 hover:text-white shrink-0 font-bold" @click="successMsg = null">✕</button>
      </div>
    </div>

    <!-- Quick Location Shortcuts -->
    <div class="pro-card p-3">
      <div class="text-[11px] font-bold text-gray-400 uppercase tracking-wider mb-2 flex items-center gap-1.5">
        <span>{{ t('exp_quick_roots') }}</span>
      </div>
      <div class="flex items-center gap-1.5 flex-wrap">
        <button
          v-for="q in quickFolders"
          :key="q.path"
          type="button"
          class="px-2.5 py-1.5 rounded-lg border text-xs font-semibold flex items-center gap-1.5 transition cursor-pointer"
          :class="currentPath === q.path || currentPath.startsWith(q.path + '/')
            ? 'bg-sky-500/20 border-sky-500/50 text-sky-300 shadow-sm'
            : 'bg-black/30 border-white/10 text-gray-400 hover:text-gray-200 hover:border-white/20'"
          :title="q.desc"
          @click="loadDirectory(q.path)"
        >
          <span>{{ q.icon }}</span>
          <span>{{ q.label }}</span>
        </button>
      </div>
    </div>

    <!-- Path Bar & Action Toolbar -->
    <div class="pro-card p-3 flex flex-col md:flex-row items-center justify-between gap-2.5">
      <!-- Breadcrumbs -->
      <div class="flex items-center gap-1 overflow-x-auto w-full md:w-auto text-xs py-1 min-w-0">
        <button
          type="button"
          class="btn-secondary px-2 py-1 text-xs shrink-0"
          :disabled="currentPath === '/' || loading"
          title="Lên thư mục cha"
          @click="loadDirectory(parentPath)"
        >
          ⬆️ Lên
        </button>
        <button
          type="button"
          class="hover:underline font-bold text-sky-400 shrink-0 px-1 cursor-pointer"
          @click="loadDirectory('/')"
        >
          root/
        </button>
        <template v-for="(seg, idx) in pathSegments" :key="seg.fullPath">
          <span class="text-gray-600">/</span>
          <button
            type="button"
            class="hover:underline font-semibold shrink-0 px-1 cursor-pointer"
            :class="idx === pathSegments.length - 1 ? 'text-gray-200' : 'text-sky-300'"
            @click="loadDirectory(seg.fullPath)"
          >
            {{ seg.name }}
          </button>
        </template>
      </div>

      <!-- Action Buttons & Search -->
      <div class="flex items-center gap-2 w-full md:w-auto shrink-0 justify-end">
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Lọc tệp..."
          class="pro-input py-1 text-xs w-36 md:w-44"
        />
        <button
          type="button"
          class="btn-secondary py-1 text-xs flex items-center gap-1 min-w-[84px] justify-center"
          :disabled="loading"
          @click="loadDirectory(currentPath)"
        >
          <span>🔄</span>
          <span class="hidden sm:inline">Làm mới</span>
        </button>
        <button
          type="button"
          class="btn-primary py-1 text-xs flex items-center gap-1 min-w-[68px] justify-center"
          @click="createType = 'file'; showCreateModal = true"
        >
          <span>+ Tệp</span>
        </button>
        <button
          type="button"
          class="btn-secondary py-1 text-xs flex items-center gap-1 min-w-[88px] justify-center"
          @click="createType = 'dir'; showCreateModal = true"
        >
          <span>+ Thư mục</span>
        </button>
      </div>
    </div>

    <!-- Active Text/Code Editor Pane -->
    <div v-if="editingFile" class="pro-card border-sky-500/40 p-0 overflow-hidden shadow-2xl">
      <!-- Editor Header -->
      <div class="px-4 py-2.5 bg-black/40 border-b border-white/10 flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-2 min-w-0">
          <span class="text-base">📝</span>
          <div class="min-w-0">
            <div class="text-xs font-bold text-sky-300 mono truncate">{{ editingFile.path }}</div>
            <div class="text-[10px] text-gray-500 mono">
              {{ editingFile.mode }} · {{ formatBytes(editingFile.content.length) }}
              <span v-if="editingFile.content !== editingFile.originalContent" class="text-amber-400 font-bold ml-1">● Có thay đổi chưa lưu</span>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2 shrink-0">
          <label class="flex items-center gap-1.5 text-[11px] text-gray-400 cursor-pointer select-none">
            <input v-model="createBackup" type="checkbox" class="rounded accent-sky-500" />
            <span>Tạo file .bak</span>
          </label>
          <button
            type="button"
            class="btn-primary py-1 text-xs flex items-center gap-1 min-w-[104px] justify-center"
            :disabled="savingFile"
            @click="saveCurrentFile"
          >
            <span>💾</span>
            <span>{{ savingFile ? 'Đang lưu...' : 'Lưu Tệp' }}</span>
          </button>
          <button
            type="button"
            class="btn-secondary py-1 text-xs min-w-[80px] justify-center"
            @click="openEditor(editingFile.path)"
          >
            🔄 Tải lại
          </button>
          <button
            type="button"
            class="btn-danger py-1 text-xs min-w-[68px] justify-center"
            @click="closeEditor"
          >
            ✕ Đóng
          </button>
        </div>
      </div>

      <!-- Editor Textarea -->
      <div class="p-2 bg-[#090d16]">
        <textarea
          v-model="editingFile.content"
          rows="18"
          class="w-full bg-[#070b12] text-emerald-300 mono text-xs p-3 rounded-lg border border-white/10 focus:border-sky-500 focus:outline-none resize-y leading-relaxed"
          placeholder="Nội dung tệp..."
          spellcheck="false"
        ></textarea>
      </div>
    </div>

    <!-- File & Folder List -->
    <div class="pro-card p-0 overflow-hidden">
      <div class="px-4 py-2 bg-black/20 border-b border-white/10 flex items-center justify-between text-xs text-gray-400 gap-2">
        <span class="truncate">Danh mục: {{ currentPath }} ({{ filteredEntries.length }} mục)</span>
        <span class="text-sky-400 min-w-[110px] text-right shrink-0">{{ loading ? 'Đang tải dữ liệu...' : '' }}</span>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs border-collapse">
          <thead>
            <tr class="border-b border-white/10 text-gray-400 font-semibold bg-white/[0.02]">
              <th class="py-2.5 px-4 w-12">Loại</th>
              <th class="py-2.5 px-3">Tên</th>
              <th class="py-2.5 px-3 w-28">Kích thước</th>
              <th class="py-2.5 px-3 w-24">Quyền</th>
              <th class="py-2.5 px-3 w-36 hidden md:table-cell">Ngày sửa</th>
              <th class="py-2.5 px-4 w-44 text-right">Thao tác</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-white/[0.04]">
            <tr v-if="!filteredEntries.length && !loading">
              <td colspan="6" class="py-8 text-center text-gray-500">
                Thư mục trống hoặc không có tệp nào phù hợp tìm kiếm.
              </td>
            </tr>
            <tr
              v-for="entry in filteredEntries"
              :key="entry.path"
              class="hover:bg-white/[0.03] transition group"
            >
              <td class="py-2 px-4 text-base cursor-pointer" @click="openEntry(entry)">
                {{ getFileIcon(entry) }}
              </td>
              <td class="py-2 px-3 cursor-pointer max-w-[280px] sm:max-w-md lg:max-w-none" @click="openEntry(entry)">
                <div class="font-medium text-gray-200 group-hover:text-sky-400 transition flex items-center gap-1.5 min-w-0">
                  <span class="truncate" :title="entry.name">{{ entry.name }}</span>
                  <span v-if="entry.is_dir" class="text-gray-500 text-[10px] shrink-0">/</span>
                </div>
              </td>
              <td class="py-2 px-3 mono text-gray-400 whitespace-nowrap">
                {{ entry.is_dir ? '—' : formatBytes(entry.size) }}
              </td>
              <td class="py-2 px-3 mono text-[11px] text-gray-500 whitespace-nowrap">
                {{ entry.mode }}
              </td>
              <td class="py-2 px-3 text-gray-500 hidden md:table-cell whitespace-nowrap">
                {{ entry.mod_time }}
              </td>
              <td class="py-2 px-4 text-right whitespace-nowrap">
                <div class="flex items-center justify-end gap-1">
                  <button
                    v-if="!entry.is_dir && entry.is_image"
                    type="button"
                    class="px-2 py-0.5 rounded bg-purple-500/20 text-purple-300 hover:bg-purple-500/30 font-semibold text-[11px] cursor-pointer"
                    title="Xem ảnh trực tiếp (JPG/PNG/WebP/GIF)"
                    @click="openMediaPreview(entry)"
                  >
                    👁️ Xem
                  </button>
                  <button
                    v-if="!entry.is_dir && entry.is_video"
                    type="button"
                    class="px-2 py-0.5 rounded bg-rose-500/20 text-rose-300 hover:bg-rose-500/30 font-semibold text-[11px] cursor-pointer flex items-center gap-1"
                    title="Phát video stream HTML5 nhẹ mượt (Không tốn RAM Router)"
                    @click="openMediaPreview(entry)"
                  >
                    <span>▶️ Stream</span>
                  </button>
                  <button
                    v-if="!entry.is_dir && entry.is_audio"
                    type="button"
                    class="px-2 py-0.5 rounded bg-amber-500/20 text-amber-300 hover:bg-amber-500/30 font-semibold text-[11px] cursor-pointer"
                    title="Nghe nhạc trực tiếp"
                    @click="openMediaPreview(entry)"
                  >
                    🎵 Nghe
                  </button>
                  <button
                    v-if="!entry.is_dir && entry.is_editable"
                    type="button"
                    class="px-2 py-0.5 rounded bg-sky-500/20 text-sky-300 hover:bg-sky-500/30 font-semibold text-[11px] cursor-pointer"
                    title="Chỉnh sửa tệp text/code trực tiếp"
                    @click="openEditor(entry.path)"
                  >
                    ✏️ Sửa
                  </button>
                  <button
                    v-if="!entry.is_dir"
                    type="button"
                    class="px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300 hover:bg-emerald-500/30 font-semibold text-[11px] cursor-pointer"
                    title="Tải về máy tính"
                    @click="downloadFile(entry.path)"
                  >
                    ⬇️ Tải
                  </button>
                  <button
                    type="button"
                    class="px-2 py-0.5 rounded bg-rose-500/10 text-rose-400 hover:bg-rose-500/20 font-semibold text-[11px] cursor-pointer"
                    title="Xóa tệp/thư mục"
                    @click="deleteItem(entry)"
                  >
                    🗑️
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Create File / Folder Modal -->
    <div
      v-if="showCreateModal"
      class="fixed inset-0 bg-black/70 backdrop-blur-sm z-50 flex items-center justify-center p-4"
    >
      <div class="pro-card max-w-md w-full border-sky-500/30 p-5 space-y-4">
        <div class="flex items-center justify-between border-b border-white/10 pb-2">
          <h3 class="text-sm font-bold text-gray-200">
            {{ createType === 'file' ? 'Tạo Tệp Văn Bản / Script Mới' : 'Tạo Thư Mục Mới' }}
          </h3>
          <button class="text-gray-400 hover:text-white" @click="showCreateModal = false">✕</button>
        </div>

        <div>
          <label class="block text-xs font-semibold text-gray-400 mb-1">Thư mục hiện tại:</label>
          <div class="text-xs text-sky-400 mono bg-black/30 p-2 rounded">{{ currentPath }}</div>
        </div>

        <div>
          <label class="block text-xs font-semibold text-gray-400 mb-1">Tên {{ createType === 'file' ? 'tệp (ví dụ: script.sh, custom.conf)' : 'thư mục' }}:</label>
          <input
            v-model="createName"
            type="text"
            class="pro-input w-full text-xs"
            placeholder="Nhập tên..."
            @keyup.enter="createNewItem"
          />
        </div>

        <div v-if="createType === 'file'">
          <label class="block text-xs font-semibold text-gray-400 mb-1">Nội dung ban đầu (tùy chọn):</label>
          <textarea
            v-model="createContent"
            rows="5"
            class="w-full bg-[#070b12] text-gray-300 mono text-xs p-2 rounded-lg border border-white/10 focus:border-sky-500 focus:outline-none"
            placeholder="#!/bin/sh..."
          ></textarea>
        </div>

        <div class="flex items-center justify-end gap-2 pt-2">
          <button class="btn-secondary text-xs" @click="showCreateModal = false">Hủy</button>
          <button
            class="btn-primary text-xs"
            :disabled="!createName.trim() || loading"
            @click="createNewItem"
          >
            Tạo Ngay
          </button>
        </div>
      </div>
    </div>

    <!-- Media Viewer Modal (Images, Streaming Videos, Audio) -->
    <div
      v-if="previewMedia"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/85 backdrop-blur-md p-2 sm:p-6"
      @click.self="closeMediaPreview"
    >
      <div class="bg-[#0b0f19] border border-white/15 rounded-2xl max-w-5xl w-full max-h-[94vh] flex flex-col shadow-2xl overflow-hidden">
        <!-- Header -->
        <div class="px-4 py-3 bg-black/40 border-b border-white/10 flex items-center justify-between gap-3 flex-shrink-0">
          <div class="flex items-center gap-2 min-w-0">
            <span class="text-xl">
              {{ previewMedia.type === 'image' ? '🖼️' : previewMedia.type === 'video' ? '🎬' : '🎵' }}
            </span>
            <div class="min-w-0">
              <div class="font-bold text-sm text-gray-100 truncate mono">{{ previewMedia.name }}</div>
              <div class="text-[11px] text-gray-400 flex items-center gap-2">
                <span>{{ formatBytes(previewMedia.size) }}</span>
                <span>•</span>
                <span class="mono text-gray-500 truncate max-w-xs">{{ previewMedia.path }}</span>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-2 flex-shrink-0">
            <span
              v-if="previewMedia.type === 'video'"
              class="px-2 py-0.5 rounded bg-purple-500/20 text-purple-300 border border-purple-500/30 text-[10px] font-semibold hidden sm:inline-flex"
            >
              HTML5 HTTP 206 STREAM
            </span>
            <span
              v-else-if="previewMedia.type === 'image'"
              class="px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300 border border-emerald-500/30 text-[10px] font-semibold hidden sm:inline-flex"
            >
              CLIENT CODEC (0 KB RAM)
            </span>
            <button
              class="px-2.5 py-1 text-xs rounded bg-emerald-500/20 hover:bg-emerald-500/30 text-emerald-300 font-semibold flex items-center gap-1 transition cursor-pointer"
              title="Tải tệp này về máy"
              @click="downloadFile(previewMedia.path)"
            >
              <span>⬇️ Tải về</span>
            </button>
            <button
              class="p-1.5 rounded-lg text-gray-400 hover:text-white hover:bg-white/10 text-lg transition cursor-pointer leading-none"
              title="Đóng (Esc)"
              @click="closeMediaPreview"
            >
              ✕
            </button>
          </div>
        </div>

        <!-- Body -->
        <div class="flex-1 overflow-auto p-4 flex items-center justify-center bg-black/60 min-h-[300px]">
          <!-- Image Viewer -->
          <div v-if="previewMedia.type === 'image'" class="flex flex-col items-center justify-center w-full">
            <img
              :src="previewMedia.url"
              :alt="previewMedia.name"
              class="max-h-[74vh] max-w-full object-contain rounded-lg shadow-2xl border border-white/10 select-none"
              loading="lazy"
            />
            <div class="mt-2 text-[11px] text-gray-400 flex items-center justify-between w-full max-w-4xl px-2">
              <span class="text-emerald-400">Trình duyệt giải mã trực tiếp (JPG / PNG / WebP / GIF)</span>
              <span class="text-gray-500 hidden sm:inline">Phím Esc để đóng</span>
            </div>
          </div>

          <!-- Video Streamer -->
          <div v-else-if="previewMedia.type === 'video'" class="w-full flex flex-col items-center">
            <video
              controls
              autoplay
              preload="metadata"
              :src="previewMedia.url"
              class="max-h-[72vh] w-full max-w-4xl mx-auto rounded-lg shadow-2xl bg-black border border-white/10 focus:outline-none"
            >
              Trình duyệt của bạn không hỗ trợ phát trực tiếp video này.
            </video>
            <div class="mt-2 text-[11px] text-gray-400 flex items-center justify-between w-full max-w-4xl px-2">
              <span class="flex items-center gap-1.5 text-emerald-400">
                <span class="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
                <span>Stream trực tiếp qua HTTP Range — Router chỉ gửi byte cần phát, không gánh RAM</span>
              </span>
              <span class="text-gray-500 hidden sm:inline">Phím Space: Tạm dừng / Phát</span>
            </div>
          </div>

          <!-- Audio Player -->
          <div v-else-if="previewMedia.type === 'audio'" class="w-full max-w-md p-6 bg-black/40 rounded-xl border border-white/10 flex flex-col items-center gap-4 text-center">
            <div class="w-20 h-20 rounded-full bg-amber-500/20 border border-amber-500/30 flex items-center justify-center text-4xl">
              🎵
            </div>
            <div>
              <div class="font-bold text-gray-200 text-sm mono">{{ previewMedia.name }}</div>
              <div class="text-xs text-gray-400 mt-1">{{ formatBytes(previewMedia.size) }}</div>
            </div>
            <audio
              controls
              autoplay
              :src="previewMedia.url"
              class="w-full mt-2"
            ></audio>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
