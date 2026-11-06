<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useNetworkStore } from '@/stores/network'
import { useSystemStore } from '@/stores/system'
import { apiRequest } from '@/services/api'

const network = useNetworkStore()
const systemStore = useSystemStore()

interface RouteEntry {
  destination: string
  gateway: string
  device: string
  source: string
  raw: string
}

interface ArpEntry {
  ip: string
  mac: string
  device: string
  flags: string
  vendor: string
}

interface NetworkInterfaceConfig {
  name: string
  label: string
  device: string
  proto: string
  ipaddr: string
  netmask: string
  gateway: string
  dns: string[]
  dns_string?: string
  mtu: string
  metric: string
  macaddr: string
  username: string
  password: string
  dhcp_enabled: boolean
  dhcp_start: string
  dhcp_limit: string
  dhcp_leasetime: string
  live_up: boolean
  live_ipv4: string
  live_mac: string
  live_gateway: string
}

const routes = ref<RouteEntry[]>([])
const dnsServers = ref<string[]>([])
const arpTable = ref<ArpEntry[]>([])
const ifaceConfigs = ref<NetworkInterfaceConfig[]>([])
const defaultGateway = ref('')
const defaultDevice = ref('br-lan')

const showEditGateway = ref(false)
const quickGatewayForm = ref({
  gateway: '',
  device: 'br-lan',
  dns_string: '1.1.1.1 8.8.8.8',
})
const savingGateway = ref(false)

const editingIface = ref<string | null>(null)
const savingIface = ref<string | null>(null)
const notice = ref<string | null>(null)
const errorMsg = ref<string | null>(null)
const loading = ref(false)

const pingHost = ref('1.1.1.1')
const pingCount = ref(4)
const pingRunning = ref(false)
const pingOutput = ref('')

const dnsQuery = ref('openwrt.org')
const dnsRunning = ref(false)
const dnsOutput = ref('')

async function loadNetworkDetails() {
  loading.value = true
  try {
    await network.refreshInterfaces()
    await network.refreshClients()
    const [det, cfgRes] = await Promise.all([
      apiRequest<{
        routes: RouteEntry[]
        dns_servers: string[]
        arp_table: ArpEntry[]
      }>('/network/details').catch(() => null),
      apiRequest<{
        interfaces: NetworkInterfaceConfig[]
        default_gateway: string
        default_device: string
        active_dns: string[]
      }>('/network/interfaces-config').catch(() => null),
    ])

    if (det) {
      routes.value = det.routes || []
      dnsServers.value = det.dns_servers || []
      arpTable.value = det.arp_table || []
    }
    if (cfgRes) {
      ifaceConfigs.value = (cfgRes.interfaces || []).map((it) => ({
        ...it,
        dns_string: (it.dns || []).join(' '),
      }))
      defaultGateway.value = cfgRes.default_gateway || ''
      defaultDevice.value = cfgRes.default_device || 'br-lan'
      quickGatewayForm.value.gateway = cfgRes.default_gateway || ''
      quickGatewayForm.value.device = cfgRes.default_device || 'br-lan'
      if (cfgRes.active_dns?.length) {
        quickGatewayForm.value.dns_string = cfgRes.active_dns.join(' ')
      }
    }
  } catch (err: any) {
    console.warn('Could not load network details:', err)
  } finally {
    loading.value = false
  }
}

async function saveQuickGateway() {
  savingGateway.value = true
  notice.value = null
  errorMsg.value = null
  try {
    const res = await apiRequest<{
      saved: boolean
      message: string
      interfaces: NetworkInterfaceConfig[]
      default_gateway: string
      default_device: string
    }>('/network/interfaces-config', {
      method: 'PUT',
      body: JSON.stringify({
        name: 'gateway_quick',
        gateway: quickGatewayForm.value.gateway,
        device: quickGatewayForm.value.device,
        dns_string: quickGatewayForm.value.dns_string,
      }),
    })
    notice.value = res?.message || 'Đã cập nhật Default Gateway & DNS!'
    showEditGateway.value = false
    setTimeout(async () => {
      await loadNetworkDetails()
      await systemStore.refreshInternet(true)
    }, 1500)
  } catch (err: any) {
    errorMsg.value = 'Lỗi lưu Gateway: ' + (err?.message || err)
  } finally {
    savingGateway.value = false
  }
}

async function saveInterfaceConfig(cfg: NetworkInterfaceConfig) {
  savingIface.value = cfg.name
  notice.value = null
  errorMsg.value = null
  try {
    const res = await apiRequest<{
      saved: boolean
      message: string
      interfaces: NetworkInterfaceConfig[]
    }>('/network/interfaces-config', {
      method: 'PUT',
      body: JSON.stringify({
        ...cfg,
        dns_string: cfg.dns_string || (cfg.dns || []).join(' '),
      }),
    })
    editingIface.value = null
    notice.value = res?.message || `Đã lưu cấu hình cổng ${cfg.name.toUpperCase()}!`
    setTimeout(async () => {
      await loadNetworkDetails()
      await systemStore.refreshInternet(true)
    }, 2000)
  } catch (err: any) {
    errorMsg.value = `Lỗi lưu cổng ${cfg.name.toUpperCase()}: ` + (err?.message || err)
  } finally {
    savingIface.value = null
  }
}

async function triggerInterfaceAction(name: string, action: 'ifup' | 'ifdown' | 'reload') {
  notice.value = `Đang thực thi ${action} trên interface ${name.toUpperCase()}…`
  try {
    const res = await apiRequest<{ message: string }>('/network/interfaces-config/action', {
      method: 'POST',
      body: JSON.stringify({ name, action }),
    })
    notice.value = res?.message || 'Đã thực thi thành công!'
    setTimeout(async () => {
      await loadNetworkDetails()
      await systemStore.refreshInternet(true)
    }, 2500)
  } catch (err: any) {
    errorMsg.value = 'Lỗi thao tác interface: ' + (err?.message || err)
  }
}

async function runPing() {
  if (pingRunning.value || !pingHost.value.trim()) return
  pingRunning.value = true
  pingOutput.value = `Executing ping -c ${pingCount.value} ${pingHost.value} on router…`
  try {
    const res = await apiRequest<{ output: string; success: boolean }>(
      `/system/ping?host=${encodeURIComponent(pingHost.value.trim())}&count=${pingCount.value}`,
    )
    pingOutput.value = res.output || 'No output returned.'
  } catch (err: any) {
    pingOutput.value = 'Ping error: ' + (err?.message || err)
  } finally {
    pingRunning.value = false
  }
}

async function runNslookup() {
  if (dnsRunning.value || !dnsQuery.value.trim()) return
  dnsRunning.value = true
  dnsOutput.value = `Executing nslookup ${dnsQuery.value} on router…`
  try {
    const res = await apiRequest<{ output: string; success: boolean }>(
      `/system/nslookup?host=${encodeURIComponent(dnsQuery.value.trim())}`,
    )
    dnsOutput.value = res.output || 'No output returned.'
  } catch (err: any) {
    dnsOutput.value = 'DNS lookup error: ' + (err?.message || err)
  } finally {
    dnsRunning.value = false
  }
}

function formatBytes(b: number): string {
  if (!b) return '0 B'
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  return `${(b / (1024 * 1024)).toFixed(2)} MB`
}

onMounted(() => {
  void loadNetworkDetails()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Top Status & Quick Gateway / DNS Editor -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-2">
      <div class="stat-mini">
        <div class="flex items-center justify-between">
          <span class="text-[10.5px] text-gray-400 uppercase tracking-wider">LAN IPv4 Address</span>
          <button
            class="text-[10px] text-sky-400 hover:underline font-semibold"
            @click="editingIface = 'lan'"
          >
            ✎ Đổi IP LAN
          </button>
        </div>
        <div class="mono text-sm font-bold text-sky-400 mt-0.5">
          {{ ifaceConfigs.find(i => i.name === 'lan')?.ipaddr || '192.168.2.1' }}
        </div>
        <div class="text-[10px] text-gray-500">Bridge br-lan (DSA Switch)</div>
      </div>

      <div class="stat-mini">
        <div class="flex items-center justify-between">
          <span class="text-[10.5px] text-gray-400 uppercase tracking-wider">Default IP Gateway</span>
          <button
            class="text-[10px] text-emerald-400 hover:underline font-semibold"
            @click="showEditGateway = !showEditGateway"
          >
            {{ showEditGateway ? '✕ Đóng' : '✎ Edit IP Gateway' }}
          </button>
        </div>
        <div class="mono text-sm font-bold text-emerald-400 mt-0.5">
          {{ defaultGateway || ifaceConfigs.find(i => i.name === 'lan')?.gateway || 'Chưa đặt Gateway' }}
        </div>
        <div class="text-[10px] text-gray-500">Qua cổng: {{ defaultDevice || 'auto' }}</div>
      </div>

      <div class="stat-mini">
        <div class="flex items-center justify-between">
          <span class="text-[10.5px] text-gray-400 uppercase tracking-wider">DNS Resolvers</span>
          <button
            class="text-[10px] text-purple-400 hover:underline font-semibold"
            @click="showEditGateway = !showEditGateway"
          >
            ✎ Edit DNS
          </button>
        </div>
        <div class="mono text-sm font-bold text-purple-400 truncate mt-0.5">
          {{ dnsServers.join(' · ') || '1.1.1.1 · 8.8.8.8' }}
        </div>
        <div class="text-[10px] text-gray-500">Active /etc/resolv.conf</div>
      </div>

      <div class="stat-mini">
        <div class="flex items-center justify-between">
          <span class="text-[10.5px] text-gray-400 uppercase tracking-wider">ARP & Interfaces</span>
          <button class="text-[10px] text-amber-400 hover:underline" @click="loadNetworkDetails">↻ Làm mới</button>
        </div>
        <div class="mono text-sm font-bold text-amber-400 mt-0.5">
          {{ network.interfaces.length }} Ifaces · {{ arpTable.length }} ARP
        </div>
        <div class="text-[10px] text-gray-500">Kernel Netifd & Switch</div>
      </div>
    </div>

    <!-- Notices -->
    <div v-if="notice" class="px-3 py-2 rounded bg-emerald-500/15 border border-emerald-500/30 text-emerald-300 text-xs font-medium">
      ✓ {{ notice }}
    </div>
    <div v-if="errorMsg" class="px-3 py-2 rounded bg-red-500/15 border border-red-500/30 text-red-300 text-xs font-medium">
      ⚠ {{ errorMsg }}
    </div>

    <!-- Quick Edit IP Gateway & DNS Panel -->
    <form
      v-if="showEditGateway"
      class="pro-card border border-emerald-500/40 bg-[#0c1624] space-y-2.5"
      @submit.prevent="saveQuickGateway"
    >
      <div class="pro-card-header">
        <span class="pro-card-title text-emerald-300">🌐 Chỉnh Sửa Nhanh IP Default Gateway & Máy Chủ DNS</span>
        <button type="button" class="text-xs text-gray-400 hover:text-white" @click="showEditGateway = false">✕ Đóng</button>
      </div>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-2.5 text-xs">
        <div>
          <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Địa Chỉ IP Default Gateway (Modem / Router Chính)</label>
          <input
            v-model="quickGatewayForm.gateway"
            type="text"
            class="w-full pro-input mono"
            placeholder="VD: 192.168.1.1 hoặc 192.168.2.254"
          />
        </div>
        <div>
          <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Cổng Định Tuyến (Uplink Interface)</label>
          <select v-model="quickGatewayForm.device" class="w-full pro-input mono">
            <option value="br-lan">br-lan (Khi nối dây vào cổng LAN)</option>
            <option value="wan">wan (Khi nối dây vào cổng WAN)</option>
            <option value="phy0-sta0">phy0-sta0 (Wi-Fi Client 2.4GHz)</option>
            <option value="phy1-sta0">phy1-sta0 (Wi-Fi Client 5GHz)</option>
            <option value="usb0">usb0 (USB Tethering / 4G)</option>
          </select>
        </div>
        <div>
          <label class="block text-[10.5px] text-gray-400 uppercase mb-1">DNS Servers (Cách nhau bởi khoảng trắng)</label>
          <input
            v-model="quickGatewayForm.dns_string"
            type="text"
            class="w-full pro-input mono"
            placeholder="1.1.1.1 8.8.8.8"
          />
        </div>
      </div>
      <div class="flex justify-end gap-2 pt-1">
        <button type="button" class="btn-secondary text-xs" @click="showEditGateway = false">Hủy</button>
        <button type="submit" class="btn-primary text-xs" :disabled="savingGateway">
          {{ savingGateway ? 'Đang áp dụng…' : '✓ Lưu & Áp Dụng Gateway / DNS' }}
        </button>
      </div>
    </form>

    <!-- Full LAN / WAN / WWAN / USBWAN Interface Configuration Cards -->
    <div class="pro-card border border-sky-500/30">
      <div class="pro-card-header">
        <div class="flex items-center gap-2">
          <span class="pro-card-title">🔌 Cấu Hình Cổng Mạng LAN / WAN / WWAN / USBWAN (UCI Network Interfaces)</span>
          <span class="badge badge-blue">4 LOGICAL INTERFACES</span>
        </div>
        <div class="flex items-center gap-1.5">
          <button
            class="btn-secondary text-[11px] py-1"
            @click="triggerInterfaceAction('all', 'reload')"
          >
            🔄 Reload Network (/etc/init.d/network reload)
          </button>
        </div>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div
          v-for="cfg in ifaceConfigs"
          :key="cfg.name"
          class="p-3 rounded-lg bg-black/35 border border-white/10 space-y-2"
        >
          <!-- Card Header -->
          <div class="flex items-center justify-between border-b border-white/10 pb-2">
            <div class="flex items-center gap-2">
              <span
                class="w-2.5 h-2.5 rounded-full"
                :class="cfg.live_up ? 'bg-emerald-400' : 'bg-gray-500'"
              ></span>
              <div>
                <div class="font-bold text-xs text-white uppercase">{{ cfg.name }} — {{ cfg.label }}</div>
                <div class="text-[10.5px] text-gray-400 mono">Device: {{ cfg.device }} · Proto: {{ cfg.proto.toUpperCase() }}</div>
              </div>
            </div>
            <div class="flex items-center gap-1.5">
              <span class="badge" :class="cfg.live_up ? 'badge-green' : 'badge-gray'">
                {{ cfg.live_up ? `UP (${cfg.live_ipv4 || 'Active'})` : 'STANDBY' }}
              </span>
              <button
                class="btn-primary text-[10.5px] py-0.5 px-2.5"
                @click="editingIface = editingIface === cfg.name ? null : cfg.name"
              >
                {{ editingIface === cfg.name ? 'Đóng' : '⚙ Cấu Hình' }}
              </button>
            </div>
          </div>

          <!-- Summary Read View -->
          <div v-if="editingIface !== cfg.name" class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-[11px]">
            <div>
              <div class="text-gray-500 text-[10px] uppercase">Chế Độ (Proto)</div>
              <div class="mono font-semibold text-sky-300 uppercase">{{ cfg.proto }}</div>
            </div>
            <div>
              <div class="text-gray-500 text-[10px] uppercase">IPv4 Address</div>
              <div class="mono font-semibold text-gray-100">{{ cfg.live_ipv4 || cfg.ipaddr || 'DHCP Auto' }}</div>
            </div>
            <div>
              <div class="text-gray-500 text-[10px] uppercase">Gateway</div>
              <div class="mono text-emerald-400">{{ cfg.gateway || defaultGateway || 'Auto' }}</div>
            </div>
            <div>
              <div class="text-gray-500 text-[10px] uppercase">DNS / DHCP</div>
              <div class="mono text-purple-300 truncate">
                {{ cfg.name === 'lan' ? (cfg.dhcp_enabled ? `DHCP .${cfg.dhcp_start}` : 'DHCP OFF') : ((cfg.dns || []).join(', ') || 'Auto DNS') }}
              </div>
            </div>

            <div class="col-span-2 sm:col-span-4 flex justify-end gap-1.5 pt-1 border-t border-white/5">
              <button
                class="btn-secondary text-[10px] py-0.5 px-2"
                @click="triggerInterfaceAction(cfg.name, 'ifup')"
              >
                ↻ Kết nối lại (ifup {{ cfg.name }})
              </button>
              <button
                v-if="cfg.name !== 'lan'"
                class="btn-danger text-[10px] py-0.5 px-2"
                @click="triggerInterfaceAction(cfg.name, 'ifdown')"
              >
                ⏸ Ngắt (ifdown)
              </button>
              <button
                class="btn-primary text-[10px] py-0.5 px-2.5"
                @click="editingIface = cfg.name"
              >
                ✎ Chỉnh Sửa IP / Gateway / Giao Thức
              </button>
            </div>
          </div>

          <!-- Full Edit Form for this Interface -->
          <form v-else class="space-y-2.5 pt-1 text-xs" @submit.prevent="saveInterfaceConfig(cfg)">
            <div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
              <div>
                <label class="block text-[10px] text-gray-400 uppercase mb-1">Giao Thức (Protocol)</label>
                <select v-model="cfg.proto" class="w-full pro-input mono">
                  <option value="static">Static IP (IP Tĩnh)</option>
                  <option value="dhcp">DHCP Client (Nhận IP Tự Động)</option>
                  <option v-if="cfg.name === 'wan'" value="pppoe">PPPoE (Quay số nhà mạng)</option>
                </select>
              </div>

              <div>
                <label class="block text-[10px] text-gray-400 uppercase mb-1">Physical Device</label>
                <input v-model="cfg.device" type="text" class="w-full pro-input mono" placeholder="br-lan / wan / usb0" />
              </div>

              <div>
                <label class="block text-[10px] text-gray-400 uppercase mb-1">MTU / Metric</label>
                <div class="flex gap-1">
                  <input v-model="cfg.mtu" type="text" class="w-1/2 pro-input mono" placeholder="1500" title="MTU" />
                  <input v-model="cfg.metric" type="text" class="w-1/2 pro-input mono" placeholder="10" title="Route Metric" />
                </div>
              </div>
            </div>

            <!-- Static IP Fields -->
            <div v-if="cfg.proto === 'static'" class="grid grid-cols-1 sm:grid-cols-3 gap-2">
              <div>
                <label class="block text-[10px] text-gray-400 uppercase mb-1">IPv4 Address</label>
                <input v-model="cfg.ipaddr" type="text" required class="w-full pro-input mono" placeholder="192.168.2.1" />
              </div>
              <div>
                <label class="block text-[10px] text-gray-400 uppercase mb-1">Subnet Mask</label>
                <input v-model="cfg.netmask" type="text" required class="w-full pro-input mono" placeholder="255.255.255.0" />
              </div>
              <div>
                <label class="block text-[10px] text-gray-400 uppercase mb-1">IPv4 Gateway</label>
                <input v-model="cfg.gateway" type="text" class="w-full pro-input mono" placeholder="VD: 192.168.1.1 (Tùy chọn)" />
              </div>
            </div>

            <!-- DHCP Optional Gateway Override -->
            <div v-else-if="cfg.proto === 'dhcp'" class="grid grid-cols-1 sm:grid-cols-2 gap-2">
              <div>
                <label class="block text-[10px] text-gray-400 uppercase mb-1">Custom Gateway Override (Tùy chọn)</label>
                <input v-model="cfg.gateway" type="text" class="w-full pro-input mono" placeholder="Để trống nhận tự động từ DHCP" />
              </div>
              <div>
                <label class="block text-[10px] text-gray-400 uppercase mb-1">MAC Address Clone (Tùy chọn)</label>
                <input v-model="cfg.macaddr" type="text" class="w-full pro-input mono" placeholder="AA:BB:CC:DD:EE:FF" />
              </div>
            </div>

            <!-- PPPoE Fields -->
            <div v-else-if="cfg.proto === 'pppoe'" class="grid grid-cols-1 sm:grid-cols-2 gap-2">
              <div>
                <label class="block text-[10px] text-gray-400 uppercase mb-1">Tài Khoản PPPoE (Username)</label>
                <input v-model="cfg.username" type="text" required class="w-full pro-input mono" placeholder="Tài khoản nhà mạng…" />
              </div>
              <div>
                <label class="block text-[10px] text-gray-400 uppercase mb-1">Mật Khẩu PPPoE (Password)</label>
                <input v-model="cfg.password" type="password" class="w-full pro-input mono" placeholder="Mật khẩu…" />
              </div>
            </div>

            <!-- Custom DNS Servers -->
            <div>
              <label class="block text-[10px] text-gray-400 uppercase mb-1">Custom DNS Servers (Cách nhau dấu cách)</label>
              <input v-model="cfg.dns_string" type="text" class="w-full pro-input mono" placeholder="1.1.1.1 8.8.8.8" />
            </div>

            <!-- LAN DHCP Server Config -->
            <div v-if="cfg.name === 'lan'" class="p-2.5 rounded bg-black/40 border border-white/10 space-y-2">
              <label class="flex items-center gap-2 text-xs text-sky-300 font-semibold cursor-pointer">
                <input v-model="cfg.dhcp_enabled" type="checkbox" class="rounded" />
                <span>Bật Máy Chủ Cấp IP Tự Động (DHCP Server trên cổng LAN)</span>
              </label>
              <div v-if="cfg.dhcp_enabled" class="grid grid-cols-3 gap-2">
                <div>
                  <label class="block text-[10px] text-gray-400 uppercase mb-1">IP Bắt Đầu (Start)</label>
                  <input v-model="cfg.dhcp_start" type="number" class="w-full pro-input mono" placeholder="100" />
                </div>
                <div>
                  <label class="block text-[10px] text-gray-400 uppercase mb-1">Số Lượng IP (Limit)</label>
                  <input v-model="cfg.dhcp_limit" type="number" class="w-full pro-input mono" placeholder="150" />
                </div>
                <div>
                  <label class="block text-[10px] text-gray-400 uppercase mb-1">Thời Hạn (Lease Time)</label>
                  <input v-model="cfg.dhcp_leasetime" type="text" class="w-full pro-input mono" placeholder="12h" />
                </div>
              </div>
            </div>

            <div class="flex justify-end gap-2 pt-1.5 border-t border-white/10">
              <button type="button" class="btn-secondary text-[11px]" @click="editingIface = null">Hủy</button>
              <button type="submit" class="btn-primary text-[11px]" :disabled="savingIface === cfg.name">
                {{ savingIface === cfg.name ? 'Đang lưu & áp dụng…' : `✓ Lưu Cấu Hình ${cfg.name.toUpperCase()}` }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>

    <!-- Live Network Interfaces Telemetry Table -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Live Network Interfaces (/proc/net/dev & Netifd)</span>
        <button class="btn-secondary text-[11px] py-1" :disabled="loading" @click="loadNetworkDetails">
          {{ loading ? 'Refreshing…' : 'Refresh Telemetry ↻' }}
        </button>
      </div>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Interface</th>
              <th>Type</th>
              <th>Status</th>
              <th>MAC Address</th>
              <th>IPv4 Address</th>
              <th>RX Total</th>
              <th>TX Total</th>
              <th>Live Rate</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="iface in network.interfaces" :key="iface.name">
              <td class="font-semibold mono text-gray-200">{{ iface.name }}</td>
              <td><span class="badge badge-gray uppercase text-[9.5px]">{{ iface.type }}</span></td>
              <td>
                <span :class="iface.up ? 'badge badge-green' : 'badge badge-gray'">
                  {{ iface.up ? 'UP' : 'DOWN' }}
                </span>
              </td>
              <td class="mono text-gray-400">{{ iface.mac || '—' }}</td>
              <td class="mono text-sky-300">{{ iface.ipv4 || '—' }}</td>
              <td class="mono text-gray-300">{{ formatBytes(iface.rxBytes) }}</td>
              <td class="mono text-gray-300">{{ formatBytes(iface.txBytes) }}</td>
              <td class="mono text-[11px]">
                <span class="text-emerald-400">↓ {{ formatBytes(iface.rxRate) }}/s</span>
                <span class="text-gray-600 mx-1">·</span>
                <span class="text-sky-400">↑ {{ formatBytes(iface.txRate) }}/s</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Network Diagnostics Section (Ping + DNS Lookup) -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-2.5">
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">ICMP Ping Diagnostic Tool</span>
        </div>
        <div class="space-y-2">
          <div class="flex items-center gap-2">
            <input
              v-model="pingHost"
              type="text"
              class="pro-input flex-1 mono text-[12px]"
              placeholder="Host or IP (e.g. 1.1.1.1 or google.com)"
              @keyup.enter="runPing"
            />
            <select v-model="pingCount" class="pro-input mono text-[12px]">
              <option :value="2">2 Pkts</option>
              <option :value="4">4 Pkts</option>
              <option :value="8">8 Pkts</option>
            </select>
            <button :disabled="pingRunning" class="btn-primary text-[11.5px]" @click="runPing">
              {{ pingRunning ? 'Pinging…' : 'Run Ping' }}
            </button>
          </div>
          <pre class="bg-black/50 border border-white/5 rounded p-2 mono text-[10.5px] text-gray-300 h-32 overflow-y-auto leading-tight whitespace-pre-wrap">{{ pingOutput || 'Enter an IP or domain and click Run Ping.' }}</pre>
        </div>
      </div>

      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">DNS Lookup Tool (nslookup)</span>
        </div>
        <div class="space-y-2">
          <div class="flex items-center gap-2">
            <input
              v-model="dnsQuery"
              type="text"
              class="pro-input flex-1 mono text-[12px]"
              placeholder="Domain or IP (e.g. openwrt.org)"
              @keyup.enter="runNslookup"
            />
            <button :disabled="dnsRunning" class="btn-primary text-[11.5px]" @click="runNslookup">
              {{ dnsRunning ? 'Querying…' : 'Lookup DNS' }}
            </button>
          </div>
          <pre class="bg-black/50 border border-white/5 rounded p-2 mono text-[10.5px] text-gray-300 h-32 overflow-y-auto leading-tight whitespace-pre-wrap">{{ dnsOutput || 'Enter a domain or hostname and click Lookup DNS.' }}</pre>
        </div>
      </div>
    </div>

    <!-- Routing Table & ARP Table -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-2.5">
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">Kernel IP Routing Table (ip route)</span>
        </div>
        <div class="overflow-x-auto max-h-44">
          <table class="pro-table">
            <thead>
              <tr>
                <th>Destination</th>
                <th>Gateway</th>
                <th>Device</th>
                <th>Source IP</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!routes.length">
                <td colspan="4" class="text-center text-gray-500 py-3">No active routes</td>
              </tr>
              <tr v-for="(r, idx) in routes" :key="idx">
                <td class="mono text-sky-300">{{ r.destination }}</td>
                <td class="mono text-gray-300">{{ r.gateway }}</td>
                <td class="mono text-emerald-400">{{ r.device || '—' }}</td>
                <td class="mono text-gray-400">{{ r.source || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title">ARP Neighbor Table (/proc/net/arp)</span>
        </div>
        <div class="overflow-x-auto max-h-44">
          <table class="pro-table">
            <thead>
              <tr>
                <th>IPv4 Address</th>
                <th>MAC Address</th>
                <th>Vendor OUI</th>
                <th>Interface</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!arpTable.length">
                <td colspan="4" class="text-center text-gray-500 py-3">No ARP neighbors found</td>
              </tr>
              <tr v-for="a in arpTable" :key="a.ip + a.mac">
                <td class="mono text-sky-300">{{ a.ip }}</td>
                <td class="mono text-gray-300">{{ a.mac }}</td>
                <td><span class="badge badge-gray text-[10px]">{{ a.vendor }}</span></td>
                <td class="mono text-emerald-400">{{ a.device }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>
