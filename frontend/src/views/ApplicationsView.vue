<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'
import { useUiPreferences } from '@/composables/useUiPreferences'

const { t } = useUiPreferences()

interface ServiceItem {
  name: string
  running: boolean
  enabled: boolean
  pid: number
  memory_kb: number
  description: string
}

interface AppRepoItem {
  id: string
  name: string
  url: string
  type: string
  branch: string
  description: string
  enabled: boolean
  pkg_count: number
  last_synced: string
}

interface OpenWrtPkgApp {
  name: string
  version: string
  category: string
  description: string
  source: string
  download_url?: string
  installed: boolean
}

const services = ref<ServiceItem[]>([])
const packages = ref<string[]>([])
const repos = ref<AppRepoItem[]>([])
const catalog = ref<OpenWrtPkgApp[]>([])
const categories = ref<string[]>([])

const pkgFilter = ref('')
const searchQuery = ref('')
const selectedCategory = ref('All')

const newRepoName = ref('')
const newRepoUrl = ref('')
const newRepoBranch = ref('main')
const directInstallInput = ref('')
const allowUntrusted = ref(false)

const loading = ref(false)
const searching = ref(false)
const syncingFeeds = ref(false)
const addingRepo = ref(false)
const pkgActionLoading = ref<string | null>(null)
const actionLoading = ref<string | null>(null)
const notice = ref<string | null>(null)
const errorMsg = ref<string | null>(null)
const terminalOutput = ref<string>('')

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

const filteredPackages = computed(() => {
  const q = pkgFilter.value.trim().toLowerCase()
  if (!q) return packages.value
  return packages.value.filter((p) => p.toLowerCase().includes(q))
})

async function loadApps() {
  loading.value = true
  try {
    const [svcRes, pkgRes, repoRes] = await Promise.all([
      apiRequest<{ services: ServiceItem[] }>('/services/all'),
      apiRequest<{ packages: string[] }>('/packages'),
      apiRequest<{ repos: AppRepoItem[] }>('/packages/repos'),
    ])
    if (svcRes?.services) services.value = svcRes.services
    if (pkgRes?.packages) packages.value = pkgRes.packages
    if (repoRes?.repos) repos.value = repoRes.repos
    await searchOpenWrtPackages()
  } catch (err: any) {
    errorMsg.value = 'Lỗi tải dữ liệu ứng dụng: ' + (err?.message || err)
  } finally {
    loading.value = false
  }
}

async function searchOpenWrtPackages() {
  searching.value = true
  try {
    const params = new URLSearchParams()
    if (searchQuery.value.trim()) params.set('q', searchQuery.value.trim())
    if (selectedCategory.value && selectedCategory.value !== 'All') {
      params.set('category', selectedCategory.value)
    }
    const res = await apiRequest<{
      packages?: OpenWrtPkgApp[]
      results?: OpenWrtPkgApp[]
      categories?: string[]
      total?: number
    }>(`/packages/search?${params.toString()}`)
    catalog.value = res.packages || res.results || []
    if (res.categories && res.categories.length > 0) {
      categories.value = res.categories
    }
  } catch (err: any) {
    errorMsg.value = 'Lỗi tìm kiếm gói OpenWrt: ' + (err?.message || err)
  } finally {
    searching.value = false
  }
}

async function addGitHubOrFeedRepo() {
  if (!newRepoUrl.value.trim()) return
  addingRepo.value = true
  errorMsg.value = null
  notice.value = null
  try {
    const res = await apiRequest<{
      repos: AppRepoItem[]
      message: string
    }>('/packages/repos', {
      method: 'POST',
      body: JSON.stringify({
        name: newRepoName.value.trim(),
        url: newRepoUrl.value.trim(),
        branch: newRepoBranch.value.trim() || 'main',
      }),
    })
    if (res.repos) repos.value = res.repos
    showNotice(res.message || 'Đã thêm kho ứng dụng GitHub / OpenWrt thành công!')
    newRepoName.value = ''
    newRepoUrl.value = ''
    await searchOpenWrtPackages()
  } catch (err: any) {
    showError('Lỗi thêm Repo: ' + (err?.message || err))
  } finally {
    addingRepo.value = false
  }
}

async function deleteRepo(id: string) {
  errorMsg.value = null
  try {
    const res = await apiRequest<{ repos: AppRepoItem[]; message: string }>(
      `/packages/repos?id=${encodeURIComponent(id)}`,
      { method: 'DELETE' },
    )
    if (res.repos) repos.value = res.repos
    showNotice(res.message || 'Đã xóa kho ứng dụng.')
    await searchOpenWrtPackages()
  } catch (err: any) {
    showError('Không thể xóa kho: ' + (err?.message || err))
  }
}

async function syncAllFeeds() {
  syncingFeeds.value = true
  errorMsg.value = null
  notice.value = 'Đang đồng bộ danh sách gói từ OpenWrt Feeds & GitHub Repositories...'
  try {
    const res = await apiRequest<{
      repos: AppRepoItem[]
      output: string
      message: string
    }>('/packages/update-feeds', { method: 'POST' })
    if (res.repos) repos.value = res.repos
    if (res.output) terminalOutput.value = res.output
    showNotice(res.message || 'Đã cập nhật danh sách gói thành công!')
    await searchOpenWrtPackages()
  } catch (err: any) {
    showError('Lỗi đồng bộ kho gói: ' + (err?.message || err))
  } finally {
    syncingFeeds.value = false
  }
}

async function installPackage(pkgName: string, downloadUrl = '') {
  const target = pkgName.trim()
  if (!target && !downloadUrl) return
  pkgActionLoading.value = `install:${target || downloadUrl}`
  errorMsg.value = null
  notice.value = `Đang cài đặt ứng dụng '${target || downloadUrl}' trên hệ thống OpenWrt...`
  try {
    const res = await apiRequest<{
      package: string
      output: string
      message: string
    }>('/packages/install', {
      method: 'POST',
      body: JSON.stringify({
        package: target,
        download_url: downloadUrl,
        allow_untrusted: allowUntrusted.value,
      }),
    })
    terminalOutput.value = res.output || ''
    showNotice(res.message || `Đã cài đặt ${target} thành công!`)
    directInstallInput.value = ''
    await loadApps()
  } catch (err: any) {
    showError(`Lỗi cài đặt ${target}: ` + (err?.message || err))
  } finally {
    pkgActionLoading.value = null
  }
}

async function removePackage(pkgName: string) {
  pkgActionLoading.value = `remove:${pkgName}`
  errorMsg.value = null
  notice.value = `Đang gỡ cài đặt '${pkgName}'...`
  try {
    const res = await apiRequest<{
      package: string
      output: string
      message: string
    }>('/packages/remove', {
      method: 'POST',
      body: JSON.stringify({ package: pkgName }),
    })
    terminalOutput.value = res.output || ''
    showNotice(res.message || `Đã gỡ cài đặt ${pkgName}.`)
    await loadApps()
  } catch (err: any) {
    showError(`Lỗi gỡ cài đặt ${pkgName}: ` + (err?.message || err))
  } finally {
    pkgActionLoading.value = null
  }
}

async function handleDirectInstall() {
  const raw = directInstallInput.value.trim()
  if (!raw) return
  if (raw.startsWith('http://') || raw.startsWith('https://')) {
    const nameGuess = raw.split('/').pop() || 'github-pkg'
    await installPackage(nameGuess, raw)
  } else {
    await installPackage(raw, '')
  }
}

async function controlService(service: string, action: 'start' | 'stop' | 'restart' | 'enable' | 'disable') {
  actionLoading.value = `${service}:${action}`
  notice.value = `Executing /etc/init.d/${service} ${action}…`
  try {
    await apiRequest('/services/control', {
      method: 'POST',
      body: JSON.stringify({ service, action }),
    })
    await loadApps()
    notice.value = `Service '${service}' (${action}) completed!`
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    errorMsg.value = `Error on ${service}: ` + (err?.message || err)
  } finally {
    actionLoading.value = null
  }
}

onMounted(() => {
  void loadApps()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Floating Non-Shifting Notifications -->
    <div v-if="notice" class="r3p-toast r3p-toast-success">
      <span class="r3p-toast-text">{{ notice }}</span>
      <button type="button" class="text-xs opacity-80 hover:opacity-100 px-1 cursor-pointer" @click="notice = null">✕</button>
    </div>
    <div v-if="errorMsg" class="r3p-toast r3p-toast-error">
      <span class="r3p-toast-text">{{ errorMsg }}</span>
      <button type="button" class="text-xs opacity-80 hover:opacity-100 px-1 cursor-pointer" @click="errorMsg = null">✕</button>
    </div>

    <!-- 1. GitHub Repository & OpenWrt Package Feed Manager -->
    <div class="pro-card border border-sky-500/25">
      <div class="pro-card-header flex-wrap">
        <div class="min-w-0">
          <span class="pro-card-title text-sky-400">{{ t('apps_repo_heading') }}</span>
          <p class="text-[11px] text-gray-400 mt-0.5">
            {{ t('apps_repo_sub') }}
          </p>
        </div>
        <button
          type="button"
          class="btn-primary text-[11px] min-w-[295px]"
          :disabled="syncingFeeds"
          @click="syncAllFeeds"
        >
          {{ syncingFeeds ? 'Đang đồng bộ Feeds & GitHub…' : '🔄 Đồng Bộ Kho Ứng Dụng (apk update & GitHub)' }}
        </button>
      </div>

      <!-- Add GitHub Repo Form -->
      <form class="grid grid-cols-1 md:grid-cols-12 gap-2 mb-3 p-2.5 rounded bg-black/35 border border-white/5" @submit.prevent="addGitHubOrFeedRepo">
        <div class="md:col-span-5">
          <label class="block text-[10px] text-gray-400 uppercase mb-1">URL GitHub Repository hoặc OpenWrt Feed</label>
          <input
            v-model="newRepoUrl"
            type="text"
            required
            class="pro-input w-full mono text-xs"
            placeholder="https://github.com/openwrt/packages hoặc https://github.com/neopro90/R3P_Project.git"
          />
        </div>
        <div class="md:col-span-3">
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Tên Gợi Nhớ (Tùy chọn)</label>
          <input
            v-model="newRepoName"
            type="text"
            class="pro-input w-full text-xs"
            placeholder="VD: Kho ứng dụng OpenWrt Cộng Đồng"
          />
        </div>
        <div class="md:col-span-2">
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Nhánh (Branch)</label>
          <input
            v-model="newRepoBranch"
            type="text"
            class="pro-input w-full mono text-xs"
            placeholder="main / master"
          />
        </div>
        <div class="md:col-span-2 flex items-end">
          <button type="submit" class="btn-primary w-full text-xs py-1.5" :disabled="addingRepo || !newRepoUrl.trim()">
            {{ addingRepo ? 'Đang quét GitHub…' : '➕ Thêm Repo URL' }}
          </button>
        </div>
      </form>

      <!-- Configured Repositories Table -->
      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Tên Kho Ứng Dụng (Repo)</th>
              <th>Loại</th>
              <th>URL Nguồn (GitHub / Feed)</th>
              <th>Nhánh</th>
              <th>Số Gói</th>
              <th>Cập Nhật Cuối</th>
              <th>Thao Tác</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in repos" :key="r.id">
              <td>
                <div class="font-semibold text-gray-200 text-xs">{{ r.name }}</div>
                <div class="text-[10.5px] text-gray-400">{{ r.description }}</div>
              </td>
              <td>
                <span :class="r.type === 'github' ? 'badge badge-purple' : 'badge badge-blue'">
                  {{ r.type === 'github' ? 'GITHUB REPO' : 'OPENWRT FEED' }}
                </span>
              </td>
              <td class="mono text-[11px] text-sky-400 max-w-xs truncate" :title="r.url">
                <a :href="r.url" target="_blank" rel="noopener" class="hover:underline">{{ r.url }}</a>
              </td>
              <td class="mono text-gray-300 text-xs">{{ r.branch }}</td>
              <td class="mono text-emerald-400 font-semibold">{{ r.pkg_count }}</td>
              <td class="mono text-gray-400 text-[11px]">{{ r.last_synced || '—' }}</td>
              <td>
                <button
                  v-if="r.id !== 'repo-openwrt-official'"
                  type="button"
                  class="btn-danger text-[10px] py-0.5 px-2"
                  @click="deleteRepo(r.id)"
                >
                  Xóa
                </button>
                <span v-else class="text-[10px] text-gray-500">Mặc định</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 2. Search & Install OpenWrt / GitHub Applications -->
    <div class="pro-card border border-emerald-500/25">
      <div class="pro-card-header flex-wrap gap-2">
        <div>
          <span class="pro-card-title text-emerald-400">🔍 Tìm Kiếm & Cài Đặt Ứng Dụng OpenWrt / GitHub ({{ catalog.length }} kết quả)</span>
          <p class="text-[11px] text-gray-400 mt-0.5">
            Tìm gói từ kho OpenWrt (<code class="mono">mipsel_24kc</code>) &amp; các GitHub Repo đã thêm, hoặc nhập tên gói / link file <code class="mono">.apk / .ipk</code> để cài đặt an toàn.
          </p>
        </div>

        <!-- Direct package name or URL installer -->
        <form class="flex flex-wrap items-center gap-2" @submit.prevent="handleDirectInstall">
          <input
            v-model="directInstallInput"
            type="text"
            class="pro-input w-72 mono text-xs"
            placeholder="Nhập tên gói (VD: htop, iperf3) hoặc URL .apk/.ipk..."
          />
          <label class="flex items-center gap-1.5 text-[11px] text-gray-300 cursor-pointer select-none" title="Chỉ bật khi cài gói tự biên dịch không chữ ký OpenWrt">
            <input v-model="allowUntrusted" type="checkbox" class="accent-amber-500" />
            <span>allow_untrusted</span>
          </label>
          <button
            type="submit"
            class="btn-primary text-xs whitespace-nowrap"
            :disabled="pkgActionLoading !== null || !directInstallInput.trim()"
          >
            ⚡ Cài Đặt Ngay
          </button>
        </form>
      </div>

      <!-- Search & Category Filter Bar -->
      <div class="flex flex-wrap items-center gap-2 mb-3">
        <div class="flex-1 min-w-[220px] flex gap-1.5">
          <input
            v-model="searchQuery"
            type="search"
            class="pro-input flex-1 text-xs"
            placeholder="Tìm ứng dụng OpenWrt (VD: wireguard, tailscale, samba, htop, tcpdump, aria2, mosquitto,luci-app...)..."
            @keyup.enter="searchOpenWrtPackages"
          />
          <button type="button" class="btn-secondary text-xs px-3 min-w-[104px]" :disabled="searching" @click="searchOpenWrtPackages">
            {{ searching ? 'Đang tìm…' : '🔍 Tìm Kiếm' }}
          </button>
        </div>

        <div class="flex items-center gap-1 overflow-x-auto">
          <button
            v-for="cat in ['All', ...categories.filter(c => c !== 'All')]"
            :key="cat"
            type="button"
            class="px-2.5 py-1 rounded text-[11px] border transition whitespace-nowrap cursor-pointer"
            :class="selectedCategory === cat
              ? 'bg-sky-500/20 border-sky-500/50 text-sky-300 font-semibold'
              : 'bg-black/30 border-white/10 text-gray-400 hover:text-gray-200'"
            @click="selectedCategory = cat; searchOpenWrtPackages()"
          >
            {{ cat }}
          </button>
        </div>
      </div>

      <!-- Search Results Grid -->
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 max-h-[460px] overflow-y-auto p-0.5">
        <div
          v-for="app in catalog"
          :key="app.name + app.source"
          class="p-3 rounded-xl bg-black/40 border border-white/10 hover:border-sky-500/40 transition flex flex-col justify-between gap-2.5 shadow-sm min-w-0"
        >
          <div>
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <span class="mono font-bold text-[18px] text-sky-300 truncate block">{{ app.name }}</span>
                <span class="mono text-[13px] text-gray-300 font-semibold">{{ app.version }}</span>
              </div>
              <span class="badge text-[11px] font-bold px-2.5 py-0.5 shrink-0" :class="app.installed ? 'badge-green' : 'badge-gray'">
                {{ app.installed ? 'INSTALLED' : app.category }}
              </span>
            </div>
            <p class="text-[13px] text-gray-200 mt-1.5 line-clamp-2 leading-relaxed">{{ app.description }}</p>
          </div>

          <div class="flex items-center justify-between pt-2.5 border-t border-white/5">
            <span class="text-[11.5px] text-gray-400 mono font-medium truncate max-w-[170px]" :title="app.source">
              {{ app.source }}
            </span>
            <div class="flex items-center gap-1.5">
              <button
                v-if="!app.installed"
                type="button"
                class="btn-primary text-[11.5px] font-bold py-1 px-3 min-w-[92px]"
                :disabled="pkgActionLoading !== null"
                @click="installPackage(app.name, app.download_url || '')"
              >
                {{ pkgActionLoading === `install:${app.name}` ? 'Đang cài…' : '⬇ Cài Đặt' }}
              </button>
              <button
                v-else-if="app.name !== 'r3p-api'"
                type="button"
                class="btn-danger text-[11.5px] font-bold py-1 px-3 min-w-[86px]"
                :disabled="pkgActionLoading !== null"
                @click="removePackage(app.name)"
              >
                {{ pkgActionLoading === `remove:${app.name}` ? 'Đang gỡ…' : '🗑 Gỡ Bỏ' }}
              </button>
              <span v-else class="badge badge-blue text-[10px]">CORE PROTECTED</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Terminal Output Log for Package Operations -->
      <div v-if="terminalOutput" class="mt-3 rounded bg-black/75 border border-white/10 p-2.5">
        <div class="flex items-center justify-between mb-1">
          <span class="text-[10.5px] font-bold uppercase text-emerald-400 mono">Package Manager Console Output (apk / opkg)</span>
          <button type="button" class="text-[10.5px] text-gray-400 hover:text-white cursor-pointer" @click="terminalOutput = ''">✕ Xóa Log</button>
        </div>
        <pre class="mono text-[10.5px] text-gray-300 whitespace-pre-wrap max-h-40 overflow-y-auto">{{ terminalOutput }}</pre>
      </div>
    </div>

    <!-- 3. Services Management Table -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">⚙ Quản Lý Dịch Vụ Hệ Thống (/etc/init.d & procd)</span>
        <div class="flex items-center gap-2">
          <span class="text-[11px] text-gray-400">{{ services.filter(s => s.running).length }} / {{ services.length }} Running</span>
          <button type="button" class="btn-secondary text-[11px] py-1" :disabled="loading" @click="loadApps">Refresh ↻</button>
        </div>
      </div>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Daemon / Init Script</th>
              <th>Process Status</th>
              <th>Boot Autostart</th>
              <th>PID</th>
              <th>RAM (VmRSS)</th>
              <th>Description</th>
              <th>Service Controls</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in services" :key="s.name">
              <td class="mono font-semibold text-sky-400">/etc/init.d/{{ s.name }}</td>
              <td>
                <span :class="s.running ? 'badge badge-green' : 'badge badge-gray'">
                  {{ s.running ? 'RUNNING' : 'STOPPED' }}
                </span>
              </td>
              <td>
                <span :class="s.enabled ? 'badge badge-blue' : 'badge badge-orange'">
                  {{ s.enabled ? 'ENABLED' : 'DISABLED' }}
                </span>
              </td>
              <td class="mono text-gray-300">{{ s.pid || '—' }}</td>
              <td class="mono text-emerald-400">{{ s.memory_kb ? `${(s.memory_kb / 1024).toFixed(2)} MB` : '—' }}</td>
              <td class="text-gray-400 max-w-xs truncate">{{ s.description }}</td>
              <td>
                <div class="flex items-center gap-1">
                  <button
                    v-if="s.name !== 'r3p-api'"
                    type="button"
                    class="btn-secondary text-[10px] py-0.5 px-2 min-w-[46px]"
                    :disabled="actionLoading !== null"
                    @click="controlService(s.name, s.running ? 'stop' : 'start')"
                  >
                    {{ s.running ? 'Stop' : 'Start' }}
                  </button>
                  <button
                    v-if="s.name !== 'r3p-api'"
                    type="button"
                    class="btn-primary text-[10px] py-0.5 px-2"
                    :disabled="actionLoading !== null"
                    @click="controlService(s.name, 'restart')"
                  >
                    Restart
                  </button>
                  <button
                    v-if="s.name !== 'r3p-api'"
                    type="button"
                    class="btn-secondary text-[10px] py-0.5 px-2 min-w-[82px]"
                    :disabled="actionLoading !== null"
                    @click="controlService(s.name, s.enabled ? 'disable' : 'enable')"
                  >
                    {{ s.enabled ? 'Disable Boot' : 'Enable Boot' }}
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 4. Installed APK Packages Catalog -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Danh Sách Gói Đã Cài Đặt Trên Hệ Thống (apk info -v · {{ packages.length }} Gói)</span>
        <input
          v-model="pkgFilter"
          type="search"
          class="pro-input w-56 mono text-[11.5px]"
          placeholder="Lọc gói đã cài đặt…"
        />
      </div>

      <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2 max-h-56 overflow-y-auto p-1">
        <div
          v-for="pkg in filteredPackages"
          :key="pkg"
          class="px-2.5 py-1.5 rounded-lg bg-black/40 border border-white/5 mono text-[12px] font-medium text-gray-200 hover:text-white hover:border-sky-500/30 transition truncate"
          :title="pkg"
        >
          {{ pkg }}
        </div>
      </div>
    </div>
  </div>
</template>
