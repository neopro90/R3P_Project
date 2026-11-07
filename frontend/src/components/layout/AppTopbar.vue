<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useSystemStore } from '@/stores/system'
import { useAuthStore } from '@/stores/auth'
import { useUiPreferences, type HassThemeStyle } from '@/composables/useUiPreferences'

const route = useRoute()
const system = useSystemStore()
const auth = useAuthStore()
const prefs = useUiPreferences()
const checkingNet = ref(false)
const copiedIp = ref(false)
const emit = defineEmits<{ openMobileSidebar: [] }>()

const routeTitleKeys: Record<string, string> = {
  '/': 'title_dashboard',
  '/ai': 'title_ai',
  '/network': 'title_network',
  '/wireless': 'title_wireless',
  '/adblock': 'title_adblock',
  '/firewall': 'title_firewall',
  '/access': 'title_access',
  '/config': 'title_config',
  '/downloads': 'title_downloads',
  '/nas': 'title_nas',
  '/files': 'title_explorer',
  '/apps': 'title_apps',
  '/automation': 'title_automation',
  '/iot': 'title_iot',
  '/plugins': 'title_plugins',
  '/system': 'title_system',
}

const currentTitle = computed(() => {
  const key = routeTitleKeys[route.path]
  return key ? prefs.t(key) : 'R3P Home Server OS'
})

const wanModeLabel = computed(() => {
  const m = system.internet?.wan_mode || ''
  if (m === 'usb_tether') return 'USB TETHER'
  if (m === 'wifi_client') return 'WIFI CLIENT'
  if (m === 'ethernet_wan') return 'WAN ETH'
  if (system.internet?.wan_iface) return system.internet.wan_iface.toUpperCase()
  return 'WAN'
})

const hassThemes: Array<{ id: HassThemeStyle; icon: string; labelKey: string }> = [
  { id: 'hass-classic', icon: '🏠', labelKey: 'hass_classic' },
  { id: 'hass-mushroom', icon: '🍄', labelKey: 'hass_mushroom' },
  { id: 'hass-neon', icon: '⚡', labelKey: 'hass_neon' },
]

const currentLangBadge = computed(() => {
  const l = prefs.state.lang
  if (l === 'cn') return 'CN'
  if (l === 'en') return 'EN'
  return 'VI'
})

function rotateLang() {
  const cur = prefs.state.lang
  if (cur === 'vi') prefs.setLang('en')
  else if (cur === 'en') prefs.setLang('cn')
  else prefs.setLang('vi')
}

async function recheckInternet() {
  if (checkingNet.value) return
  checkingNet.value = true
  try {
    await system.refreshInternet(true)
  } finally {
    checkingNet.value = false
  }
}

async function copyExternalIp() {
  const ip = system.internet?.external_ip
  if (!ip || ip === '—') return
  try {
    await navigator.clipboard.writeText(ip)
    copiedIp.value = true
    setTimeout(() => {
      copiedIp.value = false
    }, 1500)
  } catch {
    // ignore
  }
}

function signOut() {
  void auth.logout()
}
</script>

<template>
  <header class="h-14 lg:h-[72px] px-3 lg:px-5 bg-[#0d121e] border-b border-[var(--border-subtle)] flex items-center justify-between shrink-0 select-none gap-2 lg:gap-3">
    <!-- Breadcrumb / Title & Mobile Hamburger Button -->
    <div class="flex items-center gap-2 lg:gap-3 min-w-0">
      <!-- Hamburger Menu Button for Mobile 9:16 -->
      <button
        type="button"
        class="lg:hidden p-1.5 rounded-lg bg-black/40 border border-white/10 text-gray-200 hover:text-white hover:bg-sky-500/20 hover:border-sky-500/40 transition shrink-0 cursor-pointer"
        title="Mở menu danh mục"
        @click="emit('openMobileSidebar')"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
        </svg>
      </button>

      <span class="text-[13px] lg:text-[16px] font-bold text-sky-400 uppercase tracking-wider shrink-0 hidden sm:inline">R3P·SERVER</span>
      <span class="text-[14px] lg:text-[17px] text-gray-500 shrink-0 hidden sm:inline">/</span>
      <h1 class="text-[15px] lg:text-[21px] font-bold text-gray-100 truncate">{{ currentTitle }}</h1>
    </div>

    <!-- Live Telemetry, Hass.io Style Switcher, Theme Toggle & 1-Click Language Switcher -->
    <div class="flex items-center gap-1.5 lg:gap-2 text-[12px] lg:text-[13.5px] shrink-0">
      <!-- 1. 3 Home Assistant (Hass.io) Theme Presets (Desktop & Tablet) -->
      <div class="hidden md:flex items-center bg-black/25 border border-white/10 rounded-lg p-1 gap-1" title="Chọn 1 trong 3 bộ giao diện phong cách Home Assistant (Hass.io)">
        <button
          v-for="ht in hassThemes"
          :key="ht.id"
          type="button"
          class="px-2.5 py-1 lg:px-3 lg:py-1.5 rounded-lg text-xs lg:text-[13px] font-semibold transition flex items-center gap-1.5 cursor-pointer"
          :class="prefs.state.hassStyle === ht.id
            ? 'bg-sky-600/80 text-sky-100 border border-sky-400/30 shadow-sm'
            : 'text-gray-400 hover:text-gray-200'"
          @click="prefs.setHassStyle(ht.id)"
        >
          <span>{{ ht.icon }}</span>
          <span class="hidden xl:inline">{{ prefs.t(ht.labelKey) }}</span>
        </button>
      </div>

      <!-- 2. 1-Click Light / Dark Mode Button -->
      <button
        type="button"
        class="flex items-center justify-center gap-1.5 px-2.5 py-1 lg:px-3.5 lg:py-1.5 sm:min-w-[82px] rounded-lg bg-black/25 border border-white/10 hover:border-sky-500/40 text-gray-200 font-semibold transition cursor-pointer text-xs lg:text-[13px] shrink-0"
        title="Đổi giao diện Sáng / Tối (Light / Dark Mode)"
        @click="prefs.toggleTheme()"
      >
        <span>{{ prefs.state.theme === 'light' ? '☀️' : '🌙' }}</span>
        <span class="hidden sm:inline">{{ prefs.state.theme === 'light' ? prefs.t('theme_light') : prefs.t('theme_dark') }}</span>
      </button>

      <!-- 3. Single 1-Click Rotating Language Button (VI ➔ EN ➔ CN ➔ VI) -->
      <button
        type="button"
        class="px-2.5 py-1 lg:px-3.5 lg:py-1.5 min-w-[54px] justify-center rounded-lg border border-emerald-500/40 bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/20 text-xs lg:text-[13px] font-bold transition flex items-center gap-1 cursor-pointer shadow-sm shrink-0"
        title="Nhấn để đổi ngôn ngữ (Xoay vòng: VI ➔ EN ➔ CN)"
        @click="rotateLang"
      >
        <span>🌐</span>
        <span>{{ currentLangBadge }}</span>
      </button>

      <!-- 4. Internet Status Badge -->
      <button
        type="button"
        class="flex items-center justify-center gap-1.5 px-2.5 py-1 sm:min-w-[104px] rounded-md border text-xs font-semibold cursor-pointer select-none transition shrink-0"
        :class="system.internet?.online
          ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400 hover:bg-emerald-500/20'
          : 'bg-rose-500/10 border-rose-500/30 text-rose-400 hover:bg-rose-500/20'"
        title="Bấm để kiểm tra lại kết nối Internet"
        :disabled="checkingNet"
        @click="recheckInternet"
      >
        <span
          class="w-1.5 h-1.5 rounded-full shrink-0"
          :class="checkingNet ? 'bg-sky-400 animate-ping' : (system.internet?.online ? 'bg-emerald-400' : 'bg-rose-400')"
        ></span>
        <span class="hidden sm:inline font-mono text-[11px] text-center">
          {{ checkingNet ? 'CHECKING…' : wanModeLabel }}
        </span>
      </button>

      <!-- External Public IP (Desktop) -->
      <button
        type="button"
        class="hidden xl:flex items-center justify-center gap-1.5 px-2.5 py-1 min-w-[148px] rounded-md bg-white/5 border border-white/10 text-gray-300 hover:border-sky-500/40 text-xs font-mono transition cursor-pointer shrink-0"
        :title="system.internet?.isp ? `ISP: ${system.internet.isp} · Click để copy IP` : 'IP Công Cộng Ngoài Internet · Click để copy'"
        @click="copyExternalIp"
      >
        <span class="text-gray-400 shrink-0">IP:</span>
        <span class="text-sky-400 font-semibold min-w-[108px] text-center truncate">
          {{ copiedIp ? '✓ Đã copy!' : (system.internet?.external_ip || '—') }}
        </span>
      </button>

      <!-- 5. User Profile Menu -->
      <button
        type="button"
        class="px-2 py-1 rounded-md bg-white/5 border border-white/10 text-gray-300 hover:text-rose-400 text-xs font-mono font-bold transition cursor-pointer"
        title="Đăng xuất khỏi phiên quản trị Router"
        @click="signOut"
      >
        root ✕
      </button>
    </div>
  </header>
</template>
