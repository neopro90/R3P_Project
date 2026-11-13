<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useSystemStore } from '@/stores/system'
import { apiRequest } from '@/services/api'

interface SystemVersionInfo {
  version: string
  revision: number
  released_at: string
  repo_url: string
  branch: string
  commit_sha: string
  changelog: string
}

interface StagedFirmwareInfo {
  present: boolean
  filename: string
  path: string
  size_bytes: number
  sha256: string
  format_type: string
  valid: boolean
  check_msg: string
  uploaded_at: string
}

interface GitHubCheckResult {
  reachable: boolean
  repo_url: string
  owner: string
  repo: string
  branch: string
  current_version: string
  current_revision: number
  remote_version: string
  remote_revision: number
  update_available: boolean
  released_at: string
  commit_sha: string
  commit_message: string
  commit_date: string
  changelog: string
  bundle_url: string
  sysupgrade_url: string
  message: string
  logs: string[]
}

const system = useSystemStore()
const logs = ref<string[]>([])
const logFilter = ref('')
const loadingLogs = ref(false)
const rebootConfirm = ref(false)
const rebooting = ref(false)
const notice = ref<string | null>(null)
const errorMsg = ref<string | null>(null)

const editHostname = ref('R3P-Router')
const editTimezone = ref('ICT-7')
const savingSettings = ref(false)

// Firmware & GitHub Update State
const currentVer = ref<SystemVersionInfo>({
  version: 'v1.0.0',
  revision: 1,
  released_at: '2026-11-06',
  repo_url: 'https://github.com/neopro90/R3P_Project.git',
  branch: 'main',
  commit_sha: 'v1.0.0',
  changelog: '',
})
const stagedFw = ref<StagedFirmwareInfo>({
  present: false,
  filename: '',
  path: '',
  size_bytes: 0,
  sha256: '',
  format_type: '',
  valid: false,
  check_msg: '',
  uploaded_at: '',
})
const fwLogs = ref<string[]>([])
const ghCheck = ref<GitHubCheckResult | null>(null)

const checkingGh = ref(false)
const updatingFromGh = ref(false)
const uploadingFw = ref(false)
const applyingFw = ref(false)
const keepSettings = ref(true)
const forceFlash = ref(false)
const selectedFwFile = ref<File | null>(null)

const filteredLogs = computed(() => {
  const q = logFilter.value.trim().toLowerCase()
  if (!q) return logs.value
  return logs.value.filter((l) => l.toLowerCase().includes(q))
})

async function fetchFirmwareStatus() {
  try {
    const res = await apiRequest<{
      current: SystemVersionInfo
      staged: StagedFirmwareInfo
      logs: string[]
    }>('/system/firmware/status')
    if (res?.current) currentVer.value = res.current
    if (res?.staged) stagedFw.value = res.staged
    if (res?.logs) fwLogs.value = res.logs
  } catch (err: any) {
    errorMsg.value = 'Lỗi đọc trạng thái Firmware: ' + (err?.message || err)
  }
}

async function saveGitHubRepoConfig() {
  errorMsg.value = null
  try {
    const res = await apiRequest<{ current: SystemVersionInfo; message: string }>('/system/firmware/config', {
      method: 'PUT',
      body: JSON.stringify({
        repo_url: currentVer.value.repo_url.trim(),
        branch: currentVer.value.branch.trim() || 'main',
      }),
    })
    if (res.current) currentVer.value = res.current
    notice.value = res.message || 'Đã lưu cấu hình URL GitHub cập nhật!'
  } catch (err: any) {
    errorMsg.value = 'Lỗi lưu cấu hình GitHub: ' + (err?.message || err)
  }
}

async function checkUpdateFromGitHub() {
  checkingGh.value = true
  errorMsg.value = null
  notice.value = null
  try {
    await saveGitHubRepoConfig()
    const params = new URLSearchParams({
      repo_url: currentVer.value.repo_url.trim(),
      branch: currentVer.value.branch.trim() || 'main',
    })
    const res = await apiRequest<GitHubCheckResult>(`/system/firmware/github-check?${params.toString()}`)
    ghCheck.value = res
    if (res.logs) fwLogs.value = res.logs
    notice.value = res.message
  } catch (err: any) {
    errorMsg.value = 'Lỗi kiểm tra cập nhật GitHub: ' + (err?.message || err)
  } finally {
    checkingGh.value = false
  }
}

async function triggerGitHubUpdate(mode: 'os_bundle' | 'sysupgrade') {
  updatingFromGh.value = true
  errorMsg.value = null
  notice.value = mode === 'os_bundle'
    ? 'Đang tải và cập nhật trực tuyến R3P OS Bundle từ GitHub...'
    : 'Đang tải Firmware Sysupgrade (.bin) từ GitHub...'
  try {
    const res = await apiRequest<{
      current?: SystemVersionInfo
      version?: SystemVersionInfo
      staged?: StagedFirmwareInfo
      logs?: string[]
      message: string
    }>('/system/firmware/github-update', {
      method: 'POST',
      body: JSON.stringify({
        repo_url: currentVer.value.repo_url.trim(),
        branch: currentVer.value.branch.trim() || 'main',
        mode,
        keep_settings: keepSettings.value,
      }),
    })
    if (res.current) currentVer.value = res.current
    else if (res.version) currentVer.value = res.version
    if (res.staged) {
      stagedFw.value = res.staged
      forceFlash.value = false
    }
    if (res.logs) fwLogs.value = res.logs
    notice.value = res.message
    await fetchFirmwareStatus()
  } catch (err: any) {
    errorMsg.value = 'Lỗi cập nhật từ GitHub: ' + (err?.message || err)
  } finally {
    updatingFromGh.value = false
  }
}

function onFileSelected(ev: Event) {
  const input = ev.target as HTMLInputElement
  if (input.files && input.files.length > 0) {
    selectedFwFile.value = input.files[0] ?? null
  }
}

async function uploadManualFirmware() {
  if (!selectedFwFile.value) return
  uploadingFw.value = true
  errorMsg.value = null
  forceFlash.value = false
  notice.value = `Đang tải file '${selectedFwFile.value.name}' lên bộ nhớ RAM (/tmp) của Router...`
  try {
    const formData = new FormData()
    formData.append('file', selectedFwFile.value)
    const data = await apiRequest<{
      staged?: StagedFirmwareInfo
      logs?: string[]
      message?: string
    }>('/system/firmware/upload', {
      method: 'POST',
      body: formData,
    })
    if (data?.staged) stagedFw.value = data.staged
    if (data?.logs) fwLogs.value = data.logs
    notice.value = data?.message || 'Tải lên và kiểm tra file Firmware thành công!'
  } catch (err: any) {
    errorMsg.value = 'Lỗi tải lên Firmware: ' + (err?.message || err)
  } finally {
    uploadingFw.value = false
  }
}

async function applyStagedFirmware() {
  if (!stagedFw.value.valid && !forceFlash.value) {
    errorMsg.value = 'File Firmware chưa vượt qua kiểm tra hợp lệ. Vui lòng tích xác nhận Force Flash nếu thực sự muốn tiếp tục.'
    return
  }
  applyingFw.value = true
  errorMsg.value = null
  notice.value = 'Đang thực thi cập nhật Firmware / OS... Vui lòng chờ hệ thống khởi động lại dịch vụ!'
  try {
    const res = await apiRequest<{
      mode: string
      logs: string[]
      message: string
    }>('/system/firmware/apply', {
      method: 'POST',
      body: JSON.stringify({
        keep_settings: keepSettings.value,
        force: forceFlash.value,
      }),
    })
    if (res.logs) fwLogs.value = res.logs
    notice.value = res.message
    await fetchFirmwareStatus()
  } catch (err: any) {
    errorMsg.value = 'Lỗi áp dụng bản cập nhật: ' + (err?.message || err)
  } finally {
    applyingFw.value = false
  }
}

async function fetchLogs() {
  loadingLogs.value = true
  try {
    const res = await apiRequest<{ lines: string[] }>('/logs')
    if (res?.lines) logs.value = res.lines.slice().reverse()
  } catch (err: any) {
    errorMsg.value = 'Log read error: ' + (err?.message || err)
  } finally {
    loadingLogs.value = false
  }
}

const clearingLogs = ref(false)
async function clearAllLogs() {
  if (!confirm('Bạn có chắc chắn muốn xóa sạch toàn bộ bộ đệm nhật ký hệ thống (logread & dmesg)?')) return
  clearingLogs.value = true
  try {
    await apiRequest('/logs', { method: 'DELETE' })
    notice.value = 'Đã xóa sạch bộ đệm nhật ký hệ thống thành công!'
    logs.value = []
    await fetchLogs()
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    errorMsg.value = 'Lỗi xóa log: ' + (err?.message || err)
  } finally {
    clearingLogs.value = false
  }
}

async function saveSystemSettings() {
  savingSettings.value = true
  try {
    await apiRequest('/system/settings', {
      method: 'PUT',
      body: JSON.stringify({
        hostname: editHostname.value.trim(),
        timezone: editTimezone.value.trim(),
      }),
    })
    await system.refresh()
    notice.value = `System hostname updated to '${editHostname.value}'!`
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    errorMsg.value = 'Settings error: ' + (err?.message || err)
  } finally {
    savingSettings.value = false
  }
}

async function flushRam() {
  try {
    await apiRequest('/system/drop-caches', { method: 'POST' })
    await system.refresh()
    notice.value = 'Kernel pagecache, dentries and inodes flushed!'
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    errorMsg.value = 'Error: ' + (err?.message || err)
  }
}

async function triggerReboot() {
  rebooting.value = true
  notice.value = 'Router is rebooting now… Please wait ~45 seconds to reconnect.'
  try {
    await apiRequest('/system/reboot', { method: 'POST' })
  } catch {
    // Expected connection drop during reboot
  }
}

// ─── Config Backup & Restore ────────────────────────────────────────────────
const restoringConfig = ref(false)
const restoreFileInput = ref<HTMLInputElement | null>(null)
const restoreModalOpen = ref(false)
const selectedRestoreFile = ref<File | null>(null)
const restoreReboot = ref(true)

function openRestorePicker() {
  restoreFileInput.value?.click()
}

function handleRestoreFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.files && target.files.length > 0) {
    selectedRestoreFile.value = target.files[0]
    restoreModalOpen.value = true
  }
}

async function executeRestore() {
  if (!selectedRestoreFile.value) return
  restoringConfig.value = true
  notice.value = `Đang tải file '${selectedRestoreFile.value.name}' và khôi phục cấu hình hệ thống...`
  errorMsg.value = null

  try {
    const formData = new FormData()
    formData.append('file', selectedRestoreFile.value)
    formData.append('reboot', restoreReboot.value ? 'true' : 'false')

    const resp = await fetch('/api/v1/system/restore', {
      method: 'POST',
      credentials: 'include',
      body: formData,
    })

    const payload = await resp.json()
    if (!resp.ok || !payload.ok) {
      throw new Error(payload.error || payload.data || `HTTP ${resp.status}`)
    }

    restoreModalOpen.value = false
    selectedRestoreFile.value = null
    if (restoreFileInput.value) restoreFileInput.value.value = ''

    if (payload.data?.reboot) {
      rebooting.value = true
      notice.value = payload.data?.message || 'Khôi phục thành công! Router đang khởi động lại...'
    } else {
      notice.value = payload.data?.message || 'Khôi phục cấu hình thành công!'
      setTimeout(() => { notice.value = null }, 5000)
      await system.refresh()
    }
  } catch (err: any) {
    errorMsg.value = 'Lỗi khôi phục cấu hình: ' + (err?.message || err)
  } finally {
    restoringConfig.value = false
  }
}

onMounted(async () => {
  await system.refresh()
  if (system.hostname && system.hostname !== '—') {
    editHostname.value = system.hostname
  }
  await Promise.all([fetchFirmwareStatus(), fetchLogs()])
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Floating Toast Notifications (Never shift layout) -->
    <div v-if="errorMsg" class="r3p-toast r3p-toast-error">
      <div class="flex items-start justify-between gap-3">
        <span class="r3p-toast-text">{{ errorMsg }}</span>
        <button type="button" class="text-rose-300 hover:text-white shrink-0 font-bold" @click="errorMsg = null">✕</button>
      </div>
    </div>
    <div v-else-if="notice" class="r3p-toast r3p-toast-success">
      <div class="flex items-start justify-between gap-3">
        <span class="r3p-toast-text">{{ notice }}</span>
        <button type="button" class="text-emerald-300 hover:text-white shrink-0 font-bold" @click="notice = null">✕</button>
      </div>
    </div>

    <!-- 1. FIRMWARE UPDATE CENTER (ONLINE VIA GITHUB URL & MANUAL UPLOAD) -->
    <div class="pro-card border border-sky-500/30">
      <div class="pro-card-header flex-wrap gap-2">
        <div class="min-w-0">
          <span class="pro-card-title text-sky-400">🚀 Trung Tâm Cập Nhật Firmware Thủ Công & Cập Nhật Trực Tuyến Qua GitHub</span>
          <p class="text-[11px] text-gray-400 mt-0.5 truncate">
            Phiên bản hiện tại: <strong class="mono text-emerald-400">{{ currentVer.version }} (Revision #{{ currentVer.revision }})</strong> ·
            Kho GitHub: <code class="mono text-sky-300">{{ currentVer.repo_url }}</code>
          </p>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <span class="badge badge-green mono">{{ currentVer.version }} · REV {{ currentVer.revision }}</span>
          <span class="badge badge-blue mono">{{ currentVer.released_at }}</span>
        </div>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-3 items-start">
        <!-- Left Column: Online Update via GitHub URL -->
        <div class="p-3 rounded-lg bg-black/40 border border-white/10 flex flex-col justify-between gap-2.5 min-w-0">
          <div>
            <div class="flex items-center justify-between mb-2 gap-2">
              <span class="text-xs font-bold text-sky-300 uppercase truncate">🌐 Cập Nhật Qua URL Từ GitHub Repository</span>
              <span class="badge badge-purple">OTA / GITHUB SYNC</span>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-4 gap-2 mb-2.5">
              <div class="sm:col-span-3">
                <label class="block text-[10px] text-gray-400 uppercase mb-1">GitHub Repository URL</label>
                <input
                  v-model="currentVer.repo_url"
                  type="text"
                  class="pro-input w-full mono text-xs"
                  placeholder="https://github.com/neopro90/R3P_Project.git"
                />
              </div>
              <div>
                <label class="block text-[10px] text-gray-400 uppercase mb-1">Nhánh (Branch)</label>
                <input
                  v-model="currentVer.branch"
                  type="text"
                  class="pro-input w-full mono text-xs"
                  placeholder="main"
                />
              </div>
            </div>

            <!-- GitHub Check Result Box -->
            <div v-if="ghCheck" class="p-2.5 rounded bg-black/50 border border-sky-500/25 text-[11px] space-y-1 mb-2">
              <div class="flex items-center justify-between gap-2">
                <span class="text-gray-400">Trạng thái kết nối GitHub:</span>
                <span :class="ghCheck.reachable ? 'badge badge-green' : 'badge badge-orange'">
                  {{ ghCheck.reachable ? 'KẾT NỐI THÀNH CÔNG' : 'CHƯA KẾT NỐI / REPO TRỐNG' }}
                </span>
              </div>
              <div class="flex items-center justify-between gap-2">
                <span class="text-gray-400">Phiên bản trên máy / Trên GitHub:</span>
                <span class="mono font-bold text-gray-200">
                  {{ ghCheck.current_version }} (Rev {{ ghCheck.current_revision }}) →
                  <span :class="ghCheck.update_available ? 'text-emerald-400' : 'text-sky-300'">
                    {{ ghCheck.remote_version }} (Rev {{ ghCheck.remote_revision }})
                  </span>
                </span>
              </div>
              <div v-if="ghCheck.commit_sha" class="flex items-center justify-between gap-2">
                <span class="text-gray-400 shrink-0">Commit mới nhất:</span>
                <span class="mono text-purple-300 truncate">{{ ghCheck.commit_sha }} — {{ ghCheck.commit_message }}</span>
              </div>
              <div v-if="ghCheck.changelog" class="text-[10.5px] text-gray-300 pt-1 border-t border-white/5 break-words">
                📝 <strong>Changelog:</strong> {{ ghCheck.changelog }}
              </div>
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-2 pt-2 border-t border-white/10">
            <button
              type="button"
              class="btn-secondary text-xs min-w-[216px] justify-center"
              :disabled="checkingGh || updatingFromGh"
              @click="checkUpdateFromGitHub"
            >
              {{ checkingGh ? 'Đang kiểm tra GitHub…' : '🔍 Kiểm Tra Bản Mới Trên GitHub' }}
            </button>
            <button
              type="button"
              class="btn-primary text-xs min-w-[234px] justify-center"
              :disabled="updatingFromGh"
              @click="triggerGitHubUpdate('os_bundle')"
            >
              {{ updatingFromGh ? 'Đang tải & cập nhật…' : '⚡ Cập Nhật R3P OS Từ GitHub Ngay' }}
            </button>
            <button
              type="button"
              class="btn-secondary text-xs min-w-[196px] justify-center"
              :disabled="updatingFromGh"
              @click="triggerGitHubUpdate('sysupgrade')"
            >
              📦 Tải Firmware .bin Từ GitHub
            </button>
          </div>
        </div>

        <!-- Right Column: Manual Firmware Upload (.bin / .tar.gz / r3p-api) -->
        <div class="p-3 rounded-lg bg-black/40 border border-white/10 flex flex-col justify-between gap-2.5 min-w-0">
          <div>
            <div class="flex items-center justify-between mb-2 gap-2">
              <span class="text-xs font-bold text-emerald-300 uppercase truncate">📁 Cập Nhật Firmware Thủ Công (Upload File)</span>
              <span class="badge badge-blue">SYSUPGRADE / OS BUNDLE</span>
            </div>

            <p class="text-[11px] text-gray-400 mb-2">
              Hỗ trợ tải lên file <code class="mono text-sky-300">openwrt-*-sysupgrade.bin</code>, gói cập nhật <code class="mono text-emerald-300">r3p-update-bundle.tar.gz</code> hoặc binary <code class="mono">r3p-api</code>. Hệ thống sẽ tự động kiểm tra mã băm SHA256 và <code class="mono">sysupgrade -T</code>.
            </p>

            <div class="flex items-center gap-2 mb-2.5">
              <input
                type="file"
                accept=".bin,.tar.gz,.tgz,.ipk,.apk,*"
                class="pro-input flex-1 text-xs min-w-0 file:mr-2 file:py-1 file:px-2.5 file:rounded file:border-0 file:text-xs file:bg-sky-500/20 file:text-sky-300 hover:file:bg-sky-500/30"
                @change="onFileSelected"
              />
              <button
                type="button"
                class="btn-primary text-xs whitespace-nowrap min-w-[148px] justify-center shrink-0"
                :disabled="!selectedFwFile || uploadingFw"
                @click="uploadManualFirmware"
              >
                {{ uploadingFw ? 'Đang tải lên…' : '⬆ Tải Lên & Kiểm Tra' }}
              </button>
            </div>

            <!-- Staged Firmware Verification Card -->
            <div v-if="stagedFw.present" class="p-2.5 rounded bg-emerald-500/10 border border-emerald-500/30 text-[11px] space-y-1">
              <div class="flex items-center justify-between gap-2">
                <span class="font-bold text-emerald-300 mono truncate">{{ stagedFw.filename }}</span>
                <span :class="stagedFw.valid ? 'badge badge-green' : 'badge badge-orange'">
                  {{ stagedFw.valid ? 'VERIFIED OK' : 'CHECK WARNING' }}
                </span>
              </div>
              <div class="flex justify-between text-gray-300">
                <span>Định dạng: <strong class="mono text-sky-300">{{ stagedFw.format_type }}</strong></span>
                <span>Dung lượng: <strong class="mono">{{ (stagedFw.size_bytes / (1024 * 1024)).toFixed(2) }} MB</strong></span>
              </div>
              <div class="mono text-[10px] text-gray-400 truncate" :title="stagedFw.sha256">
                SHA256: {{ stagedFw.sha256 }}
              </div>
              <div class="text-[10.5px] text-gray-300 break-words">{{ stagedFw.check_msg }}</div>
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-white/10">
            <div class="space-y-1">
              <label class="flex items-center gap-1.5 text-xs text-gray-300 cursor-pointer">
                <input v-model="keepSettings" type="checkbox" class="rounded accent-sky-500" />
                <span>Giữ lại toàn bộ cấu hình hiện tại (<code class="mono text-sky-300">sysupgrade -k</code>)</span>
              </label>
              <label v-if="stagedFw.present && !stagedFw.valid" class="flex items-center gap-1.5 text-xs text-amber-300 cursor-pointer">
                <input v-model="forceFlash" type="checkbox" class="rounded accent-amber-500" />
                <span>⚠️ Kiểm tra thất bại — xác nhận Force Flash / Bỏ qua cảnh báo</span>
              </label>
            </div>
            <button
              type="button"
              class="btn-danger text-xs px-3.5 min-w-[216px] justify-center"
              :disabled="!stagedFw.present || applyingFw || (!stagedFw.valid && !forceFlash)"
              @click="applyStagedFirmware"
            >
              {{ applyingFw ? 'Đang nạp Firmware…' : '🔥 Áp Dụng / Flash Firmware Ngay' }}
            </button>
          </div>
        </div>
      </div>

      <!-- Firmware Update Activity Logs -->
      <div v-if="fwLogs.length > 0" class="mt-2.5 p-2 rounded bg-black/60 border border-white/5 font-mono text-[10.5px] text-gray-300 max-h-28 overflow-y-auto">
        <div v-for="(line, i) in fwLogs" :key="i" class="leading-relaxed">{{ line }}</div>
      </div>
    </div>

    <!-- 2. Hardware, OS & System Settings Grid -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-3 items-start">
      <!-- Hardware Specs -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Hardware Specifications</span>
          <span class="badge badge-blue">MT7621AT</span>
        </div>

        <div class="space-y-1.5 text-[11.5px]">
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Board Model</span>
            <span class="font-medium text-gray-200">Xiaomi Mi Router 3 Pro (R3P)</span>
          </div>
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">CPU SoC</span>
            <span class="mono text-gray-200">MT7621AT 880MHz (2C/4T)</span>
          </div>
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">RAM Memory</span>
            <span class="mono text-emerald-400">{{ (system.ramUsed || 0).toFixed(0) }} / {{ (system.ramTotal || 512).toFixed(0) }} MB ({{ (system.ram || 0).toFixed(1) }}%)</span>
          </div>
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Flash Chip</span>
            <span class="mono text-purple-400">ESMT F59L2G81A 256MB SLC</span>
          </div>
          <div class="flex justify-between py-1">
            <span class="text-gray-400">Wi-Fi / Switch</span>
            <span class="mono text-gray-300">MT7615N 4x4 · MT7530 GbE</span>
          </div>
        </div>
      </div>

      <!-- OS & Kernel Info -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Operating System & Kernel</span>
          <span class="badge badge-green">RUNNING</span>
        </div>

        <div class="space-y-1.5 text-[11.5px]">
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">R3P OS Revision</span>
            <span class="font-semibold text-emerald-400 mono">{{ currentVer.version }} (Rev {{ currentVer.revision }})</span>
          </div>
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Distribution</span>
            <span class="font-semibold text-sky-400">OpenWrt {{ system.firmwareVersion || '—' }}</span>
          </div>
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Linux Kernel</span>
            <span class="mono text-emerald-400">{{ system.kernel || '—' }}</span>
          </div>
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Load Average</span>
            <span class="mono text-gray-300">{{ system.loadAvg.map(n => n.toFixed(2)).join(' / ') }}</span>
          </div>
          <div class="flex justify-between py-1">
            <span class="text-gray-400">System Uptime</span>
            <span class="mono text-amber-400">{{ system.uptimeFormatted || '—' }}</span>
          </div>
        </div>
      </div>

      <!-- Editable System Identity -->
      <form class="pro-card flex flex-col justify-between" @submit.prevent="saveSystemSettings">
        <div>
          <div class="pro-card-header">
            <span class="pro-card-title">System Identity & Timezone</span>
            <span class="badge badge-purple">UCI SYSTEM</span>
          </div>

          <div class="space-y-2 text-[12px]">
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Router Hostname</label>
              <input v-model="editHostname" type="text" required class="w-full pro-input mono" />
            </div>
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Múi Giờ Hệ Thống (Timezone POSIX)</label>
              <select v-model="editTimezone" class="w-full pro-input mono">
                <option value="ICT-7">Asia/Ho_Chi_Minh (GMT+7 · Việt Nam / Indochina)</option>
                <option value="UTC">UTC (GMT+0 · Giờ Quốc Tế Phối Hợp)</option>
                <option value="CST-8">Asia/Singapore / Thượng Hải (GMT+8)</option>
                <option value="JST-9">Asia/Tokyo (GMT+9)</option>
              </select>
            </div>
          </div>
        </div>

        <div class="mt-3 pt-2 border-t border-white/5 flex justify-end">
          <button type="submit" :disabled="savingSettings" class="btn-primary text-[11px] min-w-[164px] justify-center">
            {{ savingSettings ? 'Applying…' : 'Save Hostname & Timezone' }}
          </button>
        </div>
      </form>
    </div>

    <!-- 3. Power, Backup & Memory Maintenance -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Maintenance, Backup & Power Controls</span>
      </div>

      <div class="flex flex-wrap items-center gap-2.5">
        <button type="button" class="btn-secondary text-[11.5px]" @click="flushRam">
          Flush Memory Cache (drop_caches)
        </button>

        <a
          href="/api/v1/system/backup"
          download="r3p-backup.tar.gz"
          class="btn-primary text-[11.5px]"
        >
          ↓ Download Config Backup (tar.gz)
        </a>

        <!-- Restore Config Button & Hidden File Input -->
        <input
          ref="restoreFileInput"
          type="file"
          accept=".tar.gz,.gz,application/gzip,application/x-gzip"
          class="hidden"
          @change="handleRestoreFileChange"
        />
        <button
          type="button"
          class="btn-secondary text-[11.5px] border-amber-500/40 text-amber-300 hover:bg-amber-500/10 flex items-center gap-1.5 cursor-pointer font-medium min-w-[208px] justify-center"
          :disabled="restoringConfig"
          @click="openRestorePicker"
        >
          <span>↑</span>
          <span>{{ restoringConfig ? 'Đang nạp file...' : 'Restore Config Backup (.tar.gz)' }}</span>
        </button>

        <button type="button" class="btn-secondary text-[11.5px] min-w-[144px] justify-center" :disabled="loadingLogs" @click="fetchLogs">
          {{ loadingLogs ? 'Reading Logs…' : 'Refresh System Log ↻' }}
        </button>

        <button
          type="button"
          class="px-3 py-1.5 rounded bg-red-500/20 hover:bg-red-500/30 text-red-300 border border-red-500/40 font-semibold text-[11.5px] transition cursor-pointer min-w-[146px] text-center"
          :disabled="clearingLogs"
          @click="clearAllLogs"
        >
          {{ clearingLogs ? 'Đang xóa...' : '🗑 Xóa Sạch Log (Clear)' }}
        </button>

        <button
          v-if="!rebootConfirm"
          type="button"
          class="btn-danger text-[11.5px] ml-auto min-w-[120px] justify-center"
          @click="rebootConfirm = true"
        >
          Reboot Router…
        </button>

        <div v-else class="flex items-center gap-2 bg-red-500/20 border border-red-500/30 px-3 py-1 rounded ml-auto">
          <span class="text-[11.5px] text-red-200 font-semibold">Confirm hardware reboot?</span>
          <button
            type="button"
            :disabled="rebooting"
            class="btn-danger text-[11px] py-0.5 px-2 min-w-[104px] justify-center"
            @click="triggerReboot"
          >
            {{ rebooting ? 'Rebooting…' : 'Yes, Reboot Now' }}
          </button>
          <button
            type="button"
            class="btn-secondary text-[11px] py-0.5 px-2"
            @click="rebootConfirm = false"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>

    <!-- Modal Xác Nhận Restore Config -->
    <div v-if="restoreModalOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
      <div class="bg-gray-900 border border-amber-500/50 rounded-xl max-w-md w-full p-5 shadow-2xl space-y-4">
        <div class="flex items-center gap-2 text-amber-400 font-bold text-base">
          <span class="text-lg">⚠️</span>
          <span>Khôi Phục Cấu Hình Router</span>
        </div>
        <div class="text-xs text-gray-300 space-y-2">
          <p>Bạn sắp nạp lại toàn bộ cấu hình từ file backup:</p>
          <div class="px-3 py-2 rounded bg-black/60 border border-gray-700 font-mono text-emerald-400 text-[11.5px] break-all">
            {{ selectedRestoreFile?.name }} ({{ ((selectedRestoreFile?.size || 0) / 1024).toFixed(1) }} KB)
          </div>
          <p class="text-gray-400 leading-relaxed">
            Hệ thống sẽ nạp lại mật khẩu root, mạng Wi-Fi, tường lửa, DNS, DuckDNS và cơ sở dữ liệu trợ lý AI từ bản sao lưu này.
          </p>
        </div>
        <div class="flex items-center gap-2 pt-2 border-t border-gray-800">
          <input id="restoreRebootCheck" v-model="restoreReboot" type="checkbox" class="accent-sky-500 rounded cursor-pointer" />
          <label for="restoreRebootCheck" class="text-xs text-gray-300 cursor-pointer select-none font-medium">
            Khởi động lại Router ngay sau khi nạp (Khuyên dùng)
          </label>
        </div>
        <div class="flex justify-end gap-2.5 pt-2">
          <button
            type="button"
            class="btn-secondary text-xs px-3 py-1.5"
            :disabled="restoringConfig"
            @click="restoreModalOpen = false; selectedRestoreFile = null"
          >
            Hủy Bỏ
          </button>
          <button
            type="button"
            class="px-4 py-1.5 rounded bg-amber-600 hover:bg-amber-500 text-white text-xs font-semibold transition cursor-pointer shadow"
            :disabled="restoringConfig"
            @click="executeRestore"
          >
            {{ restoringConfig ? 'Đang Nạp File...' : 'Xác Nhận Khôi Phục' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 4. Kernel & System Log Terminal -->
    <div class="pro-card">
      <div class="pro-card-header flex-wrap gap-2">
        <div class="flex items-center gap-2">
          <span class="pro-card-title">System & Kernel Log Buffer (/sbin/logread · {{ filteredLogs.length }} dòng)</span>
          <span class="text-[11px] text-gray-400 hidden sm:inline">(Mới nhất ở trên cùng)</span>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="px-2.5 py-1 text-[11px] rounded bg-red-500/20 hover:bg-red-500/30 text-red-300 border border-red-500/40 transition font-semibold cursor-pointer"
            :disabled="clearingLogs"
            @click="clearAllLogs"
          >
            {{ clearingLogs ? 'Đang xóa...' : '🗑 Xóa Log' }}
          </button>
          <input
            v-model="logFilter"
            type="search"
            class="pro-input w-56 mono text-[11px]"
            placeholder="Tìm kiếm dòng log…"
          />
        </div>
      </div>

      <div class="bg-black/60 rounded border border-white/5 p-2 font-mono text-[10.5px] text-gray-300 h-64 overflow-y-auto space-y-0.5">
        <div v-if="loadingLogs" class="text-gray-500">Reading /sbin/logread stream…</div>
        <div v-else-if="!filteredLogs.length" class="text-gray-500">No matching log lines.</div>
        <div v-for="(l, idx) in filteredLogs" :key="idx" class="leading-tight hover:bg-white/[0.04] px-1 rounded">
          <span class="text-sky-500 select-none">›</span> {{ l }}
        </div>
      </div>
    </div>
  </div>
</template>
