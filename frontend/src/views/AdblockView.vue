<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'

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

const gatewaySetup = ref<GatewaySetupInfo>({
  is_client_mode: true,
  r3p_client_ip: '192.168.1.6',
  r3p_client_iface: 'phy0-sta0',
  upstream_gateway_ip: '192.168.1.1',
  r3p_lan_ip: '192.168.2.1',
  wan_dns_listening: true,
})
const copiedText = ref('')

async function copyToClipboard(txt: string, label: string) {
  try {
    await navigator.clipboard.writeText(txt)
    copiedText.value = label
    setTimeout(() => { copiedText.value = '' }, 2000)
  } catch {
    // ignore
  }
}

const loading = ref(false)
const saving = ref(false)
const updatingLists = ref(false)
const notice = ref<string | null>(null)
const errorMsg = ref<string | null>(null)

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
    notice.value = res?.message || `Đã cập nhật ${res?.total_blocked || 0} tên miền chặn!`
    await loadAdblock()
  } catch (err: any) {
    errorMsg.value = 'Cập nhật danh sách lỗi: ' + (err?.message || err)
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
    notice.value = res?.message || 'Đã thêm nguồn danh sách chặn!'
  } catch (err: any) {
    errorMsg.value = 'Thêm danh sách thất bại: ' + (err?.message || err)
  }
}

async function toggleSource(src: AdblockSource) {
  try {
    const res = await apiRequest<{ config: AdblockConfig }>('/adblock/lists', {
      method: 'PATCH',
      body: JSON.stringify({ id: src.id, enabled: !src.enabled }),
    })
    if (res?.config) config.value = res.config
  } catch (err: any) {
    errorMsg.value = 'Lỗi: ' + (err?.message || err)
  }
}

async function deleteSource(id: string) {
  try {
    const res = await apiRequest<{ config: AdblockConfig }>(`/adblock/lists?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
    if (res?.config) config.value = res.config
  } catch (err: any) {
    errorMsg.value = 'Lỗi xóa danh sách: ' + (err?.message || err)
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
    notice.value = res?.message || 'Đã cập nhật danh sách tên miền!'
    await loadAdblock()
  } catch (err: any) {
    errorMsg.value = 'Lỗi thêm tên miền: ' + (err?.message || err)
  }
}

async function removeCustomDomain(listType: 'blacklist' | 'whitelist', domain: string) {
  try {
    const res = await apiRequest<{ config: AdblockConfig }>(
      `/adblock/domains?list_type=${listType}&domain=${encodeURIComponent(domain)}`,
      { method: 'DELETE' },
    )
    if (res?.config) config.value = res.config
    await loadAdblock()
  } catch (err: any) {
    errorMsg.value = 'Lỗi xóa tên miền: ' + (err?.message || err)
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
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Top Banner & Master Controls -->
    <div class="pro-card border border-sky-500/30">
      <div class="pro-card-header">
        <div class="flex flex-wrap items-center gap-2.5">
          <span
            class="w-3 h-3 rounded-full"
            :class="config.enabled ? 'bg-emerald-400 animate-pulse' : 'bg-red-400'"
          ></span>
          <span class="pro-card-title text-sm">R3P AdBlock DNS Shield (Chặn Quảng Cáo, Theo Dõi & Mã Độc Toàn Mạng)</span>
          <span class="badge" :class="config.enabled ? 'badge-green' : 'badge-orange'">
            {{ config.enabled ? 'ADBLOCK ACTIVE' : 'ADBLOCK DISABLED' }}
          </span>
          <span v-if="config.force_dns" class="badge badge-blue">FORCE LAN DNS: ON (PORT 53)</span>
          <span v-if="config.safe_search" class="badge badge-purple">SAFESEARCH: ON</span>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <button
            class="text-[11.5px] py-1 px-3 rounded font-semibold transition"
            :class="config.enabled ? 'btn-danger' : 'btn-primary'"
            :disabled="saving"
            @click="toggleAdblockMaster"
          >
            {{ config.enabled ? '⏸ Tắt AdBlock' : '▶ Bật AdBlock Ngay' }}
          </button>
          <button
            class="btn-primary text-[11.5px] py-1 px-3"
            :disabled="updatingLists"
            @click="updateBlocklistsNow"
          >
            {{ updatingLists ? '⏳ Đang Tải & Cập Nhật…' : '🔄 Cập Nhật Danh Sách Chặn Ngay' }}
          </button>
          <button class="btn-secondary text-[11.5px] py-1" :disabled="loading" @click="loadAdblock">
            ↻ Làm mới
          </button>
        </div>
      </div>

      <!-- Summary KPI Strip -->
      <div class="grid grid-cols-2 md:grid-cols-5 gap-2.5 mt-1">
        <div class="p-2.5 rounded bg-black/30 border border-white/5">
          <div class="text-[10px] text-gray-400 uppercase">Tổng Tên Miền Đang Chặn</div>
          <div class="mono text-lg font-bold text-emerald-400">{{ (activeRules || config.total_blocked).toLocaleString() }}</div>
          <div class="text-[10px] text-gray-500">Nạp trong /tmp/r3p-adblock.hosts</div>
        </div>
        <div class="p-2.5 rounded bg-black/30 border border-white/5">
          <div class="text-[10px] text-gray-400 uppercase">Nguồn Blocklist Đang Bật</div>
          <div class="mono text-lg font-bold text-sky-400">
            {{ config.sources.filter(s => s.enabled).length }} / {{ config.sources.length }}
          </div>
          <div class="text-[10px] text-gray-500">Hỗ trợ thêm URL tùy chỉnh</div>
        </div>
        <div class="p-2.5 rounded bg-black/30 border border-white/5">
          <div class="text-[10px] text-gray-400 uppercase">Danh Sách Đen / Trắng</div>
          <div class="mono text-lg font-bold text-purple-400">
            {{ config.blacklist.length }} Chặn · {{ config.whitelist.length }} Cho phép
          </div>
          <div class="text-[10px] text-gray-500">Quy tắc tùy chỉnh của Admin</div>
        </div>
        <div class="p-2.5 rounded bg-black/30 border border-white/5">
          <div class="text-[10px] text-gray-400 uppercase">Upstream DNS</div>
          <div class="mono text-sm font-bold text-amber-400 truncate">{{ (config.upstream_dns || []).join(' · ') }}</div>
          <div class="text-[10px] text-gray-500">Máy chủ phân giải DNS gốc</div>
        </div>
        <div class="p-2.5 rounded bg-black/30 border border-white/5">
          <div class="text-[10px] text-gray-400 uppercase">Cập Nhật Lần Cuối</div>
          <div class="mono text-xs font-bold text-gray-200">{{ config.last_rebuild || '—' }}</div>
          <div class="text-[10px] text-gray-500">Auto Cron: {{ config.auto_update ? config.update_schedule : 'Tắt' }}</div>
        </div>
      </div>
    </div>

    <!-- Notices -->
    <div v-if="notice" class="px-3 py-2 rounded bg-emerald-500/15 border border-emerald-500/30 text-emerald-300 text-xs font-medium">
      ✓ {{ notice }}
    </div>
    <div v-if="errorMsg" class="px-3 py-2 rounded bg-red-500/15 border border-red-500/30 text-red-300 text-xs font-medium">
      ⚠ {{ errorMsg }}
    </div>

    <!-- Gateway DNS Setup Guide (When R3P operates as a Client on Upstream Router) -->
    <div class="pro-card border border-emerald-500/35 bg-gradient-to-r from-emerald-500/[0.06] to-sky-500/[0.05]">
      <div class="pro-card-header flex-wrap gap-2">
        <div class="flex items-center gap-2">
          <span class="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse"></span>
          <span class="pro-card-title text-emerald-300">
            💡 Gợi Ý Setup DNS Cho Gateway Chính (Do R3P Đang Chạy Chế Độ Client · {{ gatewaySetup.r3p_client_iface || 'WWAN/WAN' }})
          </span>
          <span class="badge badge-green">PORT 53 WAN/WWAN: ĐÃ MỞ SẴN</span>
        </div>
        <div class="flex items-center gap-2">
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
            <div>
              <div class="text-[10px] text-emerald-300 uppercase font-semibold">IP R3P Trên Mạng Gateway Chính ({{ gatewaySetup.r3p_client_iface || 'WWAN' }})</div>
              <div class="mono text-sm font-bold text-white">{{ gatewaySetup.r3p_client_ip || '192.168.1.6' }}</div>
            </div>
            <button
              type="button"
              class="btn-primary text-[10.5px] py-1 px-2.5"
              @click="copyToClipboard(gatewaySetup.r3p_client_ip || '192.168.1.6', 'client_ip')"
            >
              {{ copiedText === 'client_ip' ? '✓ Đã Copy!' : '📋 Copy IP' }}
            </button>
          </div>
          <div class="flex items-center justify-between p-2 rounded bg-sky-500/10 border border-sky-500/25">
            <div>
              <div class="text-[10px] text-sky-300 uppercase font-semibold">IP R3P Trên Mạng LAN Nội Bộ (br-lan)</div>
              <div class="mono text-sm font-bold text-white">{{ gatewaySetup.r3p_lan_ip || '192.168.2.1' }}</div>
            </div>
            <button
              type="button"
              class="btn-secondary text-[10.5px] py-1 px-2.5"
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

    <!-- Global AdBlock Settings & Domain Tester Grid -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-3">
      <!-- Left 2 Cols: Full AdBlock Settings -->
      <form class="pro-card lg:col-span-2 space-y-3" @submit.prevent="saveGlobalConfig">
        <div class="pro-card-header">
          <span class="pro-card-title">⚙ Cài Đặt Chuyên Sâu Hệ Thống AdBlock DNS</span>
          <button type="submit" class="btn-primary text-[11px] py-1 px-3" :disabled="saving">
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
              class="pro-input flex-1 mono text-xs"
              placeholder="Nhập tên miền cần test…"
            />
            <button type="submit" class="btn-primary text-xs px-3" :disabled="testingDomain">
              {{ testingDomain ? 'Đang test…' : 'Kiểm Tra' }}
            </button>
          </form>

          <div v-if="testResult" class="p-2.5 rounded bg-black/50 border text-xs space-y-1.5" :class="testResult.blocked ? 'border-emerald-500/40' : 'border-amber-500/40'">
            <div class="flex items-center justify-between">
              <span class="mono font-bold text-gray-100">{{ testResult.domain }}</span>
              <span class="badge" :class="testResult.blocked ? 'badge-green' : 'badge-orange'">
                {{ testResult.blocked ? '🛡 ĐÃ CHẶN (0.0.0.0)' : '✓ CHO PHÉP TRUY CẬP' }}
              </span>
            </div>
            <pre class="mono text-[10px] text-gray-300 max-h-28 overflow-y-auto whitespace-pre-wrap bg-black/40 p-1.5 rounded">{{ testResult.dns_output }}</pre>
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
        <button class="btn-primary text-[11px] py-1" @click="showAddSource = !showAddSource">
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
                    class="btn-secondary text-[10.5px] py-0.5 px-2"
                    @click="toggleSource(s)"
                  >
                    {{ s.enabled ? 'Tắt' : 'Bật' }}
                  </button>
                  <button
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
            class="pro-input flex-1 mono text-xs"
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
            <button class="text-red-400 hover:text-white font-bold" title="Xóa" @click="removeCustomDomain('blacklist', d)">✕</button>
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
            class="pro-input flex-1 mono text-xs"
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
            <button class="text-emerald-400 hover:text-white font-bold" title="Xóa" @click="removeCustomDomain('whitelist', d)">✕</button>
          </span>
        </div>
      </div>
    </div>

    <!-- Live Dnsmasq Query & AdBlock Log -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">📜 Nhật Ký Phân Giải & Chặn DNS (dnsmasq log · {{ filteredLogs.length }} dòng)</span>
        <div class="flex items-center gap-2">
          <input
            v-model="logFilter"
            type="search"
            class="pro-input w-56 mono text-[11px]"
            placeholder="Lọc theo tên miền hoặc IP…"
          />
          <button class="btn-secondary text-[11px] py-1" @click="loadAdblock">↻ Tải lại Log</button>
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
