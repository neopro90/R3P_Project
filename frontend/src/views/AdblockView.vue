<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'
import { copyToClipboard as copyText } from '@/composables/useClipboard'

interface AdblockSource {
  id: string
  name: string
  url: string
  category: string
  enabled: boolean
  domain_count: number
  last_updated: string
  status: string
}

interface AdblockConfig {
  enabled: boolean
  force_dns: boolean
  expose_upstream_dns?: boolean
  safe_search: boolean
  block_mode: string
  auto_update: boolean
  update_schedule: string
  query_logging: boolean
  upstream_dns: string[]
  sources: AdblockSource[]
  whitelist: string[]
  blacklist: string[]
  total_blocked: number
  last_rebuild: string
}

const config = ref<AdblockConfig>({
  enabled: true,
  force_dns: true,
  expose_upstream_dns: false,
  safe_search: false,
  block_mode: 'zero_ip',
  auto_update: true,
  update_schedule: '0 3 * * *',
  query_logging: false,
  upstream_dns: ['1.1.1.1', '8.8.8.8'],
  sources: [],
  whitelist: [],
  blacklist: [],
  total_blocked: 0,
  last_rebuild: '—',
})

const activeRules = ref(0)
const blockedHits = ref(0)
const recentLogs = ref<string[]>([])
const logFilter = ref('')

interface GatewaySetupInfo {
  is_client_mode: boolean
  r3p_client_ip: string
  r3p_client_iface: string
  upstream_gateway_ip: string
  r3p_lan_ip: string
  wan_dns_listening: boolean
}

interface AdStreamEvent {
  id: string
  domain: string
  category: string
  client_ip: string
  action: string
  pattern: string
  timestamp: string
}

const streams = ref<AdStreamEvent[]>([])
const streamStats = ref({ blocked: 0, analyzed: 0, video_ads: 0, trackers: 0 })
const loadingStreams = ref(false)

async function loadAdStreams() {
  loadingStreams.value = true
  try {
    const res = await apiRequest<{
      streams: AdStreamEvent[]
      stats: { blocked: number; analyzed: number; video_ads: number; trackers: number }
    }>('/adblock/streams')
    if (res?.streams) {
      streams.value = res.streams
    }
    if (res?.stats) {
      streamStats.value = res.stats
    }
  } catch (e) {
    console.warn(e)
  } finally {
    loadingStreams.value = false
  }
}

async function blockDomainFromStream(domain: string) {
  try {
    const res = await apiRequest<{ blocked: boolean }>('/adblock/streams', {
      method: 'POST',
      body: JSON.stringify({ domain }),
    })
    if (res?.blocked) {
      showNotice(`Đã nhận diện & thêm tên miền luồng quảng cáo "${domain}" vào Blacklist!`)
      await loadAdStreams()
      await loadAdblock()
    }
  } catch (err: any) {
    showError('Lỗi: ' + (err?.message || err))
  }
}

async function clearStreamHistory() {
  try {
    await apiRequest('/adblock/streams', { method: 'DELETE' })
    showNotice('Đã xóa lịch sử luồng phân tích!')
    await loadAdStreams()
  } catch (e: any) {
    showError('Lỗi xóa lịch sử: ' + (e?.message || e))
  }
}

const gatewaySetup = ref<GatewaySetupInfo>({
  is_client_mode: false,
  r3p_client_ip: '',
  r3p_client_iface: '',
  upstream_gateway_ip: '',
  r3p_lan_ip: '',
  wan_dns_listening: false,
})
const copiedText = ref('')

async function copyToClipboard(txt: string, label: string) {
  const ok = await copyText(txt)
  if (ok) {
    copiedText.value = label
    setTimeout(() => { copiedText.value = '' }, 2000)
  }
}

const loading = ref(false)
const saving = ref(false)
const updatingLists = ref(false)
const notice = ref<string | null>(null)
const errorMsg = ref<string | null>(null)

function showNotice(msg: string, ms = 3800) {
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

const upstreamDnsInput = ref('1.1.1.1, 8.8.8.8')

const showAddSource = ref(false)
const newSource = ref({
  name: '',
  url: '',
  category: 'Ads & Trackers',
})

const newBlacklistDomain = ref('')
const newWhitelistDomain = ref('')

const testDomainInput = ref('doubleclick.net')
const testingDomain = ref(false)
const testResult = ref<{
  domain: string
  blocked: boolean
  in_hosts_db: boolean
  dns_output: string
} | null>(null)

const clearingDnsLogs = ref(false)
async function clearDnsLogs() {
  if (!confirm('Bạn có muốn xóa sạch toàn bộ nhật ký truy vấn DNS?')) return
  clearingDnsLogs.value = true
  try {
    await apiRequest('/logs', { method: 'DELETE' })
    recentLogs.value = []
    await loadAdblock()
  } catch (err: any) {
    alert('Lỗi xóa log: ' + (err?.message || err))
  } finally {
    clearingDnsLogs.value = false
  }
}

const filteredLogs = computed(() => {
  const q = logFilter.value.trim().toLowerCase()
  if (!q) return recentLogs.value
  return recentLogs.value.filter((l) => l.toLowerCase().includes(q))
})

async function loadAdblock() {
  loading.value = true
  errorMsg.value = null
  try {
    const res = await apiRequest<{
      config: AdblockConfig
      updating: boolean
      active_rules: number
      blocked_hits: number
      recent_logs: string[]
      gateway_setup?: GatewaySetupInfo
    }>('/adblock')
    if (res?.config) {
      config.value = res.config
      upstreamDnsInput.value = (res.config.upstream_dns || ['1.1.1.1', '8.8.8.8']).join(', ')
    }
    if (res?.gateway_setup) {
      gatewaySetup.value = res.gateway_setup
    }
    activeRules.value = res?.active_rules ?? config.value.total_blocked
    blockedHits.value = res?.blocked_hits ?? 0
    recentLogs.value = res?.recent_logs || []
  } catch (err: any) {
    errorMsg.value = 'Lỗi tải trạng thái AdBlock: ' + (err?.message || err)
  } finally {
    loading.value = false
  }
}

async function saveGlobalConfig() {
  saving.value = true
  notice.value = null
  errorMsg.value = null
  try {
    const dnsArr = upstreamDnsInput.value
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean)
    const res = await apiRequest<{
      saved: boolean
      total_blocked: number
      config: AdblockConfig
      message: string
    }>('/adblock/config', {
      method: 'PUT',
      body: JSON.stringify({
        enabled: config.value.enabled,
        force_dns: config.value.force_dns,
        expose_upstream_dns: Boolean(config.value.expose_upstream_dns),
        safe_search: config.value.safe_search,
        block_mode: config.value.block_mode,
        auto_update: config.value.auto_update,
        update_schedule: config.value.update_schedule,
        query_logging: config.value.query_logging,
        upstream_dns: dnsArr,
      }),
    })
    if (res?.config) config.value = res.config
    notice.value = res?.message || 'Đã lưu cài đặt AdBlock thành công!'
    await loadAdblock()
    setTimeout(() => { notice.value = null }, 4000)
  } catch (err: any) {
    errorMsg.value = 'Lưu cài đặt thất bại: ' + (err?.message || err)
  } finally {
    saving.value = false
  }
}

async function toggleAdblockMaster() {
  config.value.enabled = !config.value.enabled
  await saveGlobalConfig()
}

async function updateBlocklistsNow() {
  updatingLists.value = true
  notice.value = 'Đang tải và tổng hợp danh sách tên miền quảng cáo/mã độc từ các nguồn (vui lòng đợi 5-15 giây)…'
  errorMsg.value = null
  try {
    const res = await apiRequest<{
      updated: boolean
      total_blocked: number
      config: AdblockConfig
      message: string
    }>('/adblock/update', { method: 'POST' })
    if (res?.config) config.value = res.config
    showNotice(res?.message || `Đã cập nhật ${res?.total_blocked || 0} tên miền chặn!`, 4500)
    await loadAdblock()
  } catch (err: any) {
    showError('Cập nhật danh sách lỗi: ' + (err?.message || err))
  } finally {
    updatingLists.value = false
  }
}

async function addSourceList() {
  if (!newSource.value.name.trim() || !newSource.value.url.trim()) return
  try {
    const res = await apiRequest<{ config: AdblockConfig; message: string }>('/adblock/lists', {
      method: 'POST',
      body: JSON.stringify(newSource.value),
    })
    if (res?.config) config.value = res.config
    newSource.value = { name: '', url: '', category: 'Ads & Trackers' }
    showAddSource.value = false
    showNotice(res?.message || 'Đã thêm nguồn danh sách chặn!')
  } catch (err: any) {
    showError('Thêm danh sách thất bại: ' + (err?.message || err))
  }
}

async function toggleSource(src: AdblockSource) {
  try {
    const res = await apiRequest<{ config: AdblockConfig }>('/adblock/lists', {
      method: 'PATCH',
      body: JSON.stringify({ id: src.id, enabled: !src.enabled }),
    })
    if (res?.config) config.value = res.config
    showNotice(`Đã ${!src.enabled ? 'bật' : 'tắt'} nguồn "${src.name}"!`)
  } catch (err: any) {
    showError('Lỗi: ' + (err?.message || err))
  }
}

async function deleteSource(id: string) {
  try {
    const res = await apiRequest<{ config: AdblockConfig }>(`/adblock/lists?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
    if (res?.config) config.value = res.config
    showNotice('Đã xóa nguồn danh sách chặn!')
  } catch (err: any) {
    showError('Lỗi xóa danh sách: ' + (err?.message || err))
  }
}

async function addCustomDomain(listType: 'blacklist' | 'whitelist') {
  const raw = listType === 'blacklist' ? newBlacklistDomain.value : newWhitelistDomain.value
  if (!raw.trim()) return
  try {
    const res = await apiRequest<{ config: AdblockConfig; total_blocked: number; message: string }>('/adblock/domains', {
      method: 'POST',
      body: JSON.stringify({ list_type: listType, domain: raw }),
    })
    if (res?.config) config.value = res.config
    if (listType === 'blacklist') newBlacklistDomain.value = ''
    else newWhitelistDomain.value = ''
    showNotice(res?.message || 'Đã cập nhật danh sách tên miền!')
    await loadAdblock()
  } catch (err: any) {
    showError('Lỗi thêm tên miền: ' + (err?.message || err))
  }
}

async function removeCustomDomain(listType: 'blacklist' | 'whitelist', domain: string) {
  try {
    const res = await apiRequest<{ config: AdblockConfig }>(
      `/adblock/domains?list_type=${listType}&domain=${encodeURIComponent(domain)}`,
      { method: 'DELETE' },
    )
    if (res?.config) config.value = res.config
    showNotice(`Đã xóa "${domain}" khỏi ${listType === 'blacklist' ? 'Blacklist' : 'Whitelist'}!`)
    await loadAdblock()
  } catch (err: any) {
    showError('Lỗi xóa tên miền: ' + (err?.message || err))
  }
}

async function runDomainTest() {
  if (!testDomainInput.value.trim() || testingDomain.value) return
  testingDomain.value = true
  try {
    const res = await apiRequest<{
      domain: string
      blocked: boolean
      in_hosts_db: boolean
      dns_output: string
    }>('/adblock/test', {
      method: 'POST',
      body: JSON.stringify({ domain: testDomainInput.value.trim() }),
    })
    testResult.value = res
  } catch (err: any) {
    errorMsg.value = 'Lỗi kiểm tra tên miền: ' + (err?.message || err)
  } finally {
    testingDomain.value = false
  }
}

onMounted(() => {
  void loadAdblock()
  void loadAdStreams()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Floating Non-Shifting Notices -->
    <div v-if="notice" class="r3p-toast r3p-toast-success">
      <span class="r3p-toast-text">✓ {{ notice }}</span>
      <button type="button" class="text-xs opacity-80 hover:opacity-100 px-1 cursor-pointer" @click="notice = null">✕</button>
    </div>
    <div v-if="errorMsg" class="r3p-toast r3p-toast-error">
      <span class="r3p-toast-text">⚠ {{ errorMsg }}</span>
      <button type="button" class="text-xs opacity-80 hover:opacity-100 px-1 cursor-pointer" @click="errorMsg = null">✕</button>
    </div>

    <!-- Top Banner & Master Controls -->
    <div class="pro-card border border-sky-500/30">
      <div class="pro-card-header flex-wrap">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <span
            class="w-3 h-3 rounded-full shrink-0"
            :class="config.enabled ? 'bg-emerald-400 animate-pulse' : 'bg-red-400'"
          ></span>
          <span class="pro-card-title text-sm truncate">R3P AdBlock DNS Shield (Chặn Quảng Cáo & Mã Độc)</span>
          <span class="badge" :class="config.enabled ? 'badge-green' : 'badge-orange'">
            {{ config.enabled ? 'ADBLOCK ACTIVE' : 'ADBLOCK DISABLED' }}
          </span>
          <span class="badge" :class="config.force_dns ? 'badge-blue' : 'badge-gray'">
            FORCE DNS: {{ config.force_dns ? 'ON' : 'OFF' }}
          </span>
        </div>

        <div class="flex flex-wrap items-center gap-2 shrink-0">
          <button
            type="button"
            class="text-[11.5px] py-1 px-3 min-w-[138px] rounded font-semibold transition"
            :class="config.enabled ? 'btn-danger' : 'btn-primary'"
            :disabled="saving"
            @click="toggleAdblockMaster"
          >
            {{ config.enabled ? '⏸ Tắt AdBlock' : '▶ Bật AdBlock Ngay' }}
          </button>
          <button
            type="button"
            class="btn-primary text-[11.5px] py-1 px-3 min-w-[218px]"
            :disabled="updatingLists"
            @click="updateBlocklistsNow"
          >
            {{ updatingLists ? '⏳ Đang Tải & Cập Nhật…' : '🔄 Cập Nhật Danh Sách Chặn Ngay' }}
          </button>
          <button type="button" class="btn-secondary text-[11.5px] py-1" :disabled="loading" @click="loadAdblock">
            ↻ Làm mới
          </button>
        </div>
      </div>

      <!-- Summary KPI Strip -->
      <div class="grid grid-cols-2 md:grid-cols-5 gap-3 mt-1.5">
        <div class="p-3 rounded-lg bg-black/35 border border-white/10 space-y-1">
          <div class="text-[12px] font-bold text-gray-300 uppercase tracking-wide">Tổng Tên Miền Đang Chặn</div>
          <div class="mono text-xl font-black text-emerald-400">{{ (activeRules || config.total_blocked).toLocaleString() }}</div>
          <div class="text-[12px] text-gray-400 font-medium">Nạp trong /tmp/r3p-adblock.hosts</div>
        </div>
        <div class="p-3 rounded-lg bg-black/35 border border-white/10 space-y-1">
          <div class="text-[12px] font-bold text-gray-300 uppercase tracking-wide">Nguồn Blocklist Đang Bật</div>
          <div class="mono text-xl font-black text-sky-400">
            {{ config.sources.filter(s => s.enabled).length }} / {{ config.sources.length }}
          </div>
          <div class="text-[12px] text-gray-400 font-medium">Hỗ trợ thêm URL tùy chỉnh</div>
        </div>
        <div class="p-3 rounded-lg bg-black/35 border border-white/10 space-y-1">
          <div class="text-[12px] font-bold text-gray-300 uppercase tracking-wide">Danh Sách Đen / Trắng</div>
          <div class="mono text-xl font-black text-purple-400">
            {{ config.blacklist.length }} Chặn · {{ config.whitelist.length }} Cho phép
          </div>
          <div class="text-[12px] text-gray-400 font-medium">Quy tắc tùy chỉnh của Admin</div>
        </div>
        <div class="p-3 rounded-lg bg-black/35 border border-white/10 space-y-1">
          <div class="text-[12px] font-bold text-gray-300 uppercase tracking-wide">Upstream DNS</div>
          <div class="mono text-base font-bold text-amber-400 truncate">{{ (config.upstream_dns || []).join(' · ') }}</div>
          <div class="text-[12px] text-gray-400 font-medium">Máy chủ phân giải DNS gốc</div>
        </div>
        <div class="p-3 rounded-lg bg-black/35 border border-white/10 space-y-1">
          <div class="text-[12px] font-bold text-gray-300 uppercase tracking-wide">Cập Nhật Lần Cuối</div>
          <div class="mono text-sm font-bold text-gray-200">{{ config.last_rebuild || '—' }}</div>
          <div class="text-[12px] text-gray-400 font-medium">Auto Cron: {{ config.auto_update ? config.update_schedule : 'Tắt' }}</div>
        </div>
      </div>
    </div>

    <!-- Gateway DNS Setup Guide (When R3P operates as a Client on Upstream Router) -->
    <div class="pro-card border border-emerald-500/35 bg-gradient-to-r from-emerald-500/[0.06] to-sky-500/[0.05]">
      <div class="pro-card-header flex-wrap gap-2">
        <div class="flex items-center gap-2 min-w-0">
          <span class="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse shrink-0"></span>
          <span class="pro-card-title text-emerald-300 truncate">
            💡 Gợi Ý Setup DNS Cho Gateway Chính (Do R3P Đang Chạy Chế Độ Client · {{ gatewaySetup.r3p_client_iface || 'WWAN/WAN' }})
          </span>
          <span class="badge badge-green">PORT 53 WAN/WWAN: ĐÃ MỞ SẴN</span>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <a
            v-if="gatewaySetup.upstream_gateway_ip"
            :href="`http://${gatewaySetup.upstream_gateway_ip}`"
            target="_blank"
            rel="noopener"
            class="btn-primary text-[11px] py-1 px-2.5"
          >
            🔗 Mở Trang Quản Trị Gateway Chính ({{ gatewaySetup.upstream_gateway_ip }})
          </a>
        </div>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-3 gap-3 text-[11.5px]">
        <!-- Box 1: Live IP Addresses of R3P DNS Server -->
        <div class="p-3 rounded-lg bg-black/40 border border-white/10 space-y-2">
          <div class="font-bold text-sky-300 uppercase text-[11px]">1. Địa Chỉ IP Máy Chủ DNS Của R3P</div>
          <p class="text-gray-400 text-[11px]">
            R3P đã tự động mở cổng DNS <code class="mono text-emerald-300">53 (TCP/UDP)</code> trên cả mạng chính và mạng LAN nội bộ:
          </p>
          <div class="flex items-center justify-between p-2 rounded bg-emerald-500/10 border border-emerald-500/30">
            <div class="min-w-0 pr-2">
              <div class="text-[10px] text-emerald-300 uppercase font-semibold truncate">IP R3P Trên Mạng Gateway Chính ({{ gatewaySetup.r3p_client_iface || 'WWAN' }})</div>
              <div class="mono text-sm font-bold text-white">{{ gatewaySetup.r3p_client_ip || '192.168.1.6' }}</div>
            </div>
            <button
              type="button"
              class="btn-primary text-[10.5px] py-1 px-2.5 min-w-[96px]"
              @click="copyToClipboard(gatewaySetup.r3p_client_ip || '192.168.1.6', 'client_ip')"
            >
              {{ copiedText === 'client_ip' ? '✓ Đã Copy!' : '📋 Copy IP' }}
            </button>
          </div>
          <div class="flex items-center justify-between p-2 rounded bg-sky-500/10 border border-sky-500/25">
            <div class="min-w-0 pr-2">
              <div class="text-[10px] text-sky-300 uppercase font-semibold truncate">IP R3P Trên Mạng LAN Nội Bộ (br-lan)</div>
              <div class="mono text-sm font-bold text-white">{{ gatewaySetup.r3p_lan_ip || '192.168.2.1' }}</div>
            </div>
            <button
              type="button"
              class="btn-secondary text-[10.5px] py-1 px-2.5 min-w-[96px]"
              @click="copyToClipboard(gatewaySetup.r3p_lan_ip || '192.168.2.1', 'lan_ip')"
            >
              {{ copiedText === 'lan_ip' ? '✓ Đã Copy!' : '📋 Copy IP' }}
            </button>
          </div>
        </div>

        <!-- Box 2: How to configure on Main Gateway Router -->
        <div class="p-3 rounded-lg bg-black/40 border border-white/10 space-y-1.5">
          <div class="font-bold text-emerald-300 uppercase text-[11px]">
            2. Cách Cài Trên Modem / Gateway Chính ({{ gatewaySetup.upstream_gateway_ip || '192.168.1.1' }})
          </div>
          <p class="text-gray-300 text-[11px]">
            Để <strong>toàn bộ điện thoại, TV, laptop đang bắt Wi-Fi của Modem chính</strong> đều được R3P lọc sạch quảng cáo:
          </p>
          <ol class="list-decimal list-inside space-y-1 text-gray-300 text-[11px]">
            <li>
              Truy cập Modem chính tại
              <a
                :href="`http://${gatewaySetup.upstream_gateway_ip || '192.168.1.1'}`"
                target="_blank"
                rel="noopener"
                class="mono text-sky-400 underline font-semibold"
              >http://{{ gatewaySetup.upstream_gateway_ip || '192.168.1.1' }}</a>
            </li>
            <li>Vào mục <strong>Network / LAN → DHCP Server → DNS Server</strong>.</li>
            <li>
              Đặt <strong>Primary DNS (DNS 1)</strong> =
              <code class="mono font-bold text-emerald-300 bg-emerald-500/15 px-1.5 py-0.5 rounded">{{ gatewaySetup.r3p_client_ip || '192.168.1.6' }}</code>
            </li>
            <li>
              Đặt <strong>Secondary DNS (DNS 2 dự phòng)</strong> =
              <code class="mono font-bold text-sky-300 bg-sky-500/15 px-1.5 py-0.5 rounded">1.1.1.1</code>
            </li>
            <li>Lưu lại (Save) và kết nối lại Wi-Fi trên thiết bị để nhận DNS mới.</li>
          </ol>
        </div>

        <!-- Box 3: Per-device DNS setup (Windows / Phone / Smart TV) -->
        <div class="p-3 rounded-lg bg-black/40 border border-white/10 flex flex-col justify-between space-y-2">
          <div>
            <div class="font-bold text-purple-300 uppercase text-[11px]">3. Hoặc Trỏ DNS Trực Tiếp Trên Máy Tính / Điện Thoại</div>
            <p class="text-gray-400 text-[11px] mt-1">
              Nếu không muốn sửa cấu hình trên Modem chính, bạn có thể nhập DNS thủ công trên từng thiết bị:
            </p>
            <div class="mt-1.5 space-y-1 text-[11px] text-gray-300">
              <div>• <strong>Khi bắt Wi-Fi nhà ({{ gatewaySetup.upstream_gateway_ip || '192.168.1.1' }}):</strong> Đặt DNS = <code class="mono text-emerald-300 font-bold">{{ gatewaySetup.r3p_client_ip || '192.168.1.6' }}</code></div>
              <div>• <strong>Khi cắm dây LAN vào R3P:</strong> Đặt DNS = <code class="mono text-sky-300 font-bold">{{ gatewaySetup.r3p_lan_ip || '192.168.2.1' }}</code></div>
            </div>
          </div>

          <div class="p-2 rounded bg-black/60 border border-white/10">
            <div class="flex items-center justify-between mb-1">
              <span class="text-[10px] text-gray-400 uppercase font-semibold">Lệnh PowerShell đặt DNS nhanh trên Windows:</span>
              <button
                type="button"
                class="text-[10px] text-sky-400 hover:underline cursor-pointer"
                @click="copyToClipboard(`Set-DnsClientServerAddress -InterfaceAlias 'Wi-Fi' -ServerAddresses ('${gatewaySetup.r3p_client_ip || '192.168.1.6'}','1.1.1.1')`, 'ps_cmd')"
              >
                {{ copiedText === 'ps_cmd' ? '✓ Đã Copy Lệnh' : '📋 Copy Lệnh' }}
              </button>
            </div>
            <code class="mono text-[10px] text-emerald-300 break-all block">
              Set-DnsClientServerAddress -InterfaceAlias "Wi-Fi" -ServerAddresses ("{{ gatewaySetup.r3p_client_ip || '192.168.1.6' }}","1.1.1.1")
            </code>
          </div>
        </div>
      </div>
    </div>

    <!-- Real-Time Ad Stream Inspector & Pattern Recognizer -->
    <div class="pro-card border border-purple-500/35 bg-gradient-to-r from-purple-500/[0.05] via-black/30 to-sky-500/[0.05] space-y-3">
      <div class="pro-card-header flex-wrap gap-2">
        <div class="flex items-center gap-2">
          <span class="text-base">🔍</span>
          <span class="pro-card-title text-purple-300">
            Trình Bóc Tách & Nhận Dạng Luồng Quảng Cáo Thời Gian Thực (Ad Stream Inspector)
          </span>
          <span class="badge badge-purple">AI PATTERN RECOGNIZER</span>
        </div>
        <div class="flex items-center gap-2">
          <button type="button" class="btn-secondary text-[11px] py-1 min-w-[126px]" :disabled="loadingStreams" @click="loadAdStreams">
            {{ loadingStreams ? 'Đang phân tích…' : 'Làm mới luồng ↻' }}
          </button>
          <button type="button" class="btn-danger text-[11px] py-1" @click="clearStreamHistory">
            Xóa lịch sử
          </button>
        </div>
      </div>

      <!-- Live Stream Counters -->
      <div class="grid grid-cols-2 md:grid-cols-4 gap-2.5">
        <div class="p-2.5 rounded bg-black/40 border border-white/5">
          <div class="text-[10px] text-gray-400 uppercase font-semibold">Đã Nhận Diện & Tự Động Chặn</div>
          <div class="mono text-lg font-bold text-rose-400">{{ streamStats.blocked }}</div>
          <div class="text-[9.5px] text-gray-500">Luồng quảng cáo & tracker bị triệt tiêu</div>
        </div>
        <div class="p-2.5 rounded bg-black/40 border border-white/5">
          <div class="text-[10px] text-gray-400 uppercase font-semibold">Luồng Video Ads (YouTube/TikTok)</div>
          <div class="mono text-lg font-bold text-amber-400">{{ streamStats.video_ads }}</div>
          <div class="text-[9.5px] text-gray-500">Bóc tách video ad fragments</div>
        </div>
        <div class="p-2.5 rounded bg-black/40 border border-white/5">
          <div class="text-[10px] text-gray-400 uppercase font-semibold">Trình Theo Dõi & Pixel (Trackers)</div>
          <div class="mono text-lg font-bold text-purple-400">{{ streamStats.trackers }}</div>
          <div class="text-[9.5px] text-gray-500">Facebook Pixel, Google, ByteDance</div>
        </div>
        <div class="p-2.5 rounded bg-black/40 border border-white/5">
          <div class="text-[10px] text-gray-400 uppercase font-semibold">Tổng Số Luồng Đã Phân Tích</div>
          <div class="mono text-lg font-bold text-sky-400">{{ streamStats.analyzed }}</div>
          <div class="text-[9.5px] text-gray-500">Giám sát liên tục qua cổng DNS 53</div>
        </div>
      </div>

      <!-- Inspected Streams Table -->
      <div class="overflow-x-auto max-h-64 border border-white/10 rounded-lg">
        <table class="w-full text-left text-xs border-collapse">
          <thead>
            <tr class="bg-black/50 border-b border-white/10 text-gray-400 text-[10.5px] uppercase font-semibold">
              <th class="py-2 px-3 w-20">Thời gian</th>
              <th class="py-2 px-3">Tên miền luồng (Domain)</th>
              <th class="py-2 px-3 w-40">Phân loại nhận dạng</th>
              <th class="py-2 px-3 hidden md:table-cell">Mẫu nhận diện (Signature)</th>
              <th class="py-2 px-3 w-24 text-center">Trạng thái</th>
              <th class="py-2 px-3 w-28 text-right">Thao tác</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-white/[0.04] text-[11.5px]">
            <tr v-if="!streams.length">
              <td colspan="6" class="text-center py-6 text-gray-500">
                Chưa có luồng dữ liệu nào được ghi nhận.
              </td>
            </tr>
            <tr
              v-for="st in streams"
              :key="st.id"
              class="hover:bg-white/[0.02] transition"
            >
              <td class="py-1.5 px-3 mono text-gray-500 text-[10.5px]">{{ st.timestamp }}</td>
              <td class="py-1.5 px-3 font-semibold text-gray-200 mono">
                {{ st.domain }}
              </td>
              <td class="py-1.5 px-3">
                <span
                  class="px-1.5 py-0.5 rounded text-[10px] font-bold"
                  :class="st.category.includes('Video')
                    ? 'bg-amber-500/20 text-amber-300'
                    : st.category.includes('Tracker') || st.category.includes('Pixel')
                      ? 'bg-purple-500/20 text-purple-300'
                      : st.category.includes('Ad')
                        ? 'bg-rose-500/20 text-rose-300'
                        : 'bg-gray-500/20 text-gray-400'"
                >
                  {{ st.category }}
                </span>
              </td>
              <td class="py-1.5 px-3 text-gray-400 text-[10.5px] hidden md:table-cell truncate max-w-xs">
                {{ st.pattern }}
              </td>
              <td class="py-1.5 px-3 text-center">
                <span
                  class="px-2 py-0.5 rounded text-[10px] font-bold"
                  :class="st.action === 'BLOCKED' ? 'bg-rose-500/20 text-rose-300 border border-rose-500/30' : 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30'"
                >
                  {{ st.action }}
                </span>
              </td>
              <td class="py-1.5 px-3 text-right">
                <button
                  v-if="st.action !== 'BLOCKED'"
                  type="button"
                  class="btn-danger text-[10px] py-0.5 px-2"
                  @click="blockDomainFromStream(st.domain)"
                >
                  🚫 Chặn Luồng
                </button>
                <span v-else class="text-[10px] text-gray-500 mono">Đã chặn</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Global AdBlock Settings & Domain Tester Grid -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-3 items-start">
      <!-- Left 2 Cols: Full AdBlock Settings -->
      <form class="pro-card lg:col-span-2 space-y-3" @submit.prevent="saveGlobalConfig">
        <div class="pro-card-header">
          <span class="pro-card-title">⚙ Cài Đặt Chuyên Sâu Hệ Thống AdBlock DNS</span>
          <button type="submit" class="btn-primary text-[11px] py-1 px-3 min-w-[164px]" :disabled="saving">
            {{ saving ? 'Đang lưu & áp dụng…' : '💾 Lưu Cài Đặt AdBlock' }}
          </button>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-2.5 text-xs">
          <label class="flex items-start gap-2.5 p-2.5 rounded bg-black/30 border border-white/5 cursor-pointer hover:border-sky-500/30">
            <input v-model="config.enabled" type="checkbox" class="mt-0.5 rounded" />
            <div>
              <div class="font-semibold text-gray-100">Kích hoạt lọc quảng cáo DNS (Master Switch)</div>
              <div class="text-[11px] text-gray-400">Chặn quảng cáo, banner, pop-up và tên miền theo dõi cho mọi thiết bị kết nối LAN & Wi-Fi.</div>
            </div>
          </label>

          <label class="flex items-start gap-2.5 p-2.5 rounded bg-black/30 border border-white/5 cursor-pointer hover:border-sky-500/30">
            <input v-model="config.force_dns" type="checkbox" class="mt-0.5 rounded" />
            <div>
              <div class="font-semibold text-sky-300">Ép buộc DNS toàn mạng LAN (Force DNS Port 53)</div>
              <div class="text-[11px] text-gray-400">Chuyển hướng mọi truy vấn cổng 53 về Router, ngăn thiết bị tự đặt DNS 8.8.8.8 vượt qua AdBlock.</div>
            </div>
          </label>

          <label class="flex items-start gap-2.5 p-2.5 rounded bg-black/30 border border-white/5 cursor-pointer hover:border-sky-500/30">
            <input v-model="config.safe_search" type="checkbox" class="mt-0.5 rounded" />
            <div>
              <div class="font-semibold text-purple-300">Bật Tìm Kiếm An Toàn (SafeSearch)</div>
              <div class="text-[11px] text-gray-400">Ép buộc chế độ lọc nội dung người lớn trên Google, YouTube và Bing ở tầng DNS.</div>
            </div>
          </label>

          <label class="flex items-start gap-2.5 p-2.5 rounded bg-black/30 border border-white/5 cursor-pointer hover:border-sky-500/30">
            <input v-model="config.query_logging" type="checkbox" class="mt-0.5 rounded" />
            <div>
              <div class="font-semibold text-emerald-300">Ghi nhật ký truy vấn DNS (DNS Query Log)</div>
              <div class="text-[11px] text-gray-400">Hiển thị chi tiết các tên miền thiết bị trong mạng đang truy cập và bị chặn ở bảng Log bên dưới.</div>
            </div>
          </label>

          <label class="flex items-start gap-2.5 p-2.5 rounded bg-black/30 border border-white/5 cursor-pointer hover:border-amber-500/30 md:col-span-2">
            <input v-model="config.expose_upstream_dns" type="checkbox" class="mt-0.5 rounded" />
            <div>
              <div class="font-semibold text-amber-300">Chia sẻ AdBlock DNS cho mạng LAN upstream (Chỉ RFC1918)</div>
              <div class="text-[11px] text-gray-400">Cho phép router chính / subnet nội bộ (192.168.x.x, 10.x.x.x, 172.16.x.x) gửi DNS qua cổng 53 mà không mở ra Internet công cộng.</div>
            </div>
          </label>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-2.5 pt-1 text-xs">
          <div>
            <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Upstream DNS Servers (Cách nhau dấu phẩy)</label>
            <input
              v-model="upstreamDnsInput"
              type="text"
              class="w-full pro-input mono"
              placeholder="1.1.1.1, 8.8.8.8, 9.9.9.9"
            />
          </div>

          <div>
            <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Lịch Tự Động Cập Nhật (Cron)</label>
            <select v-model="config.update_schedule" class="w-full pro-input mono">
              <option value="0 3 * * *">Mỗi ngày lúc 03:00 sáng (0 3 * * *)</option>
              <option value="0 */12 * * *">Mỗi 12 tiếng (0 */12 * * *)</option>
              <option value="0 4 * * 0">Mỗi Chủ Nhật lúc 04:00 (0 4 * * 0)</option>
            </select>
          </div>

          <div>
            <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Chế Độ Phản Hồi Khi Chặn</label>
            <select v-model="config.block_mode" class="w-full pro-input mono">
              <option value="zero_ip">0.0.0.0 (Null IP — Tốc độ phản hồi tức thì)</option>
              <option value="nxdomain">NXDOMAIN (Tên miền không tồn tại)</option>
            </select>
          </div>
        </div>
      </form>

      <!-- Right Col: Live Domain Block Tester -->
      <div class="pro-card flex flex-col justify-between space-y-2.5">
        <div>
          <div class="pro-card-header">
            <span class="pro-card-title">🔍 Kiểm Tra Chặn Tên Miền Thực Tế</span>
            <span class="badge badge-blue">NSLOOKUP TEST</span>
          </div>
          <p class="text-[11px] text-gray-400 mb-2">
            Nhập bất kỳ tên miền nào (VD: <code class="mono text-sky-300">doubleclick.net</code>, <code class="mono text-sky-300">ads.tiktok.com</code>) để kiểm tra xem DNS Router đã chặn hay chưa.
          </p>
          <form class="flex gap-1.5 mb-2.5" @submit.prevent="runDomainTest">
            <input
              v-model="testDomainInput"
              type="text"
              class="pro-input flex-1 mono text-xs min-w-0"
              placeholder="Nhập tên miền cần test…"
            />
            <button type="submit" class="btn-primary text-xs px-3 min-w-[96px]" :disabled="testingDomain">
              {{ testingDomain ? 'Đang test…' : 'Kiểm Tra' }}
            </button>
          </form>

          <div
            class="p-2.5 rounded bg-black/50 border text-xs h-36 flex flex-col justify-between"
            :class="testResult ? (testResult.blocked ? 'border-emerald-500/40' : 'border-amber-500/40') : 'border-white/10'"
          >
            <template v-if="testResult">
              <div class="flex items-center justify-between gap-2 shrink-0">
                <span class="mono font-bold text-gray-100 truncate">{{ testResult.domain }}</span>
                <span class="badge" :class="testResult.blocked ? 'badge-green' : 'badge-orange'">
                  {{ testResult.blocked ? '🛡 ĐÃ CHẶN (0.0.0.0)' : '✓ CHO PHÉP TRUY CẬP' }}
                </span>
              </div>
              <pre class="mono text-[10px] text-gray-300 flex-1 overflow-y-auto whitespace-pre-wrap bg-black/40 p-1.5 rounded mt-1.5">{{ testResult.dns_output }}</pre>
            </template>
            <div v-else class="h-full flex items-center justify-center text-gray-500 text-center text-[11.5px] px-3">
              Bấm "Kiểm Tra" hoặc chọn một tên miền mẫu bên dưới để xem kết quả phân giải nslookup thực tế.
            </div>
          </div>
        </div>

        <div class="flex flex-wrap gap-1 pt-1 border-t border-white/5 text-[10.5px]">
          <span class="text-gray-500">Test nhanh:</span>
          <button
            v-for="d in ['doubleclick.net', 'ads.tiktok.com', 'googleadservices.com', 'openwrt.org']"
            :key="d"
            type="button"
            class="px-1.5 py-0.5 rounded bg-white/5 hover:bg-sky-500/20 text-sky-300 mono"
            @click="testDomainInput = d; runDomainTest()"
          >
            {{ d }}
          </button>
        </div>
      </div>
    </div>

    <!-- Blocklist Subscriptions Table -->
    <div class="pro-card">
      <div class="pro-card-header">
        <div class="flex items-center gap-2">
          <span class="pro-card-title">📋 Nguồn Danh Sách Chặn Quảng Cáo & Mã Độc (Blocklist Subscriptions)</span>
          <span class="badge badge-blue">{{ config.sources.length }} Nguồn</span>
        </div>
        <button type="button" class="btn-primary text-[11px] py-1 min-w-[205px]" @click="showAddSource = !showAddSource">
          {{ showAddSource ? '✕ Đóng' : '+ Thêm URL Danh Sách Chặn Mới' }}
        </button>
      </div>

      <!-- Add New Blocklist Source Form -->
      <form
        v-if="showAddSource"
        class="mb-3 p-3 rounded bg-[#0d1526] border border-sky-500/40 grid grid-cols-1 md:grid-cols-4 gap-2 text-xs"
        @submit.prevent="addSourceList"
      >
        <input
          v-model="newSource.name"
          type="text"
          required
          class="pro-input"
          placeholder="Tên bộ lọc (VD: OISD Big / VnAdblock)"
        />
        <input
          v-model="newSource.url"
          type="url"
          required
          class="pro-input mono md:col-span-2"
          placeholder="https://raw.githubusercontent.com/.../hosts.txt"
        />
        <div class="flex gap-1.5">
          <select v-model="newSource.category" class="pro-input flex-1">
            <option value="Ads & Trackers">Ads & Trackers</option>
            <option value="Malware">Malware & Phishing</option>
            <option value="Telemetry">Telemetry</option>
            <option value="Social / Adult">Social / Adult</option>
            <option value="Custom">Custom</option>
          </select>
          <button type="submit" class="btn-primary px-3">Thêm</button>
        </div>
      </form>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Trạng Thái</th>
              <th>Tên Bộ Lọc (Blocklist Name)</th>
              <th>Phân Loại</th>
              <th>URL Nguồn</th>
              <th>Số Tên Miền</th>
              <th>Cập Nhật Cuối</th>
              <th>Thao Tác</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in config.sources" :key="s.id">
              <td>
                <span class="badge" :class="s.enabled ? 'badge-green' : 'badge-gray'">
                  {{ s.enabled ? 'ENABLED' : 'DISABLED' }}
                </span>
              </td>
              <td class="font-semibold text-gray-100">{{ s.name }}</td>
              <td><span class="badge badge-purple text-[10px]">{{ s.category }}</span></td>
              <td class="mono text-[11px] text-sky-400 max-w-xs truncate" :title="s.url">{{ s.url }}</td>
              <td class="mono text-emerald-400 font-semibold">
                {{ s.domain_count ? s.domain_count.toLocaleString() : s.status }}
              </td>
              <td class="mono text-gray-400 text-[11px]">{{ s.last_updated }}</td>
              <td>
                <div class="flex items-center gap-1.5">
                  <button
                    type="button"
                    class="btn-secondary text-[10.5px] py-0.5 px-2 min-w-[44px]"
                    @click="toggleSource(s)"
                  >
                    {{ s.enabled ? 'Tắt' : 'Bật' }}
                  </button>
                  <button
                    type="button"
                    class="btn-danger text-[10.5px] py-0.5 px-2"
                    @click="deleteSource(s.id)"
                  >
                    Xóa
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Custom Blacklist & Whitelist Panels -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
      <!-- Custom Blacklist -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title text-red-300">🚫 Danh Sách Chặn Tùy Chỉnh (Custom Blacklist · {{ config.blacklist.length }})</span>
        </div>
        <p class="text-[11px] text-gray-400 mb-2">
          Thêm tên miền bạn muốn chặn hoàn toàn trên mạng LAN/Wi-Fi (hỗ trợ nhập nhiều tên miền cách nhau bởi dấu cách hoặc dấu phẩy).
        </p>
        <form class="flex gap-1.5 mb-2.5" @submit.prevent="addCustomDomain('blacklist')">
          <input
            v-model="newBlacklistDomain"
            type="text"
            class="pro-input flex-1 mono text-xs min-w-0"
            placeholder="VD: ads.example.com, tracker.net"
          />
          <button type="submit" class="btn-danger text-xs px-3">+ Chặn Tên Miền</button>
        </form>
        <div class="flex flex-wrap gap-1.5 max-h-40 overflow-y-auto p-1">
          <div v-if="!config.blacklist.length" class="text-xs text-gray-500 py-2">Chưa có tên miền chặn tùy chỉnh.</div>
          <span
            v-for="d in config.blacklist"
            :key="d"
            class="inline-flex items-center gap-1.5 px-2 py-1 rounded bg-red-500/15 border border-red-500/30 text-red-200 mono text-[11px]"
          >
            {{ d }}
            <button type="button" class="text-red-400 hover:text-white font-bold cursor-pointer" title="Xóa" @click="removeCustomDomain('blacklist', d)">✕</button>
          </span>
        </div>
      </div>

      <!-- Custom Whitelist -->
      <div class="pro-card">
        <div class="pro-card-header">
          <span class="pro-card-title text-emerald-300">✅ Danh Sách Trắng Bỏ Chặn (Custom Whitelist · {{ config.whitelist.length }})</span>
        </div>
        <p class="text-[11px] text-gray-400 mb-2">
          Các tên miền trong danh sách này sẽ luôn được phép truy cập, không bao giờ bị chặn bởi bất kỳ bộ lọc Blocklist nào.
        </p>
        <form class="flex gap-1.5 mb-2.5" @submit.prevent="addCustomDomain('whitelist')">
          <input
            v-model="newWhitelistDomain"
            type="text"
            class="pro-input flex-1 mono text-xs min-w-0"
            placeholder="VD: analytics.google.com, github.com"
          />
          <button type="submit" class="btn-primary text-xs px-3">+ Cho Phép</button>
        </form>
        <div class="flex flex-wrap gap-1.5 max-h-40 overflow-y-auto p-1">
          <div v-if="!config.whitelist.length" class="text-xs text-gray-500 py-2">Chưa có tên miền ngoại lệ.</div>
          <span
            v-for="d in config.whitelist"
            :key="d"
            class="inline-flex items-center gap-1.5 px-2 py-1 rounded bg-emerald-500/15 border border-emerald-500/30 text-emerald-200 mono text-[11px]"
          >
            {{ d }}
            <button type="button" class="text-emerald-400 hover:text-white font-bold cursor-pointer" title="Xóa" @click="removeCustomDomain('whitelist', d)">✕</button>
          </span>
        </div>
      </div>
    </div>

    <!-- Live Dnsmasq Query & AdBlock Log -->
    <div class="pro-card">
      <div class="pro-card-header flex-wrap gap-2">
        <div class="flex items-center gap-2">
          <span class="pro-card-title">📜 Nhật Ký Phân Giải & Chặn DNS (dnsmasq log · {{ filteredLogs.length }} dòng)</span>
          <span class="text-[11px] text-gray-400 hidden sm:inline">(Mới nhất ở trên cùng)</span>
        </div>
        <div class="flex items-center gap-2">
          <input
            v-model="logFilter"
            type="search"
            class="pro-input w-52 mono text-[11px]"
            placeholder="Lọc theo tên miền hoặc IP…"
          />
          <button type="button" class="btn-secondary text-[11px] py-1" @click="loadAdblock">↻ Tải lại</button>
          <button
            type="button"
            class="px-2.5 py-1 text-[11px] rounded bg-red-500/20 hover:bg-red-500/30 text-red-300 border border-red-500/40 transition font-semibold cursor-pointer"
            :disabled="clearingDnsLogs"
            @click="clearDnsLogs"
          >
            {{ clearingDnsLogs ? 'Đang xóa...' : '🗑 Xóa Log' }}
          </button>
        </div>
      </div>

      <div class="bg-black/60 rounded border border-white/5 p-2 font-mono text-[10.5px] text-gray-300 h-52 overflow-y-auto space-y-0.5">
        <div v-if="!filteredLogs.length" class="text-gray-500 py-4 text-center">
          Chưa có dòng log khớp. Bật tùy chọn "Ghi nhật ký truy vấn DNS (DNS Query Log)" ở phần Cài Đặt phía trên để xem trực tiếp mọi truy vấn tên miền.
        </div>
        <div
          v-for="(line, idx) in filteredLogs"
          :key="idx"
          class="px-1.5 py-0.5 rounded leading-tight"
          :class="line.includes('0.0.0.0') ? 'bg-red-500/15 text-red-300 font-semibold' : 'hover:bg-white/[0.04]'"
        >
          {{ line }}
        </div>
      </div>
    </div>
  </div>
</template>
