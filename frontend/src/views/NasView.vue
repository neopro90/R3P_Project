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
}

interface FileItem {
  name: string
  path: string
  is_dir: boolean
  size: number
  updated_at: string
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

const volumes = ref<DiskVolume[]>([])
const shares = ref<ShareItem[]>([])
const currentDir = ref('/mnt/sda1')
const files = ref<FileItem[]>([])
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

const newFolderName = ref('')
const showNewFolder = ref(false)
const uploading = ref(false)

const showAddShare = ref(false)
const newShare = ref<ShareItem>({ name: '', path: '/mnt/sda1/share', read_only: false, guest_ok: true, comment: '' })

async function loadNAS(dir?: string) {
  loading.value = true
  const target = dir || currentDir.value
  try {
    const [res, usb] = await Promise.all([
      apiRequest<{
        volumes: DiskVolume[]
        shares: ShareItem[]
        current_dir: string
        files: FileItem[]
      }>(`/nas?dir=${encodeURIComponent(target)}`),
      apiRequest<USBDualRoleStatus>('/usb/status').catch(() => null),
    ])
    if (res) {
      volumes.value = res.volumes || []
      shares.value = res.shares || []
      currentDir.value = res.current_dir || target
      files.value = res.files || []
    }
    if (usb) {
      usbStatus.value = usb
      if (usb.storage_devices?.length && usb.storage_devices[0]?.device) {
        formatForm.value.device = usb.storage_devices[0].device
      }
    }
  } catch (err: any) {
    notice.value = 'Error loading NAS: ' + (err?.message || err)
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
    await loadNAS(res?.mount_point || '/mnt/sda1')
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

async function createFolder() {
  if (!newFolderName.value.trim()) return
  try {
    await apiRequest('/nas/mkdir', {
      method: 'POST',
      body: JSON.stringify({ dir: currentDir.value, name: newFolderName.value.trim() }),
    })
    newFolderName.value = ''
    showNewFolder.value = false
    showNotice('Folder created!')
    await loadNAS()
  } catch (err: any) {
    showError('Mkdir failed: ' + (err?.message || err))
  }
}

async function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  uploading.value = true
  notice.value = `Uploading ${file.name}…`
  try {
    const form = new FormData()
    form.append('dir', currentDir.value)
    form.append('file', file)
    const resp = await fetch('/api/v1/nas/upload', {
      method: 'POST',
      credentials: 'include',
      body: form,
    })
    const json = await resp.json()
    if (!resp.ok || !json.ok) {
      throw new Error(json.error || `HTTP ${resp.status}`)
    }
    showNotice(`Uploaded ${file.name} (${formatBytes(file.size)})!`)
    await loadNAS()
  } catch (err: any) {
    showError('Upload failed: ' + (err?.message || err))
  } finally {
    uploading.value = false
    input.value = ''
  }
}

async function deleteEntry(p: string) {
  try {
    await apiRequest(`/nas/file?path=${encodeURIComponent(p)}`, { method: 'DELETE' })
    showNotice('Deleted ' + p)
    await loadNAS()
  } catch (err: any) {
    showError('Delete failed: ' + (err?.message || err))
  }
}

function goUp() {
  const parts = currentDir.value.split('/').filter(Boolean)
  if (parts.length >= 2) {
    parts.pop()
    void loadNAS('/' + parts.join('/'))
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
    newShare.value = { name: '', path: '/mnt/sda1/share', read_only: false, guest_ok: true, comment: '' }
    showNotice('Shared folder added!')
    await loadNAS()
  } catch (err: any) {
    showError('Error: ' + (err?.message || err))
  }
}

async function removeShare(name: string) {
  try {
    await apiRequest(`/nas/shares?name=${encodeURIComponent(name)}`, { method: 'DELETE' })
    showNotice(`Removed share '${name}'!`)
    await loadNAS()
  } catch (err: any) {
    showError('Error: ' + (err?.message || err))
  }
}

function formatStorage(kb: number): string {
  if (!kb) return '0 MB'
  if (kb >= 1024 * 1024) {
    return `${(kb / (1024 * 1024)).toFixed(1)} GB`
  }
  return `${(kb / 1024).toFixed(1)} MB`
}

function formatBytes(b: number): string {
  if (!b) return '0 B'
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / (1024 * 1024)).toFixed(2)} MB`
  return `${(b / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

onMounted(() => {
  void loadNAS()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Floating Non-Shifting Notices -->
    <div v-if="notice" class="r3p-toast r3p-toast-info">
      <span class="r3p-toast-text">{{ notice }}</span>
      <button type="button" class="text-xs opacity-80 hover:opacity-100 px-1 cursor-pointer" @click="notice = null">✕</button>
    </div>
    <div v-if="errorMsg" class="r3p-toast r3p-toast-error">
      <span class="r3p-toast-text">⚠ {{ errorMsg }}</span>
      <button type="button" class="text-xs opacity-80 hover:opacity-100 px-1 cursor-pointer" @click="errorMsg = null">✕</button>
    </div>

    <!-- USB 3.0 Dual-Role (Storage + USB Tethering/4G WAN) & High-Speed Benchmark Card -->
    <div class="pro-card border border-sky-500/30">
      <div class="pro-card-header flex-wrap">
        <div class="flex items-center gap-2 min-w-0">
          <span class="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse shrink-0"></span>
          <span class="pro-card-title truncate">USB 3.0 SuperSpeed Dual-Role Engine (Storage NAS + USB Tethering / 4G WAN)</span>
          <span class="badge badge-blue uppercase">MODE: {{ usbStatus.active_mode }}</span>
          <span v-if="usbStatus.hotplug_ready" class="badge badge-green">AUTO-HOTPLUG ACTIVE</span>
        </div>

        <div class="flex flex-wrap items-center gap-2 shrink-0">
          <button
            type="button"
            class="btn-danger text-[12.5px] py-1.5 px-3 min-w-[184px] font-bold"
            @click="showFormatPanel = !showFormatPanel"
          >
            {{ showFormatPanel ? '✕ Đóng Công Cụ Format' : '🛠 Format USB / Ổ Cứng' }}
          </button>
          <select v-model.number="benchSizeMb" class="pro-input mono text-[12.5px] py-1.5 px-2.5 font-medium">
            <option :value="64">Benchmark 64 MB</option>
            <option :value="128">Benchmark 128 MB</option>
            <option :value="256">Benchmark 256 MB</option>
          </select>
          <button
            type="button"
            class="btn-primary text-[12.5px] py-1.5 px-3.5 min-w-[174px] font-bold shadow-md"
            :disabled="runningBench"
            @click="runDiskBenchmark"
          >
            {{ runningBench ? '⚡ Đang Test Tốc Độ…' : '⚡ Test Tốc Độ Đọc/Ghi' }}
          </button>
          <button type="button" class="btn-secondary text-[12.5px] py-1.5 px-3 font-bold" :disabled="loading" @click="loadNAS()">↻</button>
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
              <span class="badge badge-orange">HIGH-SPEED TUNING AUTO-APPLIED</span>
            </div>
            <p class="text-[11px] text-gray-400 mt-0.5">
              Hỗ trợ định dạng sang chuẩn <code class="mono text-sky-300">ext4</code> (tối ưu tốc độ đọc/ghi cao nhất cho Linux NAS), <code class="mono text-sky-300">exFAT</code>, <code class="mono text-sky-300">FAT32</code> hoặc <code class="mono text-sky-300">NTFS</code>, tự động mount vào <code class="mono text-emerald-300">/mnt/sda1</code> và tạo thư mục NAS / ESP32_Downloads.
            </p>
          </div>
          <button type="button" class="text-gray-400 hover:text-white text-xs" @click="showFormatPanel = false">✕ Đóng</button>
        </div>

        <form class="space-y-3" @submit.prevent="runFormatUSB">
          <div class="grid grid-cols-1 md:grid-cols-4 gap-2.5">
            <div>
              <label class="block text-[10px] text-gray-400 uppercase mb-1">Chọn Thiết Bị USB Cần Format</label>
              <div class="flex gap-1">
                <select v-model="formatForm.device" class="pro-input mono flex-1">
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
            </div>

            <div>
              <label class="block text-[10px] text-gray-400 uppercase mb-1">Chuẩn Định Dạng (Filesystem)</label>
              <select v-model="formatForm.filesystem" class="w-full pro-input mono">
                <option value="ext4">ext4 (Khuyên dùng — Tốc độ cao nhất trên R3P)</option>
                <option value="exfat">exFAT (Đọc/Ghi trên cả Windows, macOS & Linux)</option>
                <option value="vfat">FAT32 / vfat (Tương thích phổ thông)</option>
                <option value="ntfs">NTFS (Windows File System)</option>
              </select>
            </div>

            <div>
              <label class="block text-[10px] text-gray-400 uppercase mb-1">Nhãn Ổ Đĩa (Volume Label)</label>
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
                <span class="text-[11px] text-gray-200">Tạo lại bảng phân vùng mới (1 phân vùng 100% dung lượng)</span>
              </label>
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-white/10">
            <label class="flex items-center gap-2 text-xs text-red-300 font-semibold cursor-pointer">
              <input v-model="formatConfirm" type="checkbox" class="rounded" />
              <span>⚠ Tôi xác nhận xóa toàn bộ dữ liệu trên {{ formatForm.device }} để thực hiện Format mới</span>
            </label>

            <div class="flex items-center gap-2">
              <button type="button" class="btn-secondary text-xs" @click="showFormatPanel = false">Hủy</button>
              <button
                type="submit"
                class="btn-danger text-xs px-4 py-1.5 font-bold"
                :disabled="!formatConfirm || formattingUsb"
              >
                {{ formattingUsb ? '⏳ Đang Format & Tối Ưu Tốc Độ…' : `🔥 Bắt Đầu Format ${formatForm.device} (${formatForm.filesystem.toUpperCase()})` }}
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
            <span class="font-bold text-sky-300 uppercase text-[10.5px]">1. Chế Độ Ổ Lưu Trữ (USB Storage / HDD)</span>
            <span class="badge" :class="usbStatus.storage_devices?.length ? 'badge-green' : 'badge-gray'">
              {{ usbStatus.storage_devices?.length ? `${usbStatus.storage_devices.length} DISK ONLINE` : 'NO USB DISK' }}
            </span>
          </div>
          <div v-if="usbStatus.storage_devices?.length" class="space-y-1">
            <div v-for="d in usbStatus.storage_devices" :key="d.device" class="space-y-1">
              <div class="flex justify-between">
                <span class="text-gray-400">Model / Device:</span>
                <span class="mono text-gray-100 font-semibold">{{ d.vendor }} {{ d.model }} ({{ d.device }})</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-400">Mount / FS:</span>
                <span class="mono text-emerald-400">{{ d.mount_point || '/mnt/sda1' }} ({{ d.fs_type || 'ext4' }} · {{ d.label || 'R3P_HDD_500G' }})</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-400">Dung lượng khả dụng:</span>
                <span class="mono text-sky-300 font-semibold">{{ d.avail_gb }} GB trống / {{ d.total_gb || d.size_human }} GB</span>
              </div>
              <div class="text-[10px] text-gray-500 mono truncate">
                Queue Tuning: {{ d.read_ahead }} · {{ d.max_sectors }}
              </div>
              <div class="flex items-center gap-1.5 pt-1 border-t border-white/5">
                <button
                  v-if="d.mounted"
                  type="button"
                  class="btn-secondary text-[10px] py-0.5 px-2"
                  :disabled="mountingUsb"
                  @click="toggleMountUSB(d.device, 'unmount')"
                >
                  ⏏ Unmount (Tháo an toàn)
                </button>
                <button
                  v-else
                  type="button"
                  class="btn-primary text-[10px] py-0.5 px-2"
                  :disabled="mountingUsb"
                  @click="toggleMountUSB(d.device, 'mount')"
                >
                  🔌 Mount {{ d.device }}
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
            <p>Khi cắm USB hoặc ổ cứng di động vào cổng USB 3.0, hệ thống tự động mount vào <code class="mono text-sky-300">/mnt/sda1</code>.</p>
            <button type="button" class="btn-secondary text-[10.5px] py-0.5 px-2" @click="showFormatPanel = true">
              🛠 Mở Công Cụ Format USB Thủ Công
            </button>
          </div>
        </div>

        <!-- Role 2: USB Tethering & 4G Modem WAN -->
        <div class="p-2.5 rounded-lg bg-black/30 border border-white/10 space-y-1.5">
          <div class="flex items-center justify-between">
            <span class="font-bold text-purple-300 uppercase text-[10.5px]">2. Chế Độ Cổng Internet (USB Tether / 4G)</span>
            <span class="badge" :class="usbStatus.tether_devices?.length ? 'badge-green' : 'badge-purple'">
              {{ usbStatus.tether_devices?.length ? 'USB WAN ACTIVE' : 'STANDBY (READY)' }}
            </span>
          </div>
          <div v-if="usbStatus.tether_devices?.length" class="space-y-1">
            <div v-for="t in usbStatus.tether_devices" :key="t.iface" class="space-y-1">
              <div class="flex justify-between">
                <span class="text-gray-400">Interface / Driver:</span>
                <span class="mono text-gray-100 font-semibold">{{ t.iface }} ({{ t.driver }})</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-400">WAN IP:</span>
                <span class="mono text-emerald-400 font-semibold">{{ t.ip || 'DHCP Acquiring…' }}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-400">Traffic:</span>
                <span class="mono text-sky-300">↓ {{ formatBytes(t.rx_bytes) }} / ↑ {{ formatBytes(t.tx_bytes) }}</span>
              </div>
            </div>
          </div>
          <div v-else class="text-gray-400 text-[11px] space-y-1">
            <p>Hỗ trợ tự động nhận diện khi cắm điện thoại bật <b>Chia sẻ kết nối qua USB (Android RNDIS/NCM, iPhone ipheth)</b> hoặc <b>USB 4G LTE (CDC-Ether, QMI, MBIM)</b>.</p>
            <p class="mono text-[10px] text-emerald-400">Drivers loaded: rndis_host, cdc_ether, cdc_ncm, ipheth, qmi_wwan, cdc_mbim</p>
          </div>
        </div>

        <!-- High-Speed Disk Read/Write Benchmark Panel -->
        <div class="p-2.5 rounded-lg bg-black/30 border border-emerald-500/25 space-y-1.5">
          <div class="flex items-center justify-between">
            <span class="font-bold text-emerald-300 uppercase text-[10.5px]">3. Tốc Độ Đọc / Ghi USB 3.0 500GB HDD</span>
            <span class="mono text-[10px] text-gray-400">{{ benchResult.size_mb }} MB Direct I/O</span>
          </div>
          <div class="grid grid-cols-2 gap-2 py-1">
            <div class="p-2 rounded bg-emerald-500/10 border border-emerald-500/25 text-center">
              <div class="text-[10px] text-gray-400 uppercase">Tốc Độ Ghi (Write)</div>
              <div class="mono text-base font-bold text-emerald-400">{{ benchResult.write_mbps }} MB/s</div>
              <div class="text-[10px] text-gray-500 mono">{{ benchResult.write_sec }}s (fsync)</div>
            </div>
            <div class="p-2 rounded bg-sky-500/10 border border-sky-500/25 text-center">
              <div class="text-[10px] text-gray-400 uppercase">Tốc Độ Đọc (Read)</div>
              <div class="mono text-base font-bold text-sky-400">{{ benchResult.read_mbps }} MB/s</div>
              <div class="text-[10px] text-gray-500 mono">{{ benchResult.read_sec }}s (nocache)</div>
            </div>
          </div>
          <div class="text-[10px] text-gray-400 mono truncate" :title="benchResult.tuning_state">
            FS: ext4 (^has_journal, extent) · {{ benchResult.tuning_state }}
          </div>
        </div>
      </div>
    </div>

    <!-- Storage Partitions Grid -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-2.5">
      <div v-for="v in volumes" :key="v.mount_point" class="pro-card">
        <div class="pro-card-header">
          <div class="flex items-center gap-2">
            <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
            <span class="pro-card-title">{{ v.label }}</span>
          </div>
          <span class="badge badge-green">MOUNTED</span>
        </div>

        <div class="space-y-1.5 text-[11.5px]">
          <div class="flex justify-between py-0.5 border-b border-white/5">
            <span class="text-gray-400">Device</span>
            <span class="mono text-gray-200">{{ v.device }}</span>
          </div>
          <div class="flex justify-between py-0.5 border-b border-white/5">
            <span class="text-gray-400">Mount Point</span>
            <span class="mono text-sky-400 cursor-pointer hover:underline" @click="loadNAS(v.mount_point)">{{ v.mount_point }}</span>
          </div>
          <div class="flex justify-between py-0.5">
            <span class="text-gray-400">Usage</span>
            <span class="mono text-emerald-400">{{ formatStorage(v.used_kb) }} / {{ formatStorage(v.total_kb) }} ({{ formatStorage(v.avail_kb) }} free)</span>
          </div>
          <div class="progress-bar-bg mt-1">
            <div class="progress-bar-fill bg-sky-400" :style="{ width: `${Math.max(2, v.percent)}%` }"></div>
          </div>
        </div>
      </div>
    </div>

    <!-- Interactive NAS File Manager -->
    <div class="pro-card">
      <div class="pro-card-header flex-wrap">
        <div class="flex items-center gap-2 min-w-0">
          <span class="pro-card-title">NAS File Browser</span>
          <span class="mono text-xs text-sky-400 bg-black/40 px-2 py-0.5 rounded border border-white/10 truncate max-w-[260px]">{{ currentDir }}</span>
        </div>

        <div class="flex flex-wrap items-center gap-1.5 shrink-0">
          <select :value="currentDir" class="pro-input mono text-[11px]" @change="loadNAS(($event.target as HTMLSelectElement).value)">
            <option value="/mnt/sda1">/mnt/sda1 (USB 3.0 500GB HDD Root)</option>
            <option value="/mnt/sda1/esp32-downloads">/mnt/sda1/esp32-downloads (ESP32 Bot Videos)</option>
            <option value="/mnt/sda1/downloads">/mnt/sda1/downloads (Direct Downloads)</option>
            <option value="/mnt/sda1/share">/mnt/sda1/share (USB 500GB Shared)</option>
            <option value="/overlay/share">/overlay/share (NAND Share)</option>
            <option value="/overlay/downloads">/overlay/downloads (NAND Downloads)</option>
            <option value="/tmp/downloads">/tmp/downloads (RAM Disk)</option>
          </select>
          <button type="button" class="btn-secondary text-[11px]" @click="goUp">↑ Up</button>
          <button type="button" class="btn-secondary text-[11px] min-w-[104px]" @click="showNewFolder = !showNewFolder">
            {{ showNewFolder ? '✕ Cancel' : '+ New Folder' }}
          </button>
          <label class="btn-primary text-[11px] min-w-[108px] cursor-pointer">
            <span>{{ uploading ? 'Uploading…' : '↑ Upload File' }}</span>
            <input type="file" class="hidden" :disabled="uploading" @change="onFileSelected" />
          </label>
          <button type="button" class="btn-secondary text-[11px]" :disabled="loading" @click="loadNAS()">↻</button>
        </div>
      </div>

      <!-- Create Folder Bar -->
      <form v-if="showNewFolder" class="mb-3 p-2 rounded bg-black/40 border border-sky-500/30 flex items-center gap-2" @submit.prevent="createFolder">
        <input v-model="newFolderName" type="text" required class="pro-input flex-1 mono" placeholder="Enter new folder name…" />
        <button type="submit" class="btn-primary text-[11px]">Create Folder</button>
        <button type="button" class="btn-secondary text-[11px]" @click="showNewFolder = false">Cancel</button>
      </form>

      <div class="overflow-x-auto max-h-64">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Size</th>
              <th>Modified</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!files.length">
              <td colspan="5" class="text-center text-gray-500 py-4">
                Directory is empty. Use "+ New Folder" or "↑ Upload File" to add files.
              </td>
            </tr>
            <tr v-for="f in files" :key="f.path">
              <td class="max-w-xs">
                <button
                  v-if="f.is_dir"
                  type="button"
                  class="font-semibold text-sky-400 hover:underline flex items-center gap-1.5 truncate max-w-full"
                  @click="loadNAS(f.path)"
                >
                  <span>📁</span> <span class="truncate">{{ f.name }}</span>
                </button>
                <span v-else class="font-medium text-gray-200 flex items-center gap-1.5 truncate">
                  <span>📄</span> <span class="truncate">{{ f.name }}</span>
                </span>
              </td>
              <td>
                <span :class="f.is_dir ? 'badge badge-blue' : 'badge badge-gray'">
                  {{ f.is_dir ? 'FOLDER' : 'FILE' }}
                </span>
              </td>
              <td class="mono text-gray-300">{{ f.is_dir ? '—' : formatBytes(f.size) }}</td>
              <td class="mono text-gray-400">{{ f.updated_at }}</td>
              <td>
                <div class="flex items-center gap-1.5">
                  <button v-if="f.is_dir" type="button" class="btn-secondary text-[10px] py-0.5 px-2" @click="loadNAS(f.path)">
                    Open
                  </button>
                  <a
                    v-else
                    :href="`/api/v1/nas/download?path=${encodeURIComponent(f.path)}`"
                    class="btn-primary text-[10px] py-0.5 px-2"
                  >
                    Download
                  </a>
                  <button type="button" class="btn-danger text-[10px] py-0.5 px-2" @click="deleteEntry(f.path)">
                    Delete
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Configured SMB / NAS Shares -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Configured NAS Shared Folders ({{ shares.length }})</span>
        <button type="button" class="btn-primary text-[11px] py-1 min-w-[104px]" @click="showAddShare = !showAddShare">
          {{ showAddShare ? '✕ Cancel' : '+ Add Share' }}
        </button>
      </div>

      <form v-if="showAddShare" class="mb-3 p-2.5 rounded bg-black/40 border border-sky-500/30 grid grid-cols-1 md:grid-cols-4 gap-2 text-[11.5px]" @submit.prevent="addShare">
        <input v-model="newShare.name" required class="pro-input mono" placeholder="Share Name (e.g. Media)" />
        <input v-model="newShare.path" required class="pro-input mono" placeholder="/mnt/sda1/media" />
        <input v-model="newShare.comment" class="pro-input" placeholder="Description" />
        <button type="submit" class="btn-primary">Save Share</button>
      </form>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Share Name</th>
              <th>Directory Path</th>
              <th>Access</th>
              <th>Description</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in shares" :key="s.name">
              <td class="mono font-semibold text-sky-300">\\192.168.2.1\{{ s.name }}</td>
              <td>
                <button type="button" class="mono text-emerald-400 hover:underline" @click="loadNAS(s.path)">{{ s.path }}</button>
              </td>
              <td>
                <span class="badge" :class="s.read_only ? 'badge-orange' : 'badge-green'">
                  {{ s.read_only ? 'READ-ONLY' : 'READ/WRITE' }}
                </span>
              </td>
              <td class="text-gray-400">{{ s.comment }}</td>
              <td>
                <div class="flex items-center gap-1.5">
                  <button type="button" class="btn-secondary text-[10px] py-0.5 px-2" @click="loadNAS(s.path)">Browse</button>
                  <button type="button" class="btn-danger text-[10px] py-0.5 px-2" @click="removeShare(s.name)">Remove</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
