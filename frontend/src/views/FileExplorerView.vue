<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { apiRequest } from '@/services/api'

interface ExplorerEntry {
  name: string
  path: string
  is_dir: boolean
  size: number
  mode: string
  mod_time: string
  ext: string
  is_editable: boolean
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
  if (['.jpg', '.jpeg', '.png', '.webp', '.svg'].includes(ext)) return '🖼️'
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
  window.open(`/api/v1/nas/download?path=${encodeURIComponent(path)}`, '_blank')
}

onMounted(() => {
  void loadDirectory('/mnt/sda1')
})
</script>

<template>
  <div class="space-y-4 max-w-7xl mx-auto p-2 sm:p-4">
    <!-- Notice Banners -->
    <div v-if="errorMsg" class="p-3 bg-rose-500/10 border border-rose-500/30 rounded-xl text-rose-300 text-xs flex items-center justify-between">
      <span>⚠️ {{ errorMsg }}</span>
      <button class="text-rose-400 hover:text-white" @click="errorMsg = null">✕</button>
    </div>
    <div v-if="successMsg" class="p-3 bg-emerald-500/10 border border-emerald-500/30 rounded-xl text-emerald-300 text-xs flex items-center justify-between">
      <span>✅ {{ successMsg }}</span>
      <button class="text-emerald-400 hover:text-white" @click="successMsg = null">✕</button>
    </div>

    <!-- Quick Location Shortcuts -->
    <div class="pro-card p-3">
      <div class="text-[11px] font-bold text-gray-400 uppercase tracking-wider mb-2 flex items-center gap-1.5">
        <span>⚡ Lối Tắt Thư Mục Hệ Thống & Lưu Trữ (Quick Roots)</span>
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
      <div class="flex items-center gap-1 overflow-x-auto w-full md:w-auto text-xs py-1">
        <button
          class="btn-secondary px-2 py-1 text-xs shrink-0"
          :disabled="currentPath === '/' || loading"
          title="Lên thư mục cha"
          @click="loadDirectory(parentPath)"
        >
          ⬆️ Lên
        </button>
        <button
          class="hover:underline font-bold text-sky-400 shrink-0 px-1"
          @click="loadDirectory('/')"
        >
          root/
        </button>
        <template v-for="(seg, idx) in pathSegments" :key="seg.fullPath">
          <span class="text-gray-600">/</span>
          <button
            class="hover:underline font-semibold shrink-0 px-1"
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
          class="btn-secondary py-1 text-xs flex items-center gap-1"
          :disabled="loading"
          @click="loadDirectory(currentPath)"
        >
          <span>🔄</span>
          <span class="hidden sm:inline">Làm mới</span>
        </button>
        <button
          class="btn-primary py-1 text-xs flex items-center gap-1"
          @click="createType = 'file'; showCreateModal = true"
        >
          <span>+ Tệp</span>
        </button>
        <button
          class="btn-secondary py-1 text-xs flex items-center gap-1"
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
          <div>
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
            class="btn-primary py-1 text-xs flex items-center gap-1"
            :disabled="savingFile"
            @click="saveCurrentFile"
          >
            <span>💾</span>
            <span>{{ savingFile ? 'Đang lưu...' : 'Lưu Tệp' }}</span>
          </button>
          <button
            class="btn-secondary py-1 text-xs"
            @click="openEditor(editingFile.path)"
          >
            🔄 Tải lại
          </button>
          <button
            class="btn-danger py-1 text-xs"
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
      <div class="px-4 py-2 bg-black/20 border-b border-white/10 flex items-center justify-between text-xs text-gray-400">
        <span>Danh mục: {{ currentPath }} ({{ filteredEntries.length }} mục)</span>
        <span v-if="loading" class="text-sky-400 animate-pulse">Đang tải dữ liệu...</span>
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
              <td class="py-2 px-3 cursor-pointer" @click="openEntry(entry)">
                <div class="font-medium text-gray-200 group-hover:text-sky-400 transition flex items-center gap-1.5">
                  <span>{{ entry.name }}</span>
                  <span v-if="entry.is_dir" class="text-gray-500 text-[10px]">/</span>
                </div>
              </td>
              <td class="py-2 px-3 mono text-gray-400">
                {{ entry.is_dir ? '—' : formatBytes(entry.size) }}
              </td>
              <td class="py-2 px-3 mono text-[11px] text-gray-500">
                {{ entry.mode }}
              </td>
              <td class="py-2 px-3 text-gray-500 hidden md:table-cell">
                {{ entry.mod_time }}
              </td>
              <td class="py-2 px-4 text-right">
                <div class="flex items-center justify-end gap-1">
                  <button
                    v-if="!entry.is_dir && entry.is_editable"
                    class="px-2 py-0.5 rounded bg-sky-500/20 text-sky-300 hover:bg-sky-500/30 font-semibold text-[11px]"
                    title="Chỉnh sửa tệp text/code trực tiếp"
                    @click="openEditor(entry.path)"
                  >
                    ✏️ Sửa
                  </button>
                  <button
                    v-if="!entry.is_dir"
                    class="px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300 hover:bg-emerald-500/30 font-semibold text-[11px]"
                    title="Tải về máy tính"
                    @click="downloadFile(entry.path)"
                  >
                    ⬇️ Tải
                  </button>
                  <button
                    class="px-2 py-0.5 rounded bg-rose-500/10 text-rose-400 hover:bg-rose-500/20 font-semibold text-[11px]"
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
  </div>
</template>
