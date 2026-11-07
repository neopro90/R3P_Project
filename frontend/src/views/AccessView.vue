<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { apiRequest } from '@/services/api'

interface StaticLease {
  id: string
  name: string
  mac: string
  ip: string
}

interface ActiveLease {
  expires: number
  mac: string
  ip: string
  hostname: string
  oui_vendor: string
}

interface DDNSConfigModel {
  enabled: boolean
  provider: string
  domain: string
  token: string
  username: string
  custom_url: string
  check_interval: number
  last_ip: string
  last_update: string
  last_status: string
}

const sshConfig = ref({
  port: '22',
  password_auth: true,
  root_password_auth: true,
})

const dhcpConfig = ref({
  start: '100',
  limit: '150',
  leasetime: '12h',
})

const ddnsConfig = ref<DDNSConfigModel>({
  enabled: false,
  provider: 'duckdns',
  domain: '',
  token: '',
  username: '',
  custom_url: '',
  check_interval: 10,
  last_ip: '',
  last_update: 'Chưa cập nhật',
  last_status: 'Chưa kích hoạt',
})

const externalIp = ref('')
const updatingDdns = ref(false)
const savingDdns = ref(false)
const showDdnsToken = ref(false)

const ddnsProviders = [
  { id: 'duckdns', name: '🦆 DuckDNS.org (Ưu tiên / Miễn phí trọn đời)', placeholder: 'subdomain (vd: r3phome)' },
  { id: 'cloudflare', name: '☁️ Cloudflare DNS', placeholder: 'sub.domain.com' },
  { id: 'noip', name: '🌐 No-IP (noip.com)', placeholder: 'myhost.ddns.net' },
  { id: 'dyndns', name: '⚡ DynDNS', placeholder: 'myhost.dyndns.org' },
  { id: 'freedns', name: '🔒 FreeDNS (afraid.org)', placeholder: 'myhost.mooo.com' },
  { id: 'custom', name: '🛠️ Custom URL (Tùy chỉnh máy chủ)', placeholder: 'myhost.example.com' },
]

const staticLeases = ref<StaticLease[]>([])
const activeLeases = ref<ActiveLease[]>([])
const nowUnix = ref(Math.floor(Date.now() / 1000))
const loading = ref(false)
const notice = ref<string | null>(null)

const newPassword = ref('')
const confirmPassword = ref('')
const savingPassword = ref(false)
const savingSSH = ref(false)
const savingDHCP = ref(false)

const showAddStatic = ref(false)
const newStatic = ref({ name: '', mac: '', ip: '192.168.2.' })
const staticCardRef = ref<HTMLElement | null>(null)

function showToast(msg: string, ms = 3800) {
  notice.value = msg
  setTimeout(() => {
    if (notice.value === msg) notice.value = null
  }, ms)
}

async function loadAccess() {
  loading.value = true
  try {
    const res = await apiRequest<{
      ssh: typeof sshConfig.value
      dhcp_lan: typeof dhcpConfig.value
      static_leases: StaticLease[]
      active_leases: ActiveLease[]
      now_unix: number
      ddns?: DDNSConfigModel
      external_ip?: string
    }>('/access')
    if (res) {
      if (res.ssh) sshConfig.value = res.ssh
      if (res.dhcp_lan) {
        dhcpConfig.value = {
          start: res.dhcp_lan.start || '100',
          limit: res.dhcp_lan.limit || '150',
          leasetime: res.dhcp_lan.leasetime || '12h',
        }
      }
      staticLeases.value = res.static_leases || []
      activeLeases.value = res.active_leases || []
      nowUnix.value = res.now_unix || Math.floor(Date.now() / 1000)
      if (res.ddns) {
        ddnsConfig.value = { ...ddnsConfig.value, ...res.ddns }
      }
      if (res.external_ip) {
        externalIp.value = res.external_ip
      }
    }
  } catch (err: any) {
    notice.value = 'Lỗi tải thông tin cấu hình: ' + (err?.message || err)
  } finally {
    loading.value = false
  }
}

async function saveSSH() {
  savingSSH.value = true
  try {
    await apiRequest('/access/ssh', {
      method: 'PUT',
      body: JSON.stringify(sshConfig.value),
    })
    showToast('Đã lưu cấu hình SSH Dropbear thành công!')
  } catch (err: any) {
    showToast('Lỗi lưu SSH: ' + (err?.message || err))
  } finally {
    savingSSH.value = false
  }
}

async function saveDHCP() {
  savingDHCP.value = true
  try {
    await apiRequest('/access/dhcp', {
      method: 'PUT',
      body: JSON.stringify(dhcpConfig.value),
    })
    showToast('Đã lưu dải cấp phát DHCP pool và nạp lại dnsmasq!')
  } catch (err: any) {
    showToast('Lỗi lưu DHCP: ' + (err?.message || err))
  } finally {
    savingDHCP.value = false
  }
}

async function saveDDNS() {
  savingDdns.value = true
  ddnsConfig.value.domain = (ddnsConfig.value.domain || '').trim()
  ddnsConfig.value.token = (ddnsConfig.value.token || '').trim()
  ddnsConfig.value.username = (ddnsConfig.value.username || '').trim()
  ddnsConfig.value.custom_url = (ddnsConfig.value.custom_url || '').trim()
  try {
    const res = await apiRequest<{ message: string; ddns: DDNSConfigModel }>('/access/ddns', {
      method: 'PUT',
      body: JSON.stringify(ddnsConfig.value),
    })
    showToast(res?.message || 'Đã lưu cấu hình DDNS thành công!')
    if (res?.ddns) ddnsConfig.value = res.ddns
  } catch (err: any) {
    showToast('Lỗi lưu DDNS: ' + (err?.message || err))
  } finally {
    savingDdns.value = false
  }
}

async function forceUpdateDDNS() {
  updatingDdns.value = true
  ddnsConfig.value.domain = (ddnsConfig.value.domain || '').trim()
  ddnsConfig.value.token = (ddnsConfig.value.token || '').trim()
  notice.value = 'Đang gửi yêu cầu cập nhật IP tới máy chủ DDNS…'
  try {
    // First save the trimmed values
    await apiRequest<{ message: string; ddns: DDNSConfigModel }>('/access/ddns', {
      method: 'PUT',
      body: JSON.stringify(ddnsConfig.value),
    })
    const res = await apiRequest<{ message: string; ddns: DDNSConfigModel }>('/access/ddns/update', {
      method: 'POST',
    })
    showToast(res?.message || 'Cập nhật DDNS thành công!', 4500)
    if (res?.ddns) ddnsConfig.value = res.ddns
  } catch (err: any) {
    const rawErr = String(err?.message || err).replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
    showToast('Cập nhật DDNS thất bại: ' + rawErr, 5000)
  } finally {
    updatingDdns.value = false
  }
}

async function updatePassword() {
  if (!newPassword.value || newPassword.value !== confirmPassword.value) {
    showToast('Lỗi: Mật khẩu mới và mật khẩu xác nhận không khớp!')
    return
  }
  savingPassword.value = true
  try {
    await apiRequest('/access/password', {
      method: 'POST',
      body: JSON.stringify({ password: newPassword.value }),
    })
    showToast('Đã cập nhật mật khẩu root/admin Router R3P thành công!')
    newPassword.value = ''
    confirmPassword.value = ''
  } catch (err: any) {
    showToast('Lỗi cập nhật mật khẩu: ' + (err?.message || err))
  } finally {
    savingPassword.value = false
  }
}

async function addStaticLease() {
  if (!newStatic.value.name || !newStatic.value.mac || !newStatic.value.ip) {
    showToast('Lỗi: Vui lòng điền đủ Tên, Địa chỉ MAC và Địa chỉ IP!')
    return
  }
  try {
    await apiRequest('/access/static-lease', {
      method: 'POST',
      body: JSON.stringify(newStatic.value),
    })
    showToast(`Đã ghim IP cố định cho ${newStatic.value.name} (${newStatic.value.ip})!`)
    showAddStatic.value = false
    newStatic.value = { name: '', mac: '', ip: '192.168.2.' }
    await loadAccess()
  } catch (err: any) {
    showToast('Lỗi thêm ghim IP: ' + (err?.message || err))
  }
}

async function deleteStaticLease(id: string) {
  if (!confirm('Bạn có chắc chắn muốn xóa ghim IP này không?')) return
  try {
    await apiRequest('/access/static-lease', {
      method: 'DELETE',
      body: JSON.stringify({ id }),
    })
    showToast('Đã xóa ghim IP cố định!')
    await loadAccess()
  } catch (err: any) {
    showToast('Lỗi xóa ghim IP: ' + (err?.message || err))
  }
}

function pinActiveClient(client: ActiveLease) {
  newStatic.value = {
    name: client.hostname || client.oui_vendor || 'Device',
    mac: client.mac,
    ip: client.ip,
  }
  showAddStatic.value = true
  setTimeout(() => {
    staticCardRef.value?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }, 50)
}

function formatRemainingTime(expiresSec: number): string {
  const diff = expiresSec - nowUnix.value
  if (diff <= 0) return 'Hết hạn'
  const h = Math.floor(diff / 3600)
  const m = Math.floor((diff % 3600) / 60)
  const s = diff % 60
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

onMounted(() => {
  void loadAccess()
})
</script>

<template>
  <div class="p-4 space-y-4 max-w-7xl mx-auto">
    <!-- Floating Non-Shifting Notice Toast Bar -->
    <div
      v-if="notice"
      class="r3p-toast"
      :class="notice.includes('Lỗi') || notice.includes('Error') || notice.includes('thất bại')
        ? 'r3p-toast-error'
        : 'r3p-toast-success'"
    >
      <span class="r3p-toast-text">{{ notice }}</span>
      <button type="button" class="text-xs opacity-75 hover:opacity-100 px-1 cursor-pointer" @click="notice = null">✕</button>
    </div>

    <!-- Row 1: SSH, DHCP Server, Admin Password -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-3.5">
      <!-- 1. SSH Dropbear Config -->
      <form class="pro-card flex flex-col justify-between p-3.5" @submit.prevent="saveSSH">
        <div>
          <div class="pro-card-header pb-2 mb-2 border-b border-white/10">
            <span class="pro-card-title">SSH Server (Dropbear)</span>
            <span class="badge badge-green">RUNNING</span>
          </div>
          <div class="space-y-2.5 text-[13px]">
            <div class="flex items-center justify-between">
              <span class="text-gray-300 font-medium">Cổng Lắng Nghe (Port)</span>
              <input v-model="sshConfig.port" type="number" min="1" max="65535" class="pro-input w-24 mono text-right font-bold text-sky-400" />
            </div>
            <label class="flex items-center justify-between py-1 cursor-pointer">
              <span class="text-gray-300 font-medium">Xác thực Mật khẩu</span>
              <input v-model="sshConfig.password_auth" type="checkbox" class="w-4 h-4 accent-sky-500" />
            </label>
            <label class="flex items-center justify-between py-1 cursor-pointer">
              <span class="text-gray-300 font-medium">Cho phép đăng nhập root</span>
              <input v-model="sshConfig.root_password_auth" type="checkbox" class="w-4 h-4 accent-sky-500" />
            </label>
          </div>
        </div>
        <div class="mt-4 pt-2.5 border-t border-white/10 flex justify-end">
          <button type="submit" :disabled="savingSSH" class="btn-primary text-xs min-w-[145px]">
            {{ savingSSH ? 'Đang lưu SSH…' : '💾 Lưu Cấu Hình SSH' }}
          </button>
        </div>
      </form>

      <!-- 2. DHCP Server Pool Config -->
      <form class="pro-card flex flex-col justify-between p-3.5" @submit.prevent="saveDHCP">
        <div>
          <div class="pro-card-header pb-2 mb-2 border-b border-white/10">
            <span class="pro-card-title">DHCP Server (dnsmasq)</span>
            <span class="badge badge-green">192.168.2.1/24</span>
          </div>
          <div class="space-y-2.5 text-[13px]">
            <div class="flex items-center justify-between">
              <span class="text-gray-300 font-medium">IP Khởi Đầu (Start .X)</span>
              <input v-model="dhcpConfig.start" type="number" min="2" max="250" class="pro-input w-24 mono text-right font-bold text-emerald-400" />
            </div>
            <div class="flex items-center justify-between">
              <span class="text-gray-300 font-medium">Số lượng cấp tối đa (Limit)</span>
              <input v-model="dhcpConfig.limit" type="number" min="1" max="250" class="pro-input w-24 mono text-right font-bold text-emerald-400" />
            </div>
            <div class="flex items-center justify-between">
              <span class="text-gray-300 font-medium">Thời hạn thuê (Lease time)</span>
              <input v-model="dhcpConfig.leasetime" type="text" class="pro-input w-24 mono text-right font-bold text-gray-200" placeholder="12h" />
            </div>
          </div>
        </div>
        <div class="mt-4 pt-2.5 border-t border-white/10 flex justify-end">
          <button type="submit" :disabled="savingDHCP" class="btn-primary text-xs min-w-[152px]">
            {{ savingDHCP ? 'Đang lưu DHCP…' : '💾 Lưu Cấu Hình DHCP' }}
          </button>
        </div>
      </form>

      <!-- 3. Admin Password Change -->
      <form class="pro-card flex flex-col justify-between p-3.5" @submit.prevent="updatePassword">
        <div>
          <div class="pro-card-header pb-2 mb-2 border-b border-white/10">
            <span class="pro-card-title">Mật Khẩu Quản Trị (Root)</span>
            <span class="badge badge-blue">WEB + SSH</span>
          </div>
          <div class="space-y-2 text-[13px]">
            <div>
              <label class="block text-[11.5px] text-gray-400 uppercase font-semibold mb-1">Mật khẩu mới (root)</label>
              <input v-model="newPassword" type="password" required minlength="4" class="w-full pro-input mono" placeholder="Nhập mật khẩu mới" />
            </div>
            <div>
              <label class="block text-[11.5px] text-gray-400 uppercase font-semibold mb-1">Xác nhận mật khẩu</label>
              <input v-model="confirmPassword" type="password" required minlength="4" class="w-full pro-input mono" placeholder="Nhập lại mật khẩu mới" />
            </div>
          </div>
        </div>
        <div class="mt-4 pt-2.5 border-t border-white/10 flex justify-end">
          <button type="submit" :disabled="savingPassword" class="btn-primary text-xs min-w-[152px]">
            {{ savingPassword ? 'Đang cập nhật…' : '🔑 Đổi Mật Khẩu Root' }}
          </button>
        </div>
      </form>
    </div>

    <!-- Row 2: Dynamic DNS (DDNS) Configuration Card (DuckDNS.org Priority) -->
    <div class="pro-card p-4 space-y-3.5 border border-sky-500/25 shadow-xl">
      <div class="flex flex-wrap items-center justify-between gap-2 pb-2.5 border-b border-white/10">
        <div class="flex items-center gap-2.5">
          <span class="text-xl">🦆</span>
          <div>
            <h2 class="text-[15px] font-bold text-gray-100 flex items-center gap-2">
              Cấu Hình Tên Miền Động DDNS (Dynamic DNS)
              <span class="badge badge-blue text-[11px] font-bold uppercase">Tự Động Cập Nhật IP</span>
            </h2>
            <p class="text-xs text-gray-400">
              Định danh tên miền Internet cố định cho Router R3P khi dùng mạng gia đình (IP động). Tự động đồng bộ IP WAN/4G lên máy chủ.
            </p>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <span class="badge badge-green text-xs font-mono">
            IP Ngoài: {{ externalIp || ddnsConfig.last_ip || 'Đang phát hiện…' }}
          </span>
          <label class="flex items-center gap-2 cursor-pointer bg-black/30 px-3 py-1.5 rounded-lg border border-white/10">
            <span class="text-xs font-bold" :class="ddnsConfig.enabled ? 'text-emerald-400' : 'text-gray-400'">
              {{ ddnsConfig.enabled ? 'ĐANG BẬT DDNS' : 'ĐANG TẮT DDNS' }}
            </span>
            <input v-model="ddnsConfig.enabled" type="checkbox" class="w-4 h-4 accent-emerald-500" />
          </label>
        </div>
      </div>

      <!-- DDNS Settings Fields -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-3.5 text-[13px]">
        <!-- Provider Select -->
        <div>
          <label class="block text-xs font-bold text-gray-300 uppercase mb-1">Máy Chủ / Nhà Cung Cấp DDNS</label>
          <select v-model="ddnsConfig.provider" class="w-full pro-input font-medium text-sky-300">
            <option v-for="p in ddnsProviders" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
          <p class="text-[11px] text-gray-400 mt-1">Khuyên dùng <strong>DuckDNS.org</strong>: Hoàn toàn miễn phí, không bị hết hạn tên miền.</p>
        </div>

        <!-- Domain / Subdomain Input -->
        <div>
          <label class="block text-xs font-bold text-gray-300 uppercase mb-1">Tên Miền (Domain / Subdomain)</label>
          <div class="flex items-center gap-1">
            <input
              v-model.trim="ddnsConfig.domain"
              type="text"
              class="w-full pro-input mono font-bold text-emerald-400"
              :placeholder="ddnsProviders.find(p => p.id === ddnsConfig.provider)?.placeholder || 'subdomain'"
            />
            <span v-if="ddnsConfig.provider === 'duckdns'" class="text-xs text-gray-400 mono font-semibold shrink-0">
              .duckdns.org
            </span>
          </div>
          <p class="text-[11px] text-gray-400 mt-1">
            {{ ddnsConfig.provider === 'duckdns' ? 'Chỉ cần điền tên phụ (vd: r3phome)' : 'Điền đầy đủ tên miền FQDN' }}
          </p>
        </div>

        <!-- Token / Secret Key Input -->
        <div>
          <div class="flex items-center justify-between mb-1">
            <label class="text-xs font-bold text-gray-300 uppercase">Token / Khóa Bảo Mật (API Key)</label>
            <button type="button" class="text-[11px] text-sky-400 hover:underline" @click="showDdnsToken = !showDdnsToken">
              {{ showDdnsToken ? 'Ẩn token' : 'Hiện token' }}
            </button>
          </div>
          <input
            v-model.trim="ddnsConfig.token"
            :type="showDdnsToken ? 'text' : 'password'"
            class="w-full pro-input mono text-gray-200"
            placeholder="Nhập Token lấy từ DuckDNS hoặc API Key"
          />
          <p class="text-[11px] text-gray-400 mt-1">Lấy tại trang quản trị tài khoản DuckDNS hoặc nhà cung cấp DNS.</p>
        </div>
      </div>

      <!-- Extra fields for Custom / Username / Interval -->
      <div v-if="ddnsConfig.provider === 'custom'" class="p-3 rounded-lg bg-black/30 border border-white/10 space-y-2">
        <label class="block text-xs font-bold text-amber-300 uppercase">URL Cập Nhật Tùy Chỉnh (Custom HTTP GET URL)</label>
        <input
          v-model="ddnsConfig.custom_url"
          type="text"
          class="w-full pro-input mono text-xs text-sky-300"
          placeholder="https://update.example.com/nic/update?hostname=[DOMAIN]&myip=[IP]&key=[TOKEN]"
        />
        <p class="text-[11px] text-gray-400">
          Các biến thay thế tự động: <code>[DOMAIN]</code>, <code>[TOKEN]</code>, <code>[IP]</code>, <code>[USERNAME]</code>.
        </p>
      </div>

      <!-- Status & Action Footer -->
      <div class="flex flex-wrap items-center justify-between gap-3 pt-2 border-t border-white/10 text-xs">
        <div class="flex items-center gap-2 min-w-0 flex-1 overflow-hidden">
          <span class="text-gray-400 shrink-0">Trạng thái gần nhất:</span>
          <span
            class="font-bold mono truncate block max-w-[360px] lg:max-w-[500px]"
            :class="ddnsConfig.last_status.includes('Thành công') || ddnsConfig.last_status.includes('OK') ? 'text-emerald-400' : 'text-amber-300'"
            :title="ddnsConfig.last_status"
          >
            {{ (ddnsConfig.last_status || 'Chưa cập nhật').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim() }}
          </span>
          <span class="text-gray-500 font-mono shrink-0">· {{ ddnsConfig.last_update }}</span>
        </div>

        <div class="flex items-center gap-2 shrink-0">
          <button
            type="button"
            class="btn-secondary text-xs py-1.5 px-3 min-w-[184px] flex items-center gap-1.5 cursor-pointer"
            :disabled="updatingDdns"
            @click="forceUpdateDDNS"
          >
            <span>⚡</span>
            <span>{{ updatingDdns ? 'Đang cập nhật IP…' : 'Cập Nhật IP Lên DDNS Ngay' }}</span>
          </button>
          <button
            type="button"
            class="btn-primary text-xs py-1.5 px-3.5 min-w-[156px] flex items-center gap-1.5 cursor-pointer shadow-sm font-bold"
            :disabled="savingDdns"
            @click="saveDDNS"
          >
            <span>💾</span>
            <span>{{ savingDdns ? 'Đang lưu…' : 'Lưu Cấu Hình DDNS' }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Row 3: Active DHCP Client Leases -->
    <div class="pro-card p-4 space-y-3">
      <div class="pro-card-header flex items-center justify-between pb-2 border-b border-white/10">
        <div class="flex items-center gap-2">
          <span class="pro-card-title">Thiết Bị Đang Kết Nối (Active DHCP Leases)</span>
          <span class="badge badge-green">{{ activeLeases.length }} thiết bị</span>
        </div>
        <button type="button" class="btn-secondary text-xs py-1 px-3" :disabled="loading" @click="loadAccess">Làm mới ↻</button>
      </div>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Tên Thiết Bị (Hostname)</th>
              <th>Địa Chỉ IP</th>
              <th>Địa Chỉ MAC</th>
              <th>Nhà Sản Xuất (OUI)</th>
              <th>Thời Gian Thuê Còn Lại</th>
              <th class="text-right">Hành Động</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!activeLeases.length">
              <td colspan="6" class="text-center text-gray-400 py-4 text-xs">
                Chưa có thiết bị nào đang nhận IP DHCP từ Router.
              </td>
            </tr>
            <tr v-for="c in activeLeases" :key="c.mac">
              <td class="font-bold text-gray-100">{{ c.hostname || '—' }}</td>
              <td class="mono font-bold text-sky-400">{{ c.ip }}</td>
              <td class="mono text-gray-300 font-medium">{{ c.mac }}</td>
              <td class="text-gray-300">{{ c.oui_vendor || 'Generic' }}</td>
              <td class="mono text-emerald-400 font-semibold">{{ formatRemainingTime(c.expires) }}</td>
              <td class="text-right">
                <button
                  type="button"
                  class="btn-secondary text-xs py-1 px-2.5 font-semibold cursor-pointer"
                  title="Ghim địa chỉ IP này cố định cho thiết bị"
                  @click="pinActiveClient(c)"
                >
                  📌 Ghim IP
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Row 4: Static DHCP Reservations -->
    <div ref="staticCardRef" class="pro-card p-4 space-y-3">
      <div class="pro-card-header flex items-center justify-between pb-2 border-b border-white/10">
        <div class="flex items-center gap-2">
          <span class="pro-card-title">Danh Sách IP Cố Định Đã Ghim (Static Reservations)</span>
          <span class="badge badge-blue">{{ staticLeases.length }} quy tắc</span>
        </div>
        <button type="button" class="btn-primary text-xs py-1 px-3 min-w-[162px]" @click="showAddStatic = !showAddStatic">
          {{ showAddStatic ? '✕ Hủy Bỏ' : '+ Ghim Thêm IP Cố Định' }}
        </button>
      </div>

      <!-- Add Static Lease Form -->
      <form v-if="showAddStatic" class="p-3.5 rounded-lg bg-black/40 border border-sky-500/30 grid grid-cols-1 md:grid-cols-4 gap-3 text-xs items-end" @submit.prevent="addStaticLease">
        <div>
          <label class="block text-gray-300 font-semibold mb-1">Tên Thiết Bị</label>
          <input v-model="newStatic.name" type="text" required class="w-full pro-input" placeholder="vd: Camera-Sank" />
        </div>
        <div>
          <label class="block text-gray-300 font-semibold mb-1">Địa Chỉ MAC</label>
          <input v-model="newStatic.mac" type="text" required class="w-full pro-input mono" placeholder="AA:BB:CC:DD:EE:FF" />
        </div>
        <div>
          <label class="block text-gray-300 font-semibold mb-1">Địa Chỉ IP Muốn Ghim</label>
          <input v-model="newStatic.ip" type="text" required class="w-full pro-input mono font-bold text-sky-400" placeholder="192.168.2.X" />
        </div>
        <div>
          <button type="submit" class="btn-primary w-full py-1.5 text-xs font-bold">Xác Nhận Ghim IP</button>
        </div>
      </form>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Tên Thiết Bị</th>
              <th>Địa Chỉ MAC</th>
              <th>Địa Chỉ IP Được Ghim</th>
              <th class="text-right">Thao Tác</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!staticLeases.length">
              <td colspan="4" class="text-center text-gray-400 py-4 text-xs">
                Chưa có quy tắc ghim IP cố định nào. Bấm "📌 Ghim IP" ở bảng trên hoặc "+ Ghim Thêm IP Cố Định".
              </td>
            </tr>
            <tr v-for="s in staticLeases" :key="s.id">
              <td class="font-bold text-gray-100">{{ s.name }}</td>
              <td class="mono text-gray-300 font-medium">{{ s.mac }}</td>
              <td class="mono font-bold text-emerald-400">{{ s.ip }}</td>
              <td class="text-right">
                <button
                  type="button"
                  class="btn-danger text-xs py-1 px-2.5 font-semibold cursor-pointer"
                  title="Xóa ghim IP cố định này"
                  @click="deleteStaticLease(s.id)"
                >
                  Xóa
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
