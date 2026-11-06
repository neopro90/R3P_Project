<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'
import { useSystemStore } from '@/stores/system'

const systemStore = useSystemStore()

interface RadioInfo {
  device: string
  iface_section: string
  band: string
  chip: string
  enabled: boolean
  ssid: string
  encryption: string
  key: string
  channel: string
  htmode: string
  txpower: string
  country: string
}

interface WirelessStation {
  mac: string
  ip: string
  hostname: string
  iface: string
  band: string
  signal: string
  tx_rate: string
}

interface ScannedNetwork {
  ssid: string
  bssid: string
  channel: number
  band: string
  device: string
  signal_dbm: number
  encryption: string
  security: string
}

interface WirelessClientStatus {
  configured: boolean
  enabled: boolean
  connected: boolean
  device: string
  band: string
  ssid: string
  bssid: string
  encryption: string
  ip_address: string
  signal_dbm: string
  proto?: string
  static_ip?: string
  netmask?: string
  gateway?: string
  dns?: string
}

const radios = ref<RadioInfo[]>([])
const stations = ref<WirelessStation[]>([])
const clientStatus = ref<WirelessClientStatus>({
  configured: false,
  enabled: false,
  connected: false,
  device: 'radio0',
  band: '2.4GHz',
  ssid: '',
  bssid: '',
  encryption: 'psk2',
  ip_address: '',
  signal_dbm: '',
  proto: 'dhcp',
})
const scannedNetworks = ref<ScannedNetwork[]>([])
const scanning = ref(false)
const connectingClient = ref(false)
const showClientForm = ref(false)
const clientForm = ref({
  device: 'radio0',
  ssid: '',
  bssid: '',
  encryption: 'psk2',
  key: '',
  enabled: true,
  proto: 'dhcp',
  ipaddr: '',
  netmask: '255.255.255.0',
  gateway: '',
  dns: '1.1.1.1 8.8.8.8',
})

const loading = ref(false)
const savingDevice = ref<string | null>(null)
const restarting = ref(false)
const notice = ref<string | null>(null)
const editingRadio = ref<string | null>(null)

async function loadWireless() {
  loading.value = true
  try {
    const [res, cli] = await Promise.all([
      apiRequest<{ radios: RadioInfo[]; stations: WirelessStation[] }>('/wireless'),
      apiRequest<WirelessClientStatus>('/wireless/client').catch(() => null),
    ])
    if (res) {
      radios.value = res.radios || []
      stations.value = res.stations || []
    }
    if (cli) {
      clientStatus.value = cli
      if (cli.configured && !clientForm.value.ssid) {
        clientForm.value.device = cli.device || 'radio0'
        clientForm.value.ssid = cli.ssid || ''
        clientForm.value.bssid = cli.bssid || ''
        clientForm.value.encryption = cli.encryption || 'psk2'
        clientForm.value.proto = cli.proto || 'dhcp'
        clientForm.value.ipaddr = cli.static_ip || ''
        clientForm.value.netmask = cli.netmask || '255.255.255.0'
        clientForm.value.gateway = cli.gateway || ''
        clientForm.value.dns = cli.dns || '1.1.1.1 8.8.8.8'
      }
    }
  } catch (err: any) {
    notice.value = 'Error loading wireless: ' + (err?.message || err)
  } finally {
    loading.value = false
  }
}

async function scanNearbyWifi(band = 'all') {
  scanning.value = true
  notice.value = 'Đang quét sóng Wi-Fi xung quanh (2.4GHz & 5GHz)…'
  try {
    const res = await apiRequest<{ networks: ScannedNetwork[]; count: number }>(`/wireless/scan?band=${band}`)
    scannedNetworks.value = res?.networks || []
    notice.value = `Đã tìm thấy ${scannedNetworks.value.length} mạng Wi-Fi xung quanh!`
    setTimeout(() => { notice.value = null }, 4000)
  } catch (err: any) {
    notice.value = 'Quét Wi-Fi thất bại: ' + (err?.message || err)
  } finally {
    scanning.value = false
  }
}

function selectNetworkForClient(net: ScannedNetwork) {
  clientForm.value = {
    device: net.device || (net.band === '5GHz' ? 'radio1' : 'radio0'),
    ssid: net.ssid,
    bssid: '',
    encryption: net.encryption || 'psk2',
    key: '',
    enabled: true,
    proto: clientForm.value.proto || 'dhcp',
    ipaddr: clientForm.value.ipaddr || '',
    netmask: clientForm.value.netmask || '255.255.255.0',
    gateway: clientForm.value.gateway || '',
    dns: clientForm.value.dns || '1.1.1.1 8.8.8.8',
  }
  showClientForm.value = true
}

async function saveAndConnectClient() {
  if (!clientForm.value.ssid.trim()) return
  connectingClient.value = true
  notice.value = `Đang kết nối R3P vào Wi-Fi "${clientForm.value.ssid}" làm cổng WAN (WWAN Client)…`
  try {
    await apiRequest('/wireless/client', {
      method: 'POST',
      body: JSON.stringify(clientForm.value),
    })
    showClientForm.value = false
    notice.value = `Đã cấu hình thu Wi-Fi "${clientForm.value.ssid}"! Đang nhận IP và kiểm tra Internet…`
    setTimeout(async () => {
      await loadWireless()
      await systemStore.refreshInternet(true)
    }, 3500)
  } catch (err: any) {
    notice.value = 'Kết nối Wi-Fi Client thất bại: ' + (err?.message || err)
  } finally {
    connectingClient.value = false
  }
}

async function disconnectClient() {
  connectingClient.value = true
  notice.value = 'Đang ngắt kết nối Wi-Fi Client (WWAN)…'
  try {
    await apiRequest('/wireless/client', { method: 'DELETE' })
    notice.value = 'Đã tắt chế độ thu Wi-Fi Client.'
    await loadWireless()
    await systemStore.refreshInternet(true)
  } catch (err: any) {
    notice.value = 'Lỗi khi tắt Wi-Fi Client: ' + (err?.message || err)
  } finally {
    connectingClient.value = false
  }
}

async function saveRadio(r: RadioInfo) {
  savingDevice.value = r.device
  notice.value = null
  try {
    await apiRequest('/wireless', {
      method: 'PUT',
      body: JSON.stringify({
        device: r.device,
        enabled: r.enabled,
        ssid: r.ssid,
        encryption: r.encryption,
        key: r.key,
        channel: r.channel,
        htmode: r.htmode,
        txpower: r.txpower,
        country: r.country,
      }),
    })
    editingRadio.value = null
    notice.value = `${r.band} (${r.ssid}) saved to UCI and wifi reload triggered!`
    await loadWireless()
    setTimeout(() => { notice.value = null }, 3500)
  } catch (err: any) {
    notice.value = 'Save failed: ' + (err?.message || err)
  } finally {
    savingDevice.value = null
  }
}

async function toggleRadioEnabled(r: RadioInfo) {
  r.enabled = !r.enabled
  await saveRadio(r)
}

async function restartWifi() {
  restarting.value = true
  notice.value = 'Reloading wireless driver & hostapd…'
  try {
    await apiRequest('/wireless/restart', { method: 'POST' })
    await loadWireless()
    notice.value = 'Wireless subsystem reloaded successfully!'
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    notice.value = 'Restart failed: ' + (err?.message || err)
  } finally {
    restarting.value = false
  }
}

onMounted(() => {
  void loadWireless()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Top Wireless Action Bar -->
    <div class="flex flex-wrap items-center justify-between gap-2 bg-[#0d1322] border border-white/10 rounded px-3 py-1.5">
      <div class="flex items-center gap-2 text-[11.5px]">
        <span class="font-semibold text-gray-200">MediaTek MT7615N Dual-Band 4x4 MU-MIMO (AP Broadcast + WWAN Client Repeater)</span>
        <span v-if="notice" class="px-2 py-0.5 rounded bg-sky-500/15 text-sky-300 border border-sky-500/30 font-medium">
          {{ notice }}
        </span>
      </div>
      <div class="flex items-center gap-2">
        <button class="btn-secondary text-[11px] py-1" :disabled="loading" @click="loadWireless">
          Refresh Status ↻
        </button>
        <button class="btn-primary text-[11px] py-1" :disabled="restarting" @click="restartWifi">
          {{ restarting ? 'Reloading Wi-Fi…' : 'Reload All Radios' }}
        </button>
      </div>
    </div>

    <!-- Wireless Client / Repeater (Thu Wi-Fi Truy Cập Internet) -->
    <div class="pro-card border border-sky-500/30">
      <div class="pro-card-header">
        <div class="flex items-center gap-2">
          <span
            class="w-2.5 h-2.5 rounded-full"
            :class="clientStatus.connected ? 'bg-emerald-400 animate-pulse' : clientStatus.enabled ? 'bg-amber-400' : 'bg-gray-500'"
          ></span>
          <span class="pro-card-title">Wireless WAN Client / Thu Wi-Fi Làm Internet (STA Mode · WWAN)</span>
          <span
            class="badge"
            :class="clientStatus.connected ? 'badge-green' : clientStatus.enabled ? 'badge-orange' : 'badge-gray'"
          >
            {{ clientStatus.connected ? `CONNECTED (${clientStatus.ip_address})` : clientStatus.enabled ? 'ASSOCIATING…' : 'CLIENT OFF' }}
          </span>
        </div>

        <div class="flex flex-wrap items-center gap-1.5">
          <button
            class="btn-primary text-[11px] py-1"
            :disabled="scanning"
            @click="scanNearbyWifi('all')"
          >
            {{ scanning ? 'Đang quét sóng Wi-Fi…' : '🔍 Quét Wi-Fi Xung Quanh (2.4G & 5G)' }}
          </button>
          <button
            class="btn-secondary text-[11px] py-1"
            @click="showClientForm = !showClientForm"
          >
            {{ showClientForm ? 'Ẩn Khung Nhập' : '+ Nhập Thủ Công Wi-Fi Cần Thu' }}
          </button>
          <button
            v-if="clientStatus.configured && clientStatus.enabled"
            class="btn-danger text-[11px] py-1"
            :disabled="connectingClient"
            @click="disconnectClient"
          >
            Ngắt Thu Wi-Fi
          </button>
        </div>
      </div>

      <!-- Current WWAN Status Summary -->
      <div class="grid grid-cols-2 md:grid-cols-5 gap-2 text-[11.5px] mb-2">
        <div class="p-2 rounded bg-black/30 border border-white/5">
          <div class="text-[10px] text-gray-500 uppercase">Upstream SSID</div>
          <div class="mono font-bold text-sky-300 truncate">{{ clientStatus.ssid || 'Chưa cấu hình' }}</div>
        </div>
        <div class="p-2 rounded bg-black/30 border border-white/5">
          <div class="text-[10px] text-gray-500 uppercase">Radio / Band</div>
          <div class="mono text-gray-200">{{ clientStatus.configured ? `${clientStatus.band} (${clientStatus.device})` : '—' }}</div>
        </div>
        <div class="p-2 rounded bg-black/30 border border-white/5">
          <div class="text-[10px] text-gray-500 uppercase">WWAN Local IP</div>
          <div class="mono text-emerald-400 font-semibold">{{ clientStatus.ip_address || 'DHCP Waiting' }}</div>
        </div>
        <div class="p-2 rounded bg-black/30 border border-white/5">
          <div class="text-[10px] text-gray-500 uppercase">Signal / RSSI</div>
          <div class="mono text-gray-200">{{ clientStatus.signal_dbm || '—' }}</div>
        </div>
        <div class="p-2 rounded bg-black/30 border border-white/5">
          <div class="text-[10px] text-gray-500 uppercase">Security</div>
          <div class="mono text-purple-300 uppercase">{{ clientStatus.encryption || '—' }}</div>
        </div>
      </div>

      <!-- Connect to Upstream Wi-Fi Form -->
      <form
        v-if="showClientForm"
        class="p-3 rounded-lg bg-[#0d1526] border border-sky-500/40 space-y-2.5 text-[12px] mb-3"
        @submit.prevent="saveAndConnectClient"
      >
        <div class="flex items-center justify-between">
          <span class="font-bold text-sky-300 uppercase text-[11px]">Cấu Hình Thu Sóng Wi-Fi Làm Cổng Ra Internet (WWAN STA)</span>
          <button type="button" class="text-gray-400 hover:text-white text-xs" @click="showClientForm = false">✕ Đóng</button>
        </div>
        <div class="grid grid-cols-1 md:grid-cols-4 gap-2.5">
          <div>
            <label class="block text-[10px] text-gray-400 uppercase mb-1">Băng Tần Thu (Radio)</label>
            <select v-model="clientForm.device" class="w-full pro-input mono">
              <option value="radio0">2.4 GHz (radio0)</option>
              <option value="radio1">5 GHz (radio1)</option>
            </select>
          </div>
          <div>
            <label class="block text-[10px] text-gray-400 uppercase mb-1">Tên Wi-Fi Cần Thu (SSID)</label>
            <input v-model="clientForm.ssid" type="text" required class="w-full pro-input mono" placeholder="Nhập tên Wi-Fi gốc…" />
          </div>
          <div>
            <label class="block text-[10px] text-gray-400 uppercase mb-1">Chuẩn Bảo Mật</label>
            <select v-model="clientForm.encryption" class="w-full pro-input mono">
              <option value="psk2">WPA2-PSK (Phổ biến nhất)</option>
              <option value="psk-mixed">WPA/WPA2 Mixed</option>
              <option value="sae-mixed">WPA2/WPA3 Mixed</option>
              <option value="sae">WPA3-SAE</option>
              <option value="none">Open (Không mật khẩu)</option>
            </select>
          </div>
          <div v-if="clientForm.encryption !== 'none'">
            <label class="block text-[10px] text-gray-400 uppercase mb-1">Mật Khẩu Wi-Fi Gốc</label>
            <input v-model="clientForm.key" type="text" class="w-full pro-input mono" placeholder="Nhập mật khẩu Wi-Fi…" />
          </div>
        </div>

        <!-- WWAN IP / Gateway Configuration -->
        <div class="grid grid-cols-1 md:grid-cols-5 gap-2.5 pt-2 border-t border-white/10">
          <div>
            <label class="block text-[10px] text-gray-400 uppercase mb-1">Chế Độ Nhận IP (WWAN)</label>
            <select v-model="clientForm.proto" class="w-full pro-input mono">
              <option value="dhcp">DHCP (Nhận IP & Gateway Tự Động)</option>
              <option value="static">Static IP (Tự đặt IP & Gateway)</option>
            </select>
          </div>
          <div v-if="clientForm.proto === 'static'">
            <label class="block text-[10px] text-gray-400 uppercase mb-1">WWAN IPv4 Address</label>
            <input v-model="clientForm.ipaddr" type="text" class="w-full pro-input mono" placeholder="192.168.1.150" />
          </div>
          <div v-if="clientForm.proto === 'static'">
            <label class="block text-[10px] text-gray-400 uppercase mb-1">Subnet Mask</label>
            <input v-model="clientForm.netmask" type="text" class="w-full pro-input mono" placeholder="255.255.255.0" />
          </div>
          <div>
            <label class="block text-[10px] text-gray-400 uppercase mb-1">IP Gateway (Tùy chọn)</label>
            <input v-model="clientForm.gateway" type="text" class="w-full pro-input mono" placeholder="VD: 192.168.1.1 (Auto nếu trống)" />
          </div>
          <div>
            <label class="block text-[10px] text-gray-400 uppercase mb-1">DNS Servers</label>
            <input v-model="clientForm.dns" type="text" class="w-full pro-input mono" placeholder="1.1.1.1 8.8.8.8" />
          </div>
        </div>
        <div class="flex justify-end gap-2 pt-1">
          <button type="button" class="btn-secondary text-[11px]" @click="showClientForm = false">Hủy</button>
          <button type="submit" class="btn-primary text-[11px]" :disabled="connectingClient">
            {{ connectingClient ? 'Đang áp dụng & Kết nối…' : '✓ Kết Nối & Thu Internet Qua Wi-Fi Này' }}
          </button>
        </div>
      </form>

      <!-- Scanned Nearby Wi-Fi Table -->
      <div v-if="scannedNetworks.length > 0" class="overflow-x-auto max-h-60 border-t border-white/10 pt-2">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Tên Wi-Fi (SSID)</th>
              <th>Băng Tần / Kênh</th>
              <th>Cường Độ Sóng</th>
              <th>Bảo Mật</th>
              <th>BSSID (MAC)</th>
              <th>Thao Tác</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="net in scannedNetworks" :key="net.bssid + net.ssid">
              <td class="font-semibold text-gray-100">{{ net.ssid }}</td>
              <td>
                <span class="badge" :class="net.band === '5GHz' ? 'badge-purple' : 'badge-blue'">
                  {{ net.band }} · CH {{ net.channel || 'Auto' }}
                </span>
              </td>
              <td>
                <span
                  class="mono font-semibold"
                  :class="net.signal_dbm >= -65 ? 'text-emerald-400' : net.signal_dbm >= -78 ? 'text-amber-400' : 'text-red-400'"
                >
                  {{ net.signal_dbm }} dBm
                </span>
              </td>
              <td class="mono text-xs text-gray-300">{{ net.security }}</td>
              <td class="mono text-xs text-gray-500">{{ net.bssid }}</td>
              <td>
                <button
                  class="btn-primary text-[10.5px] py-0.5 px-2.5"
                  @click="selectNetworkForClient(net)"
                >
                  Chọn Thu Wi-Fi Này
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- AP Broadcast Radios Grid -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
      <div v-for="r in radios" :key="r.device" class="pro-card">
        <div class="pro-card-header">
          <div class="flex items-center gap-2">
            <span class="w-2.5 h-2.5 rounded-full" :class="r.enabled ? 'bg-emerald-400' : 'bg-amber-400'"></span>
            <span class="pro-card-title">{{ r.band }} AP · {{ r.chip }}</span>
          </div>
          <span :class="r.enabled ? 'badge badge-green' : 'badge badge-orange'">
            {{ r.enabled ? 'AP BROADCASTING' : 'RADIO DISABLED' }}
          </span>
        </div>

        <!-- Read-Only Summary View -->
        <div v-if="editingRadio !== r.device" class="space-y-2 text-[12px]">
          <div class="flex items-center justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">UCI Device / Section</span>
            <span class="mono text-gray-300">{{ r.device }} / {{ r.iface_section }}</span>
          </div>
          <div class="flex items-center justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Broadcast SSID</span>
            <span class="mono font-semibold text-sky-400">{{ r.ssid }}</span>
          </div>
          <div class="flex items-center justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Channel & Bandwidth</span>
            <span class="mono text-gray-200">Channel {{ r.channel }} ({{ r.htmode }})</span>
          </div>
          <div class="flex items-center justify-between py-1 border-b border-white/5">
            <span class="text-gray-400">Transmit Power</span>
            <span class="mono text-gray-200">{{ r.txpower }} dBm</span>
          </div>
          <div class="flex items-center justify-between py-1">
            <span class="text-gray-400">Security Mode</span>
            <span class="mono text-emerald-400 uppercase">{{ r.encryption }}</span>
          </div>

          <div class="mt-3 pt-2 border-t border-white/5 flex justify-end gap-2">
            <button
              class="btn-secondary text-[11px]"
              :disabled="savingDevice === r.device"
              @click="toggleRadioEnabled(r)"
            >
              {{ r.enabled ? 'Disable AP' : 'Enable AP' }}
            </button>
            <button class="btn-primary text-[11px]" @click="editingRadio = r.device">
              Configure AP Radio
            </button>
          </div>
        </div>

        <!-- Editable Configuration Form -->
        <form v-else class="space-y-2.5 text-[12px]" @submit.prevent="saveRadio(r)">
          <div class="grid grid-cols-2 gap-2">
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">SSID Name</label>
              <input v-model="r.ssid" type="text" required class="w-full pro-input mono" />
            </div>
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Radio State</label>
              <select v-model="r.enabled" class="w-full pro-input">
                <option :value="true">Enabled (Broadcast ON)</option>
                <option :value="false">Disabled (Radio OFF)</option>
              </select>
            </div>
          </div>

          <div class="grid grid-cols-3 gap-2">
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Channel</label>
              <input v-model="r.channel" type="text" class="w-full pro-input mono" placeholder="auto, 1, 6, 36, 149" />
            </div>
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">HT Mode</label>
              <select v-model="r.htmode" class="w-full pro-input mono">
                <option value="HT20">HT20 (20MHz)</option>
                <option value="HT40">HT40 (40MHz)</option>
                <option v-if="r.device === 'radio1'" value="VHT80">VHT80 (80MHz AC)</option>
              </select>
            </div>
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">TX Power (dBm)</label>
              <input v-model="r.txpower" type="number" min="5" max="23" class="w-full pro-input mono" />
            </div>
          </div>

          <div class="grid grid-cols-2 gap-2">
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Encryption</label>
              <select v-model="r.encryption" class="w-full pro-input mono">
                <option value="none">Open (No Password)</option>
                <option value="psk2">WPA2-PSK (CCMP)</option>
                <option value="psk-mixed">WPA/WPA2 Mixed</option>
                <option value="sae-mixed">WPA2/WPA3 Mixed</option>
              </select>
            </div>
            <div v-if="r.encryption !== 'none'">
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Wi-Fi Password (min 8 chars)</label>
              <input v-model="r.key" type="text" minlength="8" class="w-full pro-input mono" placeholder="Enter WPA key" />
            </div>
          </div>

          <div class="pt-2 border-t border-white/5 flex justify-end gap-2">
            <button type="button" class="btn-secondary text-[11px]" @click="editingRadio = null">
              Cancel
            </button>
            <button type="submit" class="btn-primary text-[11px]" :disabled="savingDevice === r.device">
              {{ savingDevice === r.device ? 'Saving & Reloading…' : 'Save & Apply to Router' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Connected Wireless Stations -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Connected Wireless Stations ({{ stations.length }})</span>
      </div>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Hostname</th>
              <th>IP Address</th>
              <th>MAC Address</th>
              <th>Interface / Band</th>
              <th>Signal RSSI</th>
              <th>PHY Bitrate</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!stations.length">
              <td colspan="6" class="text-center text-gray-500 py-4">
                No wireless stations currently associated. Enable a radio above to accept Wi-Fi clients.
              </td>
            </tr>
            <tr v-for="c in stations" :key="c.mac">
              <td class="font-medium text-gray-200">{{ c.hostname }}</td>
              <td class="mono text-sky-300">{{ c.ip }}</td>
              <td class="mono text-gray-400">{{ c.mac }}</td>
              <td><span class="badge badge-blue text-[10px] mono">{{ c.iface }} ({{ c.band }})</span></td>
              <td class="mono text-emerald-400">{{ c.signal }}</td>
              <td class="mono text-gray-300">{{ c.tx_rate }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
