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
    if (res.version) currentVer.value = res.version
    if (res.staged) stagedFw.value = res.staged
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
  notice.value = `Đang tải file '${selectedFwFile.value.name}' lên bộ nhớ RAM (/tmp) của Router...`
  try {
    const resp = await fetch(
      `/api/v1/system/firmware/upload?filename=${encodeURIComponent(selectedFwFile.value.name)}`,
      {
        method: 'POST',
        credentials: 'include',
        headers: {
          'X-Filename': selectedFwFile.value.name,
        },
        body: selectedFwFile.value,
      },
    )
    const payload = await resp.json()
    if (!resp.ok || !payload.ok) {
      throw new Error(payload.error || payload.data || `HTTP ${resp.status}`)
    }
    if (payload.data?.staged) stagedFw.value = payload.data.staged
    if (payload.data?.logs) fwLogs.value = payload.data.logs
    notice.value = payload.data?.message || 'Tải lên và kiểm tra file Firmware thành công!'
  } catch (err: any) {
    errorMsg.value = 'Lỗi tải lên Firmware: ' + (err?.message || err)
  } finally {
    uploadingFw.value = false
  }
}

async function applyStagedFirmware() {
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
        force: true,
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
    <div v-if="notice" class="px-3 py-1.5 rounded bg-emerald-500/15 border border-emerald-500/30 text-emerald-300 text-[11.5px] font-medium flex items-center justify-between">
      <span>{{ notice }}</span>
      <button class="text-xs text-gray-400 hover:text-white" @click="notice = null">✕</button>
    </div>
    <div v-if="errorMsg" class="px-3 py-1.5 rounded bg-red-500/15 border border-red-500/30 text-red-300 text-[11.5px] font-medium flex items-center justify-between">
      <span>{{ errorMsg }}</span>
      <button class="text-xs text-gray-400 hover:text-white" @click="errorMsg = null">✕</button>
    </div>

    <!-- 1. FIRMWARE UPDATE CENTER (ONLINE VIA GITHUB URL & MANUAL UPLOAD) -->
    <div class="pro-card border border-sky-500/30">
      <div class="pro-card-header flex-wrap gap-2">
        <div>
          <span class="pro-card-title text-sky-400">🚀 Trung Tâm Cập Nhật Firmware Thủ Công & Cập Nhật Trực Tuyến Qua GitHub</span>
          <p class="text-[11px] text-gray-400 mt-0.5">
            Phiên bản hiện tại: <strong class="mono text-emerald-400">{{ currentVer.version }} (Revision #{{ currentVer.revision }})</strong> ·
            Kho GitHub: <code class="mono text-sky-300">{{ currentVer.repo_url }}</code>
          </p>
        </div>
        <div class="flex items-center gap-2">
          <span class="badge badge-green mono">{{ currentVer.version }} · REV {{ currentVer.revision }}</span>
          <span class="badge badge-blue mono">{{ currentVer.released_at }}</span>
        </div>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-3">
        <!-- Left Column: Online Update via GitHub URL -->
        <div class="p-3 rounded-lg bg-black/40 border border-white/10 flex flex-col justify-between gap-2.5">
          <div>
            <div class="flex items-center justify-between mb-2">
              <span class="text-xs font-bold text-sky-300 uppercase">🌐 Cập Nhật Qua URL Từ GitHub Repository</span>
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
              <div class="flex items-center justify-between">
                <span class="text-gray-400">Trạng thái kết nối GitHub:</span>
                <span :class="ghCheck.reachable ? 'badge badge-green' : 'badge badge-orange'">
                  {{ ghCheck.reachable ? 'KẾT NỐI THÀNH CÔNG' : 'CHƯA KẾT NỐI / REPO TRỐNG' }}
                </span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-gray-400">Phiên bản trên máy / Trên GitHub:</span>
                <span class="mono font-bold text-gray-200">
                  {{ ghCheck.current_version }} (Rev {{ ghCheck.current_revision }}) →
                  <span :class="ghCheck.update_available ? 'text-emerald-400' : 'text-sky-300'">
                    {{ ghCheck.remote_version }} (Rev {{ ghCheck.remote_revision }})
                  </span>
                </span>
              </div>
              <div v-if="ghCheck.commit_sha" class="flex items-center justify-between">
                <span class="text-gray-400">Commit mới nhất:</span>
                <span class="mono text-purple-300">{{ ghCheck.commit_sha }} — {{ ghCheck.commit_message }}</span>
              </div>
              <div v-if="ghCheck.changelog" class="text-[10.5px] text-gray-300 pt-1 border-t border-white/5">
                📝 <strong>Changelog:</strong> {{ ghCheck.changelog }}
              </div>
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-2 pt-2 border-t border-white/10">
            <button
              type="button"
              class="btn-secondary text-xs"
              :disabled="checkingGh || updatingFromGh"
              @click="checkUpdateFromGitHub"
            >
              {{ checkingGh ? 'Đang kiểm tra GitHub…' : '🔍 Kiểm Tra Bản Mới Trên GitHub' }}
            </button>
            <button
              type="button"
              class="btn-primary text-xs"
              :disabled="updatingFromGh"
              @click="triggerGitHubUpdate('os_bundle')"
            >
              {{ updatingFromGh ? 'Đang tải & cập nhật…' : '⚡ Cập Nhật R3P OS Từ GitHub Ngay' }}
            </button>
            <button
              type="button"
              class="btn-secondary text-xs"
              :disabled="updatingFromGh"
              @click="triggerGitHubUpdate('sysupgrade')"
            >
              📦 Tải Firmware .bin Từ GitHub
            </button>
          </div>
        </div>

        <!-- Right Column: Manual Firmware Upload (.bin / .tar.gz / r3p-api) -->
        <div class="p-3 rounded-lg bg-black/40 border border-white/10 flex flex-col justify-between gap-2.5">
          <div>
            <div class="flex items-center justify-between mb-2">
              <span class="text-xs font-bold text-emerald-300 uppercase">📁 Cập Nhật Firmware Thủ Công (Upload File)</span>
              <span class="badge badge-blue">SYSUPGRADE / OS BUNDLE</span>
            </div>

            <p class="text-[11px] text-gray-400 mb-2">
              Hỗ trợ tải lên file <code class="mono text-sky-300">openwrt-*-sysupgrade.bin</code>, gói cập nhật <code class="mono text-emerald-300">r3p-update-bundle.tar.gz</code> hoặc binary <code class="mono">r3p-api</code>. Hệ thống sẽ tự động kiểm tra mã băm SHA256 và <code class="mono">sysupgrade -T</code>.
            </p>

            <div class="flex items-center gap-2 mb-2.5">
              <input
                type="file"
                accept=".bin,.tar.gz,.tgz,.ipk,.apk,*"
                class="pro-input flex-1 text-xs file:mr-2 file:py-1 file:px-2.5 file:rounded file:border-0 file:text-xs file:bg-sky-500/20 file:text-sky-300 hover:file:bg-sky-500/30"
                @change="onFileSelected"
              />
              <button
                type="button"
                class="btn-primary text-xs whitespace-nowrap"
                :disabled="!selectedFwFile || uploadingFw"
                @click="uploadManualFirmware"
              >
                {{ uploadingFw ? 'Đang tải lên…' : '⬆ Tải Lên & Kiểm Tra' }}
              </button>
            </div>

            <!-- Staged Firmware Verification Card -->
            <div v-if="stagedFw.present" class="p-2.5 rounded bg-emerald-500/10 border border-emerald-500/30 text-[11px] space-y-1">
              <div class="flex items-center justify-between">
                <span class="font-bold text-emerald-300 mono">{{ stagedFw.filename }}</span>
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
              <div class="text-[10.5px] text-gray-300">{{ stagedFw.check_msg }}</div>
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-white/10">
            <label class="flex items-center gap-1.5 text-xs text-gray-300 cursor-pointer">
              <input v-model="keepSettings" type="checkbox" class="rounded accent-sky-500" />
              <span>Giữ lại toàn bộ cấu hình hiện tại (<code class="mono text-sky-300">sysupgrade -k</code>)</span>
            </label>
            <button
              type="button"
              class="btn-danger text-xs px-3.5"
              :disabled="!stagedFw.present || applyingFw"
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
    <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
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
            <span class="font-semibold text-sky-400">OpenWrt {{ system.firmwareVersion || '25.12.5' }}</span>
          </div>
          <div class="flex justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Linux Kernel</span>
            <span class="mono text-emerald-400">{{ system.kernel || '6.12.74' }}</span>
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
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Timezone String</label>
              <select v-model="editTimezone" class="w-full pro-input mono">
                <option value="ICT-7">Asia/Ho_Chi_Minh (ICT-7)</option>
                <option value="UTC">UTC (Coordinated Universal Time)</option>
                <option value="CST-8">Asia/Singapore (UTC+8)</option>
                <option value="JST-9">Asia/Tokyo (UTC+9)</option>
              </select>
            </div>
          </div>
        </div>

        <div class="mt-3 pt-2 border-t border-white/5 flex justify-end">
          <button type="submit" :disabled="savingSettings" class="btn-primary text-[11px]">
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
        <button class="btn-secondary text-[11.5px]" @click="flushRam">
          Flush Memory Cache (drop_caches)
        </button>

        <a
          href="/api/v1/system/backup"
          download="r3p-backup.tar.gz"
          class="btn-primary text-[11.5px]"
        >
          ↓ Download Config Backup (tar.gz)
        </a>

        <button class="btn-secondary text-[11.5px]" :disabled="loadingLogs" @click="fetchLogs">
          {{ loadingLogs ? 'Reading Logs…' : 'Refresh System Log ↻' }}
        </button>

        <button
          v-if="!rebootConfirm"
          class="btn-danger text-[11.5px] ml-auto"
          @click="rebootConfirm = true"
        >
          Reboot Router…
        </button>

        <div v-else class="flex items-center gap-2 bg-red-500/20 border border-red-500/30 px-3 py-1 rounded ml-auto">
          <span class="text-[11.5px] text-red-200 font-semibold">Confirm hardware reboot?</span>
          <button
            :disabled="rebooting"
            class="btn-danger text-[11px] py-0.5 px-2"
            @click="triggerReboot"
          >
            {{ rebooting ? 'Rebooting…' : 'Yes, Reboot Now' }}
          </button>
          <button
            class="btn-secondary text-[11px] py-0.5 px-2"
            @click="rebootConfirm = false"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>

    <!-- 4. Kernel & System Log Terminal -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">System & Kernel Log Buffer (/sbin/logread · {{ filteredLogs.length }} lines)</span>
        <input
          v-model="logFilter"
          type="search"
          class="pro-input w-56 mono text-[11px]"
          placeholder="Search log entries…"
        />
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
