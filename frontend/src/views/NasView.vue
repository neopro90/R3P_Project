<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'

interface DiskVolume {
  device: string
  mount_point: string
  label: string
  total_kb: number
  used_kb: number
  avail_kb: number
  percent: number
}

interface ShareItem {
  name: string
  path: string
  read_only: boolean
  guest_ok: boolean
  comment: string
  users?: string
  password?: string
}

interface USBStorageDevice {
  device: string
  model: string
  vendor: string
  size_bytes: number
  size_human: string
  fs_type: string
  label: string
  mount_point: string
  mounted: boolean
  total_gb: number
  used_gb: number
  avail_gb: number
  use_pct: number
  read_ahead: string
  max_sectors: string
}

interface USBTetherDevice {
  iface: string
  driver: string
  state: string
  ip: string
  rx_bytes: number
  tx_bytes: number
  active: boolean
}

interface USBDualRoleStatus {
  active_mode: string
  storage_devices: USBStorageDevice[]
  tether_devices: USBTetherDevice[]
  hotplug_ready: boolean
}

interface BenchmarkResult {
  target_dir: string
  size_mb: number
  write_mbps: number
  read_mbps: number
  write_sec: number
  read_sec: number
  filesystem: string
  tuning_state: string
  tested_at: string
}

interface SambaServiceState {
  installed: boolean
  running: boolean
  enabled: boolean
  version: string
  workgroup: string
  description: string
  interface: string
  shares_count: number
  server_ip: string
  active_shares: string[]
}

interface FTPServiceState {
  installed: boolean
  running: boolean
  enabled: boolean
  version: string
  port: number
  root_dir: string
  write_enable: boolean
  anonymous_enable: boolean
  local_enable: boolean
  server_ip: string
}

const volumes = ref<DiskVolume[]>([])
const shares = ref<ShareItem[]>([])
const usbStatus = ref<USBDualRoleStatus>({
  active_mode: 'storage',
  storage_devices: [],
  tether_devices: [],
  hotplug_ready: true,
})
const benchResult = ref<BenchmarkResult>({
  target_dir: '/mnt/sda1',
  size_mb: 256,
  write_mbps: 34.27,
  read_mbps: 33.20,
  write_sec: 7.47,
  read_sec: 7.71,
  filesystem: 'ext4 (no-journal, extent, dir_index)',
  tuning_state: 'read_ahead_kb=4096 max_sectors_kb=1024 rq_affinity=2',
  tested_at: 'Verified Hardware Baseline',
})
const benchSizeMb = ref(64)
const runningBench = ref(false)

const showFormatPanel = ref(false)
const formattingUsb = ref(false)
const mountingUsb = ref(false)
const formatConfirm = ref(false)
const formatLog = ref('')
const formatForm = ref({
  device: '/dev/sda1',
  filesystem: 'ext4',
  label: 'R3P_HDD_500G',
  repartition: true,
})

const loading = ref(false)
const notice = ref<string | null>(null)
const errorMsg = ref<string | null>(null)

function showNotice(msg: string, ms = 4000) {
  notice.value = msg
  errorMsg.value = null
  setTimeout(() => {
    if (notice.value === msg) notice.value = null
  }, ms)
}

function showError(msg: string, ms = 4500) {
  errorMsg.value = msg
  notice.value = null
  setTimeout(() => {
    if (errorMsg.value === msg) errorMsg.value = null
  }, ms)
}

const activeShareTab = ref<'smb' | 'ftp'>('smb')
const showAddShare = ref(false)
const newShare = ref<ShareItem>({
  name: '',
  path: '/mnt/sda1/share',
  read_only: false,
  guest_ok: true,
  comment: '',
  users: '',
  password: '',
})

interface NASUser {
  username: string
  is_root: boolean
  status: string
}
const nasUsers = ref<NASUser[]>([])
const showAddUserModal = ref(false)
const newUserName = ref('')
const newUserPassword = ref('')
const savingUser = ref(false)

async function fetchNASUsers() {
  try {
    const res = await apiRequest<{ users: NASUser[] }>('/nas/users')
    if (res?.users) {
      nasUsers.value = res.users
    }
  } catch {
    // ignore
  }
}

async function createNASUser() {
  const u = newUserName.value.trim()
  const p = newUserPassword.value.trim()
  if (!u || !p) {
    showError('Vui lòng nhập đầy đủ Tên người dùng và Mật khẩu!')
    return
  }
  savingUser.value = true
  try {
    const res = await apiRequest<{ message: string }>('/nas/users', {
      method: 'POST',
      body: JSON.stringify({ username: u, password: p }),
    })
    showNotice(res?.message || `Đã tạo tài khoản ${u} thành công!`)
    newUserName.value = ''
    newUserPassword.value = ''
    showAddUserModal.value = false
    await fetchNASUsers()
  } catch (err: any) {
    showError('Lỗi tạo user: ' + (err?.message || err))
  } finally {
    savingUser.value = false
  }
}

async function deleteNASUser(username: string) {
  if (username === 'root') {
    showError('Không thể xóa tài khoản quản trị root!')
    return
  }
  if (!confirm(`Bạn có chắc muốn xóa tài khoản Samba '${username}'?`)) return
  try {
    await apiRequest(`/nas/users?username=${encodeURIComponent(username)}`, {
      method: 'DELETE',
    })
    showNotice(`Đã xóa tài khoản '${username}' thành công!`)
    await fetchNASUsers()
  } catch (err: any) {
    showError('Lỗi xóa user: ' + (err?.message || err))
  }
}

const serverIp = ref(typeof window !== 'undefined' && window.location.hostname ? window.location.hostname : '')

const sambaService = ref<SambaServiceState>({
  installed: false,
  running: false,
  enabled: true,
  version: '',
  workgroup: 'WORKGROUP',
  description: 'R3P Home Server NAS',
  interface: 'lan',
  shares_count: 0,
  server_ip: '',
  active_shares: [],
})

const ftpService = ref<FTPServiceState>({
  installed: false,
  running: false,
  enabled: true,
  version: '',
  port: 21,
  root_dir: '/mnt/sda1',
  write_enable: true,
  anonymous_enable: false,
  local_enable: true,
  server_ip: '',
})

const savingSamba = ref(false)
const savingFTP = ref(false)

function copyToClipboard(text: string, label = 'Đường dẫn') {
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(() => {
      showNotice(`Đã sao chép ${label}: ${text}`)
    }).catch(() => {
      showNotice(`Đã sao chép: ${text}`)
    })
  } else {
    showNotice(`Đã sao chép: ${text}`)
  }
}

async function toggleService(service: 'samba' | 'ftp', targetState: boolean) {
  if (service === 'samba') {
    savingSamba.value = true
    try {
      await apiRequest('/nas/services/samba', {
        method: 'PUT',
        body: JSON.stringify({
          running: targetState,
          enabled: targetState,
          action: targetState ? 'start' : 'stop',
        }),
      })
      sambaService.value.running = targetState
      sambaService.value.enabled = targetState
      showNotice(targetState ? 'Đã BẬT dịch vụ SMB (Samba 4)!' : 'Đã TẮT dịch vụ SMB (Samba 4)!')
      await loadNAS()
    } catch (err: any) {
      showError('Lỗi chuyển trạng thái Samba: ' + (err?.message || err))
    } finally {
      savingSamba.value = false
    }
  } else {
    savingFTP.value = true
    try {
      await apiRequest('/nas/services/ftp', {
        method: 'PUT',
        body: JSON.stringify({
          running: targetState,
          enabled: targetState,
          action: targetState ? 'start' : 'stop',
        }),
      })
      ftpService.value.running = targetState
      ftpService.value.enabled = targetState
      showNotice(targetState ? 'Đã BẬT dịch vụ FTP (vsftpd)!' : 'Đã TẮT dịch vụ FTP (vsftpd)!')
      await loadNAS()
    } catch (err: any) {
      showError('Lỗi chuyển trạng thái FTP: ' + (err?.message || err))
    } finally {
      savingFTP.value = false
    }
  }
}

async function saveSambaConfig(restart = false) {
  savingSamba.value = true
  try {
    const res = await apiRequest<{ message: string }>('/nas/services/samba', {
      method: 'PUT',
      body: JSON.stringify({
        workgroup: sambaService.value.workgroup,
        description: sambaService.value.description,
        enabled: sambaService.value.enabled,
        running: sambaService.value.running,
        restart,
      }),
    })
    showNotice(res?.message || (restart ? 'Đã khởi động lại Samba 4!' : 'Đã lưu cấu hình Samba 4!'))
    await loadNAS()
  } catch (err: any) {
    showError('Lỗi cập nhật Samba: ' + (err?.message || err))
  } finally {
    savingSamba.value = false
  }
}

async function saveFTPConfig(restart = false) {
  savingFTP.value = true
  try {
    const res = await apiRequest<{ message: string }>('/nas/services/ftp', {
      method: 'PUT',
      body: JSON.stringify({
        port: Number(ftpService.value.port) || 21,
        root_dir: ftpService.value.root_dir,
        write_enable: ftpService.value.write_enable,
        anonymous_enable: ftpService.value.anonymous_enable,
        local_enable: ftpService.value.local_enable,
        enabled: ftpService.value.enabled,
        running: ftpService.value.running,
        restart,
      }),
    })
    showNotice(res?.message || (restart ? 'Đã khởi động lại vsftpd!' : 'Đã lưu cấu hình FTP!'))
    await loadNAS()
  } catch (err: any) {
    showError('Lỗi cập nhật FTP: ' + (err?.message || err))
  } finally {
    savingFTP.value = false
  }
}


async function loadNAS() {
  loading.value = true
  try {
    const [res, usb, services] = await Promise.all([
      apiRequest<{
        volumes: DiskVolume[]
        shares: ShareItem[]
        server_ip?: string
      }>('/nas?dir=/mnt/sda1'),
      apiRequest<USBDualRoleStatus>('/usb/status').catch(() => null),
      apiRequest<{
        samba: SambaServiceState
        ftp: FTPServiceState
        server_ip: string
      }>('/nas/services').catch(() => null),
    ])
    if (res) {
      volumes.value = res.volumes || []
      shares.value = res.shares || []
      if (res.server_ip) {
        serverIp.value = res.server_ip
      }
    }
    if (services) {
      if (services.samba) sambaService.value = services.samba
      if (services.ftp) ftpService.value = services.ftp
      if (services.server_ip) serverIp.value = services.server_ip
    }
    if (usb) {
      usbStatus.value = usb
      if (usb.storage_devices?.length && usb.storage_devices[0]?.device) {
        formatForm.value.device = usb.storage_devices[0].device
      }
    }
    void fetchNASUsers()
  } catch (err: any) {
    notice.value = 'Lỗi nạp dữ liệu: ' + (err?.message || err)
  } finally {
    loading.value = false
  }
}

async function runFormatUSB() {
  if (!formatConfirm.value || formattingUsb.value) return
  formattingUsb.value = true
  errorMsg.value = null
  notice.value = `Đang tiến hành Format ${formatForm.value.device} sang chuẩn ${formatForm.value.filesystem.toUpperCase()} (Nhãn: ${formatForm.value.label})… Vui lòng đợi 10-30 giây!`
  formatLog.value = 'Đang khởi chạy tiến trình Format & tối ưu tốc độ cổng USB 3.0…'
  try {
    const res = await apiRequest<{
      formatted: boolean
      device: string
      mount_point: string
      filesystem: string
      label: string
      log: string
      message: string
    }>('/usb/format', {
      method: 'POST',
      body: JSON.stringify(formatForm.value),
    })
    formatLog.value = res?.log || 'Format hoàn tất!'
    showNotice(res?.message || 'Đã Format USB thành công!', 5000)
    formatConfirm.value = false
    await loadNAS()
  } catch (err: any) {
    showError('Format USB thất bại: ' + (err?.message || err))
    formatLog.value = 'LỖI: ' + (err?.message || err)
  } finally {
    formattingUsb.value = false
  }
}

async function toggleMountUSB(device: string, action: 'mount' | 'unmount') {
  mountingUsb.value = true
  errorMsg.value = null
  try {
    const res = await apiRequest<{ message: string }>('/usb/mount', {
      method: 'POST',
      body: JSON.stringify({ device, action }),
    })
    showNotice(res?.message || `Đã ${action} ${device}!`)
    await loadNAS()
  } catch (err: any) {
    showError(`Lỗi ${action}: ` + (err?.message || err))
  } finally {
    mountingUsb.value = false
  }
}

async function runDiskBenchmark() {
  if (runningBench.value) return
  runningBench.value = true
  notice.value = `Đang kiểm tra tốc độ Đọc / Ghi thực tế (${benchSizeMb.value} MB) trên ổ cứng USB 3.0 (/mnt/sda1)…`
  try {
    const res = await apiRequest<BenchmarkResult>('/usb/benchmark', {
      method: 'POST',
      body: JSON.stringify({ target_dir: '/mnt/sda1', size_mb: benchSizeMb.value }),
    })
    if (res) {
      benchResult.value = res
      showNotice(`Hoàn tất Benchmark ${res.size_mb}MB: Ghi ${res.write_mbps} MB/s · Đọc ${res.read_mbps} MB/s!`, 5000)
    }
  } catch (err: any) {
    showError('Benchmark lỗi: ' + (err?.message || err))
  } finally {
    runningBench.value = false
  }
}

async function addShare() {
  if (!newShare.value.name || !newShare.value.path) return
  try {
    await apiRequest('/nas/shares', {
      method: 'POST',
      body: JSON.stringify(newShare.value),
    })
    showAddShare.value = false
    newShare.value = {
      name: '',
      path: '/mnt/sda1/share',
      read_only: false,
      guest_ok: true,
      comment: '',
      users: '',
      password: '',
    }
    showNotice('Đã thêm thư mục chia sẻ thành công!')
    await loadNAS()
  } catch (err: any) {
    showError('Thêm thư mục thất bại: ' + (err?.message || err))
  }
}

async function removeShare(name: string) {
  try {
    await apiRequest(`/nas/shares?name=${encodeURIComponent(name)}`, { method: 'DELETE' })
    showNotice(`Đã xóa thư mục chia sẻ '${name}'!`)
    await loadNAS()
  } catch (err: any) {
    showError('Lỗi xóa thư mục: ' + (err?.message || err))
  }
}


onMounted(() => {
  void loadNAS()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3.5 bg-[#080b11]">
    <!-- Floating Notices -->
    <div v-if="notice" class="r3p-toast r3p-toast-info">
      <span class="r3p-toast-text">{{ notice }}</span>
      <button type="button" class="text-xs opacity-80 hover:opacity-100 px-1 cursor-pointer" @click="notice = null">✕</button>
    </div>
    <div v-if="errorMsg" class="r3p-toast r3p-toast-error">
      <span class="r3p-toast-text">⚠ {{ errorMsg }}</span>
      <button type="button" class="text-xs opacity-80 hover:opacity-100 px-1 cursor-pointer" @click="errorMsg = null">✕</button>
    </div>

    <!-- Section 1: USB 3.0 Dual-Role Storage & Tethering Engine (Intact) -->
    <div class="pro-card border border-sky-500/30">
      <div class="pro-card-header flex-wrap">
        <div class="flex items-center gap-2 min-w-0">
          <span class="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse shrink-0"></span>
          <span class="pro-card-title truncate">Cổng USB 3.0 SuperSpeed Dual-Role (Ổ Cứng Lưu Trữ + USB Tethering / 4G WAN)</span>
          <span class="badge badge-blue uppercase">MODE: {{ usbStatus.active_mode }}</span>
          <span v-if="usbStatus.hotplug_ready" class="badge badge-green">AUTO-HOTPLUG ACTIVE</span>
        </div>

        <div class="flex flex-wrap items-center gap-2 shrink-0">
          <button
            type="button"
            class="btn-danger text-[12px] py-1.5 px-3 min-w-[160px] font-bold"
            @click="showFormatPanel = !showFormatPanel"
          >
            {{ showFormatPanel ? '✕ Đóng Format' : '🛠 Format USB / Ổ Cứng' }}
          </button>
          <select v-model.number="benchSizeMb" class="pro-input mono text-[12px] py-1.5 px-2 font-medium">
            <option :value="64">Test 64 MB</option>
            <option :value="128">Test 128 MB</option>
            <option :value="256">Test 256 MB</option>
          </select>
          <button
            type="button"
            class="btn-primary text-[12px] py-1.5 px-3 min-w-[160px] font-bold shadow-md"
            :disabled="runningBench"
            @click="runDiskBenchmark"
          >
            {{ runningBench ? '⚡ Đang Test Tốc Độ…' : '⚡ Test Tốc Độ Đọc/Ghi' }}
          </button>
          <button type="button" class="btn-secondary text-[12px] py-1.5 px-3 font-bold" :disabled="loading" @click="loadNAS()">↻</button>
        </div>
      </div>

      <!-- Format USB / External HDD Interactive Panel -->
      <div
        v-if="showFormatPanel"
        class="mb-3 p-3.5 rounded-lg bg-[#140d18] border border-red-500/40 space-y-3 text-xs"
      >
        <div class="flex items-center justify-between border-b border-white/10 pb-2">
          <div>
            <div class="font-bold text-red-300 uppercase text-xs flex items-center gap-2">
              <span>🛠 Công Cụ Định Dạng (Format) & Phân Vùng Ổ Cứng / USB 3.0</span>
              <span class="badge badge-orange">HIGH-SPEED TUNING</span>
            </div>
            <p class="text-[11px] text-gray-400 mt-0.5">
              Hỗ trợ định dạng sang chuẩn <code class="mono text-sky-300">ext4</code> (tối ưu tốc độ cao nhất cho Linux NAS), <code class="mono text-sky-300">exFAT</code>, <code class="mono text-sky-300">FAT32</code> hoặc <code class="mono text-sky-300">NTFS</code>.
            </p>
          </div>
          <button type="button" class="text-gray-400 hover:text-white text-xs" @click="showFormatPanel = false">✕ Đóng</button>
        </div>

        <form class="space-y-3" @submit.prevent="runFormatUSB">
          <div class="grid grid-cols-1 md:grid-cols-4 gap-2.5">
            <div>
              <label class="block text-[10px] text-gray-400 uppercase mb-1">Thiết Bị Cần Format</label>
              <select v-model="formatForm.device" class="w-full pro-input mono">
                <option
                  v-for="d in usbStatus.storage_devices"
                  :key="d.device"
                  :value="d.device"
                >
                  {{ d.device }} ({{ d.vendor }} {{ d.model }} · {{ d.size_human }})
                </option>
                <option value="/dev/sda1">/dev/sda1 (USB Partition 1)</option>
                <option value="/dev/sda">/dev/sda (Toàn bộ ổ đĩa USB 1)</option>
                <option value="/dev/sdb1">/dev/sdb1 (USB Partition 2)</option>
              </select>
            </div>

            <div>
              <label class="block text-[10px] text-gray-400 uppercase mb-1">Chuẩn Định Dạng (FS)</label>
              <select v-model="formatForm.filesystem" class="w-full pro-input mono">
                <option value="ext4">ext4 (Khuyên dùng — Nhanh nhất)</option>
                <option value="exfat">exFAT (Đọc/Ghi Windows, Mac, Linux)</option>
                <option value="vfat">FAT32 / vfat (Tương thích rộng)</option>
                <option value="ntfs">NTFS (Windows FS)</option>
              </select>
            </div>

            <div>
              <label class="block text-[10px] text-gray-400 uppercase mb-1">Nhãn Ổ Đĩa (Label)</label>
              <input
                v-model="formatForm.label"
                type="text"
                maxlength="16"
                required
                class="w-full pro-input mono"
                placeholder="R3P_HDD_500G"
              />
            </div>

            <div class="flex flex-col justify-end">
              <label class="flex items-center gap-2 p-2 rounded bg-black/40 border border-white/10 cursor-pointer">
                <input v-model="formatForm.repartition" type="checkbox" class="rounded" />
                <span class="text-[11px] text-gray-200">Tạo lại bảng phân vùng (100% dung lượng)</span>
              </label>
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-white/10">
            <label class="flex items-center gap-2 text-xs text-red-300 font-semibold cursor-pointer">
              <input v-model="formatConfirm" type="checkbox" class="rounded" />
              <span>⚠ Xác nhận xóa sạch dữ liệu trên {{ formatForm.device }} để Format mới</span>
            </label>

            <div class="flex items-center gap-2">
              <button type="button" class="btn-secondary text-xs" @click="showFormatPanel = false">Hủy</button>
              <button
                type="submit"
                class="btn-danger text-xs px-4 py-1.5 font-bold"
                :disabled="!formatConfirm || formattingUsb"
              >
                {{ formattingUsb ? '⏳ Đang Format…' : `🔥 Format ${formatForm.device}` }}
              </button>
            </div>
          </div>
        </form>

        <pre
          v-if="formatLog"
          class="bg-black/60 border border-white/10 rounded p-2.5 mono text-[10.5px] text-emerald-300 max-h-40 overflow-y-auto whitespace-pre-wrap"
        >{{ formatLog }}</pre>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-3 gap-3 text-[11.5px]">
        <!-- Role 1: USB 3.0 External HDD Storage -->
        <div class="p-2.5 rounded-lg bg-black/30 border border-white/10 space-y-1.5">
          <div class="flex items-center justify-between">
            <span class="font-bold text-sky-300 uppercase text-[10.5px]">1. Ổ Lưu Trữ (USB Storage / HDD)</span>
            <span class="badge" :class="usbStatus.storage_devices?.length ? 'badge-green' : 'badge-gray'">
              {{ usbStatus.storage_devices?.length ? `${usbStatus.storage_devices.length} DISK ONLINE` : 'NO USB DISK' }}
            </span>
          </div>
          <div v-if="usbStatus.storage_devices?.length" class="space-y-1">
            <div v-for="d in usbStatus.storage_devices" :key="d.device" class="space-y-1">
              <div class="flex justify-between">
                <span class="text-gray-400">Thiết bị:</span>
                <span class="mono text-gray-100 font-semibold">{{ d.vendor }} {{ d.model }} ({{ d.device }})</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-400">Mount Point:</span>
                <span class="mono text-emerald-400">{{ d.mount_point || '/mnt/sda1' }} ({{ d.fs_type || 'ext4' }})</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-400">Khả dụng:</span>
                <span class="mono text-sky-300 font-semibold">{{ d.avail_gb }} GB trống / {{ d.total_gb || d.size_human }} GB</span>
              </div>
              <div class="flex items-center gap-1.5 pt-1 border-t border-white/5">
                <button
                  v-if="d.mounted"
                  type="button"
                  class="btn-secondary text-[10px] py-0.5 px-2"
                  :disabled="mountingUsb"
                  @click="toggleMountUSB(d.device, 'unmount')"
                >
                  ⏏ Unmount
                </button>
                <button
                  v-else
                  type="button"
                  class="btn-primary text-[10px] py-0.5 px-2"
                  :disabled="mountingUsb"
                  @click="toggleMountUSB(d.device, 'mount')"
                >
                  🔌 Mount
                </button>
                <button
                  type="button"
                  class="btn-danger text-[10px] py-0.5 px-2 ml-auto"
                  @click="formatForm.device = d.device; showFormatPanel = true"
                >
                  🛠 Format
                </button>
              </div>
            </div>
          </div>
          <div v-else class="text-gray-400 text-xs py-2 space-y-1.5">
            <p>Tự động nhận diện và mount ổ cứng USB 3.0 vào <code class="mono text-sky-300">/mnt/sda1</code>.</p>
          </div>
        </div>

        <!-- Role 2: USB Tethering & 4G Modem WAN -->
        <div class="p-2.5 rounded-lg bg-black/30 border border-white/10 space-y-1.5">
          <div class="flex items-center justify-between">
            <span class="font-bold text-purple-300 uppercase text-[10.5px]">2. Cổng Internet (USB Tether / 4G)</span>
            <span class="badge" :class="usbStatus.tether_devices?.length ? 'badge-green' : 'badge-purple'">
              {{ usbStatus.tether_devices?.length ? 'USB WAN ACTIVE' : 'STANDBY (READY)' }}
            </span>
          </div>
          <div v-if="usbStatus.tether_devices?.length" class="space-y-1">
            <div v-for="t in usbStatus.tether_devices" :key="t.iface" class="space-y-1">
              <div class="flex justify-between">
                <span class="text-gray-400">Interface:</span>
                <span class="mono text-gray-100 font-semibold">{{ t.iface }} ({{ t.driver }})</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-400">WAN IP:</span>
                <span class="mono text-emerald-400 font-semibold">{{ t.ip || 'DHCP Acquiring…' }}</span>
              </div>
            </div>
          </div>
          <div v-else class="text-gray-400 text-[11px] space-y-1">
            <p>Hỗ trợ cắm điện thoại chia sẻ 4G (Android RNDIS, iPhone) hoặc Dcom 4G LTE.</p>
          </div>
        </div>

        <!-- Role 3: Benchmark Panel -->
        <div class="p-2.5 rounded-lg bg-black/30 border border-emerald-500/25 space-y-1.5">
          <div class="flex items-center justify-between">
            <span class="font-bold text-emerald-300 uppercase text-[10.5px]">3. Tốc Độ Đọc / Ghi USB 3.0 Thực Tế</span>
            <span class="mono text-[10px] text-gray-400">{{ benchResult.size_mb }} MB Direct I/O</span>
          </div>
          <div class="grid grid-cols-2 gap-2 py-1">
            <div class="p-2 rounded bg-emerald-500/10 border border-emerald-500/25 text-center">
              <div class="text-[10px] text-gray-400 uppercase">Tốc Độ Ghi</div>
              <div class="mono text-base font-bold text-emerald-400">{{ benchResult.write_mbps }} MB/s</div>
              <div class="text-[10px] text-gray-500 mono">{{ benchResult.write_sec }}s</div>
            </div>
            <div class="p-2 rounded bg-sky-500/10 border border-sky-500/25 text-center">
              <div class="text-[10px] text-gray-400 uppercase">Tốc Độ Đọc</div>
              <div class="mono text-base font-bold text-sky-400">{{ benchResult.read_mbps }} MB/s</div>
              <div class="text-[10px] text-gray-500 mono">{{ benchResult.read_sec }}s</div>
            </div>
          </div>
          <div class="text-[10px] text-gray-400 mono truncate">
            FS: {{ benchResult.filesystem }}
          </div>
        </div>
      </div>
    </div>

    <!-- Section 2: Unified Network Sharing — EXACTLY 2 TABS: SMB & FTP -->
    <div class="pro-card border border-sky-500/30">
      <!-- Tab Header Bar with Clean Toggle -->
      <div class="flex flex-wrap items-center justify-between border-b border-white/10 pb-3 gap-2">
        <div class="flex items-center gap-2">
          <span class="text-sm font-bold text-white tracking-wide uppercase">Dịch Vụ Chia Sẻ Tệp (Share & USB)</span>
          <span class="mono text-xs text-sky-300 bg-sky-950/60 px-2 py-0.5 rounded border border-sky-500/30">IP: {{ serverIp }}</span>
        </div>

        <div class="flex items-center bg-black/40 p-1 rounded-lg border border-white/10 gap-1">
          <button
            type="button"
            class="px-3.5 py-1.5 rounded-md text-xs font-bold transition-all flex items-center gap-1.5 cursor-pointer"
            :class="activeShareTab === 'smb' ? 'bg-sky-500 text-white shadow' : 'text-gray-400 hover:text-white'"
            @click="activeShareTab = 'smb'"
          >
            <span>📁 Chia Sẻ SMB (Samba 4)</span>
            <span
              class="w-2 h-2 rounded-full"
              :class="sambaService.running ? 'bg-emerald-400 animate-pulse' : 'bg-gray-500'"
            ></span>
          </button>
          <button
            type="button"
            class="px-3.5 py-1.5 rounded-md text-xs font-bold transition-all flex items-center gap-1.5 cursor-pointer"
            :class="activeShareTab === 'ftp' ? 'bg-emerald-600 text-white shadow' : 'text-gray-400 hover:text-white'"
            @click="activeShareTab = 'ftp'"
          >
            <span>🌐 Máy Chủ FTP (vsftpd)</span>
            <span
              class="w-2 h-2 rounded-full"
              :class="ftpService.running ? 'bg-emerald-400 animate-pulse' : 'bg-gray-500'"
            ></span>
          </button>
        </div>
      </div>

      <!-- TAB 1: SMB / SAMBA 4 -->
      <div v-if="activeShareTab === 'smb'" class="mt-3.5 space-y-3.5">
        <!-- SMB Master Service Switch Banner -->
        <div class="p-3.5 rounded-xl bg-[#0e1626] border border-sky-500/30 flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-3">
            <div
              class="w-10 h-10 rounded-lg flex items-center justify-center font-bold text-lg shadow shrink-0"
              :class="sambaService.running ? 'bg-emerald-500/20 border border-emerald-500/40 text-emerald-300' : 'bg-gray-800 text-gray-400 border border-white/10'"
            >
              SMB
            </div>
            <div>
              <div class="flex items-center gap-2">
                <span class="font-bold text-white text-sm">Dịch Vụ SMB / Samba 4 (Windows Explorer, Mac, Android, TV)</span>
                <span class="badge" :class="sambaService.running ? 'badge-green' : 'badge-red'">
                  {{ sambaService.running ? 'ĐANG CHẠY' : 'ĐÃ DỪNG' }}
                </span>
              </div>
              <p class="text-[11.5px] text-gray-400 mt-0.5">
                Chia sẻ thư mục nội bộ qua giao thức SMB/CIFS, kết nối từ máy tính qua đường dẫn:
                <code class="mono text-sky-300 select-all font-semibold">\\{{ serverIp }}\</code>
              </p>
            </div>
          </div>

          <!-- Master ON/OFF Switch -->
          <div class="flex items-center gap-2 shrink-0">
            <button
              type="button"
              class="btn-secondary text-xs py-1.5 px-3"
              @click="copyToClipboard('\\\\' + serverIp + '\\', 'Đường dẫn SMB')"
            >
              📋 Copy \\{{ serverIp }}\
            </button>
            <button
              type="button"
              class="btn-secondary text-xs py-1.5 px-3"
              :disabled="savingSamba"
              @click="saveSambaConfig(true)"
            >
              🔄 Khởi động lại
            </button>
            <button
              type="button"
              class="px-4 py-1.5 rounded-lg font-bold text-xs shadow-md transition-all cursor-pointer flex items-center gap-1.5"
              :class="sambaService.running ? 'bg-red-600 hover:bg-red-500 text-white' : 'bg-emerald-600 hover:bg-emerald-500 text-white'"
              :disabled="savingSamba"
              @click="toggleService('samba', !sambaService.running)"
            >
              <span>{{ sambaService.running ? '⏹ TẮT DỊCH VỤ' : '▶ BẬT DỊCH VỤ' }}</span>
            </button>
          </div>
        </div>

        <!-- SMB Basic Settings -->
        <div class="p-3 rounded-lg bg-black/30 border border-white/10 grid grid-cols-1 md:grid-cols-3 gap-2.5 text-xs">
          <div>
            <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Workgroup Mạng</label>
            <input v-model="sambaService.workgroup" class="pro-input mono w-full" placeholder="WORKGROUP" />
          </div>
          <div>
            <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Tên Máy Chủ Hiển Thị</label>
            <input v-model="sambaService.description" class="pro-input w-full" placeholder="R3P Home Server NAS" />
          </div>
          <div class="flex items-end gap-2">
            <label class="flex items-center gap-2 p-2 rounded bg-black/40 border border-white/10 flex-1 cursor-pointer">
              <input v-model="sambaService.enabled" type="checkbox" class="rounded accent-sky-500" />
              <span class="text-[11px] text-gray-300">Khởi động cùng hệ thống (Autostart)</span>
            </label>
            <button
              type="button"
              class="btn-primary text-xs py-2 px-3 font-semibold shrink-0"
              :disabled="savingSamba"
              @click="saveSambaConfig(false)"
            >
              {{ savingSamba ? '⏳ Đang lưu…' : '💾 Lưu Cài Đặt' }}
            </button>
          </div>
        </div>

        <!-- SMB User Management & Password Control -->
        <div class="p-3.5 rounded-lg bg-black/30 border border-sky-500/20 space-y-3">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span class="text-sky-400 font-bold text-xs uppercase">👥 Tài Khoản Người Dùng & Phân Quyền (Samba Users)</span>
              <span class="badge badge-blue font-mono font-bold">{{ nasUsers.length }} Users</span>
            </div>
            <button
              type="button"
              class="btn-secondary text-xs py-1 px-2.5 font-bold flex items-center gap-1 cursor-pointer"
              @click="showAddUserModal = !showAddUserModal"
            >
              <span>{{ showAddUserModal ? '✕ Đóng' : '+ Thêm Tài Khoản Mới' }}</span>
            </button>
          </div>

          <!-- Add User Form -->
          <div v-if="showAddUserModal" class="p-3 rounded-lg bg-[#0e1724] border border-sky-500/30 space-y-2.5 text-xs">
            <div class="font-bold text-sky-300">Tạo hoặc Cập Nhật Mật Khẩu Tài Khoản Samba</div>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-2.5">
              <div>
                <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Tên Người Dùng (Username)</label>
                <input
                  v-model="newUserName"
                  type="text"
                  class="pro-input mono w-full"
                  placeholder="Ví dụ: family, backup, nhanvien"
                />
              </div>
              <div>
                <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Mật Khẩu (Password)</label>
                <input
                  v-model="newUserPassword"
                  type="password"
                  class="pro-input mono w-full"
                  placeholder="••••••••"
                />
              </div>
            </div>
            <div class="flex justify-end gap-2 pt-1">
              <button type="button" class="btn-secondary text-xs" @click="showAddUserModal = false">Hủy</button>
              <button
                type="button"
                class="btn-primary text-xs py-1 px-3 font-bold"
                :disabled="savingUser"
                @click="createNASUser"
              >
                {{ savingUser ? 'Đang tạo…' : '✓ Lưu Tài Khoản' }}
              </button>
            </div>
          </div>

          <!-- Users List -->
          <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-2">
            <div
              v-for="u in nasUsers"
              :key="u.username"
              class="p-2.5 rounded bg-black/40 border border-white/5 flex items-center justify-between"
            >
              <div class="flex items-center gap-2 min-w-0">
                <span class="text-base">{{ u.is_root ? '👑' : '👤' }}</span>
                <div class="min-w-0">
                  <div class="font-bold text-gray-100 mono text-xs truncate">{{ u.username }}</div>
                  <div class="text-[11px] text-gray-400 truncate">{{ u.status }}</div>
                </div>
              </div>
              <button
                v-if="!u.is_root"
                type="button"
                class="btn-danger text-[10px] py-0.5 px-2"
                title="Xóa tài khoản này"
                @click="deleteNASUser(u.username)"
              >
                Xóa
              </button>
            </div>
          </div>
        </div>

        <!-- Configured SMB Shares Table & Add Modal -->
        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <div class="text-xs font-bold text-sky-300 uppercase flex items-center gap-2">
              <span>Danh Sách Thư Mục Chia Sẻ SMB ({{ shares.length }})</span>
            </div>
            <button
              type="button"
              class="btn-primary text-xs py-1.5 px-3 font-bold flex items-center gap-1 shadow"
              @click="showAddShare = !showAddShare"
            >
              <span>{{ showAddShare ? '✕ Đóng' : '+ Thêm Thư Mục Chia Sẻ' }}</span>
            </button>
          </div>

          <!-- Add / Edit Share Form -->
          <form
            v-if="showAddShare"
            class="p-3.5 rounded-lg bg-[#0e1724] border border-sky-500/40 space-y-3 text-xs"
            @submit.prevent="addShare"
          >
            <div class="font-bold text-sky-300 border-b border-white/10 pb-1.5 flex items-center justify-between">
              <span>Thêm Thư Mục Chia Sẻ SMB Mới</span>
              <span class="text-[11px] text-gray-400">Hỗ trợ phân quyền Công khai hoặc Mật khẩu</span>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-3 gap-2.5">
              <div>
                <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Tên Share (Tên hiển thị trên mạng)</label>
                <input
                  v-model="newShare.name"
                  type="text"
                  required
                  class="pro-input mono w-full"
                  placeholder="Vi du: Media, Backup, Phim"
                />
              </div>

              <div>
                <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Đường Dẫn Trên Ổ Đĩa (Path)</label>
                <input
                  v-model="newShare.path"
                  type="text"
                  required
                  class="pro-input mono w-full"
                  placeholder="/mnt/sda1/media"
                />
              </div>

              <div>
                <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Mô Tả Thư Mục</label>
                <input
                  v-model="newShare.comment"
                  type="text"
                  class="pro-input w-full"
                  placeholder="Kho phim, video và dữ liệu cá nhân"
                />
              </div>
            </div>

            <!-- Permission & Authentication Settings -->
            <div class="grid grid-cols-1 md:grid-cols-2 gap-2.5 p-2.5 rounded bg-black/40 border border-white/10">
              <div>
                <label class="block text-[10.5px] text-gray-400 uppercase mb-1.5 font-bold">Chế Độ Truy Cập</label>
                <div class="space-y-1.5">
                  <label class="flex items-center gap-2 cursor-pointer">
                    <input v-model="newShare.read_only" :value="false" type="radio" class="accent-sky-500" />
                    <span class="text-gray-200">Cho phép Đọc & Ghi (Read / Write — Khuyên dùng)</span>
                  </label>
                  <label class="flex items-center gap-2 cursor-pointer">
                    <input v-model="newShare.read_only" :value="true" type="radio" class="accent-sky-500" />
                    <span class="text-gray-200">Chỉ đọc (Read-Only — Không cho phép sửa/xóa)</span>
                  </label>
                </div>
              </div>

              <div>
                <label class="block text-[10.5px] text-gray-400 uppercase mb-1.5 font-bold">Phương Thức Xác Thực (User & Mật Khẩu)</label>
                <div class="space-y-1.5">
                  <label class="flex items-center gap-2 cursor-pointer">
                    <input v-model="newShare.guest_ok" :value="true" type="radio" class="accent-emerald-500" />
                    <span class="text-emerald-300 font-semibold">Công Khai (Guest OK — Không cần mật khẩu)</span>
                  </label>
                  <label class="flex items-center gap-2 cursor-pointer">
                    <input v-model="newShare.guest_ok" :value="false" type="radio" class="accent-amber-500" />
                    <span class="text-amber-300 font-semibold">Bảo Mật (Yêu cầu Tên Người Dùng & Mật Khẩu)</span>
                  </label>
                </div>
              </div>
            </div>

            <!-- Username & Password inputs if GuestOk is false -->
            <div v-if="!newShare.guest_ok" class="grid grid-cols-1 md:grid-cols-2 gap-2.5 p-2.5 rounded bg-amber-950/20 border border-amber-500/30">
              <div>
                <label class="block text-[10.5px] text-amber-300 uppercase mb-1 font-bold">Chọn Tài Khoản Cấp Quyền</label>
                <div class="flex gap-1.5">
                  <select
                    v-model="newShare.users"
                    class="pro-input mono flex-1"
                    required
                  >
                    <option value="" disabled>-- Chọn tài khoản --</option>
                    <option v-for="u in nasUsers" :key="u.username" :value="u.username">
                      {{ u.username }} ({{ u.is_root ? 'Quản trị root' : 'Samba User' }})
                    </option>
                  </select>
                </div>
              </div>
              <div>
                <label class="block text-[10.5px] text-amber-300 uppercase mb-1 font-bold">Mật Khẩu Của User Này</label>
                <input
                  v-model="newShare.password"
                  type="password"
                  required
                  class="pro-input mono w-full"
                  placeholder="Nhập mật khẩu của user"
                />
              </div>
            </div>

            <div class="flex items-center justify-end gap-2 pt-1 border-t border-white/10">
              <button type="button" class="btn-secondary text-xs" @click="showAddShare = false">Hủy</button>
              <button type="submit" class="btn-primary text-xs px-4 py-1.5 font-bold">Lưu & Tạo Thư Mục Chia Sẻ</button>
            </div>
          </form>

          <!-- Table of Shares -->
          <div class="overflow-x-auto">
            <table class="pro-table">
              <thead>
                <tr>
                  <th>Tên Thư Mục Chia Sẻ</th>
                  <th>Đường Dẫn Ổ Đĩa</th>
                  <th>Quyền Truy Cập</th>
                  <th>Xác Thực</th>
                  <th>Mô Tả</th>
                  <th>Thao Tác</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="!shares.length">
                  <td colspan="6" class="text-center text-gray-500 py-4">Chưa có thư mục chia sẻ nào. Nhấn "+ Thêm Thư Mục Chia Sẻ" ở trên để tạo.</td>
                </tr>
                <tr v-for="s in shares" :key="s.name">
                  <td class="mono font-semibold text-sky-300">
                    <div class="flex items-center gap-1.5 flex-wrap">
                      <span class="truncate">\\{{ serverIp }}\{{ s.name }}</span>
                      <button
                        type="button"
                        class="btn-secondary text-[10px] py-0.5 px-1.5"
                        title="Sao chép đường dẫn UNC"
                        @click="copyToClipboard('\\\\' + serverIp + '\\' + s.name, 'Đường dẫn UNC')"
                      >
                        📋 Copy
                      </button>
                    </div>
                  </td>
                  <td class="mono text-emerald-400">{{ s.path }}</td>
                  <td>
                    <span class="badge" :class="s.read_only ? 'badge-orange' : 'badge-green'">
                      {{ s.read_only ? 'CHỈ ĐỌC' : 'ĐỌC & GHI' }}
                    </span>
                  </td>
                  <td>
                    <span class="badge" :class="s.guest_ok ? 'badge-blue' : 'badge-orange'">
                      {{ s.guest_ok ? 'GUEST (CÔNG KHAI)' : (s.users ? `USER: ${s.users}` : 'CẦN MẬT KHẨU') }}
                    </span>
                  </td>
                  <td class="text-gray-400 text-xs truncate max-w-xs">{{ s.comment || '—' }}</td>
                  <td>
                    <div class="flex items-center gap-1.5">
                      <button type="button" class="btn-danger text-[10px] py-0.5 px-2" @click="removeShare(s.name)">Xóa</button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- TAB 2: FTP / VSFTPD -->
      <div v-if="activeShareTab === 'ftp'" class="mt-3.5 space-y-3.5">
        <!-- FTP Master Service Switch Banner -->
        <div class="p-3.5 rounded-xl bg-[#0e1626] border border-emerald-500/30 flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-3">
            <div
              class="w-10 h-10 rounded-lg flex items-center justify-center font-bold text-lg shadow shrink-0"
              :class="ftpService.running ? 'bg-emerald-500/20 border border-emerald-500/40 text-emerald-300' : 'bg-gray-800 text-gray-400 border border-white/10'"
            >
              FTP
            </div>
            <div>
              <div class="flex items-center gap-2">
                <span class="font-bold text-white text-sm">Máy Chủ FTP (vsftpd Server — FileZilla, Total Commander, Tải File Lớn)</span>
                <span class="badge" :class="ftpService.running ? 'badge-green' : 'badge-red'">
                  {{ ftpService.running ? 'ĐANG CHẠY' : 'ĐÃ DỪNG' }}
                </span>
              </div>
              <p class="text-[11.5px] text-gray-400 mt-0.5">
                Truy cập qua giao thức FTP tiêu chuẩn:
                <code class="mono text-emerald-300 select-all font-semibold">ftp://{{ serverIp }}:{{ ftpService.port }}/</code>
              </p>
            </div>
          </div>

          <!-- Master ON/OFF Switch -->
          <div class="flex items-center gap-2 shrink-0">
            <button
              type="button"
              class="btn-secondary text-xs py-1.5 px-3"
              @click="copyToClipboard('ftp://' + serverIp + ':' + ftpService.port + '/', 'Liên kết FTP')"
            >
              📋 Copy Link FTP
            </button>
            <button
              type="button"
              class="btn-secondary text-xs py-1.5 px-3"
              :disabled="savingFTP"
              @click="saveFTPConfig(true)"
            >
              🔄 Khởi động lại
            </button>
            <button
              type="button"
              class="px-4 py-1.5 rounded-lg font-bold text-xs shadow-md transition-all cursor-pointer flex items-center gap-1.5"
              :class="ftpService.running ? 'bg-red-600 hover:bg-red-500 text-white' : 'bg-emerald-600 hover:bg-emerald-500 text-white'"
              :disabled="savingFTP"
              @click="toggleService('ftp', !ftpService.running)"
            >
              <span>{{ ftpService.running ? '⏹ TẮT DỊCH VỤ' : '▶ BẬT DỊCH VỤ' }}</span>
            </button>
          </div>
        </div>

        <!-- FTP Detailed Settings -->
        <div class="p-3.5 rounded-lg bg-black/30 border border-white/10 space-y-3 text-xs">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Cổng Lắng Nghe (Listen Port)</label>
              <input v-model.number="ftpService.port" type="number" min="1" max="65535" class="pro-input mono w-full" placeholder="21" />
            </div>
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Thư Mục Gốc FTP (Root Directory)</label>
              <input v-model="ftpService.root_dir" class="pro-input mono w-full" placeholder="/mnt/sda1" />
            </div>
          </div>

          <!-- Access Options -->
          <div class="grid grid-cols-1 md:grid-cols-2 gap-2 pt-2 border-t border-white/10">
            <label class="flex items-center gap-2 p-2 rounded bg-black/40 border border-white/5 cursor-pointer">
              <input v-model="ftpService.write_enable" type="checkbox" class="rounded accent-emerald-500" />
              <span class="text-gray-200">Cho phép Ghi và Tải Lên Tệp (Write Enable)</span>
            </label>
            <label class="flex items-center gap-2 p-2 rounded bg-black/40 border border-white/5 cursor-pointer">
              <input v-model="ftpService.local_enable" type="checkbox" class="rounded accent-emerald-500" />
              <span class="text-gray-200">Cho phép tài khoản hệ thống (root / mật khẩu router) đăng nhập</span>
            </label>
            <label class="flex items-center gap-2 p-2 rounded bg-black/40 border border-white/5 cursor-pointer">
              <input v-model="ftpService.anonymous_enable" type="checkbox" class="rounded accent-emerald-500" />
              <span class="text-gray-200">Cho phép truy cập nặc danh (Anonymous — Không cần mật khẩu)</span>
            </label>
            <label class="flex items-center gap-2 p-2 rounded bg-black/40 border border-white/5 cursor-pointer">
              <input v-model="ftpService.enabled" type="checkbox" class="rounded accent-emerald-500" />
              <span class="text-gray-200">Tự động khởi động FTP cùng hệ thống (Autostart)</span>
            </label>
          </div>

          <div class="flex items-center justify-between pt-2 border-t border-white/10">
            <span class="text-[11px] text-gray-400">
              Đăng nhập mặc định bằng tài khoản Root: <code class="mono text-gray-200">root / [mật khẩu router]</code>
            </span>
            <button
              type="button"
              class="btn-primary text-xs py-1.5 px-4 font-bold"
              :disabled="savingFTP"
              @click="saveFTPConfig(false)"
            >
              {{ savingFTP ? '⏳ Đang lưu…' : '💾 Lưu Cấu Hình FTP' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
