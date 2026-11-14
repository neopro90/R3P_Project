<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useSystemStore } from '@/stores/system'
import { useAuthStore } from '@/stores/auth'
import { useUiPreferences, type HassThemeStyle } from '@/composables/useUiPreferences'
import { copyToClipboard } from '@/composables/useClipboard'

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
  const key = system.internet?.wan_mode_key || ''
  const raw = system.internet?.wan_mode || ''
  if (key === 'usb_tether' || raw === 'usb_tether' || raw.includes('USB Tether')) return prefs.t('wan_usb_tether')
  if (key === 'wifi_client' || raw === 'wifi_client' || raw.includes('Wi-Fi Client')) return prefs.t('wan_wifi_client')
  if (key === 'ethernet_wan' || raw === 'ethernet_wan' || raw.includes('Ethernet WAN')) return prefs.t('wan_eth')
  if (key === 'offline' || raw.toLowerCase() === 'offline' || raw.toLowerCase() === 'disconnected') return prefs.t('wan_offline')
  if (system.internet?.wan_iface) return system.internet.wan_iface.toUpperCase()
  return 'WAN'
})

const accentColors: Record<string, string> = {
  blue: '#0284c7',
  teal: '#0d9488',
  purple: '#7c3aed',
  green: '#059669',
  orange: '#ea580c',
}

const currentAccentHex = computed(() => accentColors[prefs.state.accent] || '#0284c7')

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
  const ok = await copyToClipboard(ip)
  if (ok) {
    copiedIp.value = true
    setTimeout(() => {
      copiedIp.value = false
    }, 1500)
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
        :title="prefs.t('open_menu_tooltip')"
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

    <!-- Live Telemetry, Hass.io Style Switcher, Accent, Theme Toggle & 1-Click Language Switcher -->
    <div class="flex items-center gap-1.5 lg:gap-2 text-[12px] lg:text-[13.5px] shrink-0">
      <!-- 1. 3 Home Assistant (Hass.io) Theme Presets (Desktop & Tablet) -->
      <div class="hidden md:flex items-center bg-black/25 border border-white/10 rounded-lg p-1 gap-1" :title="prefs.t('hass_tooltip')">
        <button
          v-for="ht in hassThemes"
          :key="ht.id"
          type="button"
          class="px-2.5 py-1 lg:px-3 lg:py-1.5 rounded-lg text-xs lg:text-[13px] font-semibold transition flex items-center gap-1.5 cursor-pointer"
          :class="prefs.state.hassStyle === ht.id
            ? 'bg-sky-600 keep-white border border-sky-400/30 shadow-sm'
            : 'text-gray-400 hover:text-gray-200'"
          @click="prefs.setHassStyle(ht.id)"
        >
          <span>{{ ht.icon }}</span>
          <span class="hidden xl:inline">{{ prefs.t(ht.labelKey) }}</span>
        </button>
      </div>

      <!-- 1b. Accent Color Cyclable Chip -->
      <button
        type="button"
        class="hidden sm:flex items-center justify-center p-1.5 lg:px-2.5 lg:py-1.5 rounded-lg bg-black/25 border border-white/10 hover:border-sky-500/40 transition cursor-pointer shrink-0"
        :title="`${prefs.t('accent_tooltip')}: ${prefs.state.accent}`"
        @click="prefs.cycleAccent()"
      >
        <span class="w-3.5 h-3.5 rounded-full border border-white/40 shadow-sm" :style="{ backgroundColor: currentAccentHex }"></span>
      </button>

      <!-- 2. 1-Click Light / Dark Mode Button -->
      <button
        type="button"
        class="flex items-center justify-center gap-1.5 px-2.5 py-1 lg:px-3.5 lg:py-1.5 sm:min-w-[82px] rounded-lg bg-black/25 border border-white/10 hover:border-sky-500/40 text-gray-200 font-semibold transition cursor-pointer text-xs lg:text-[13px] shrink-0"
        :title="prefs.t('theme_toggle_tooltip')"
        @click="prefs.toggleTheme()"
      >
        <span>{{ prefs.state.theme === 'light' ? '☀️' : '🌙' }}</span>
        <span class="hidden sm:inline">{{ prefs.state.theme === 'light' ? prefs.t('theme_light') : prefs.t('theme_dark') }}</span>
      </button>

      <!-- 3. Single 1-Click Rotating Language Button (VI ➔ EN ➔ CN ➔ VI) -->
      <button
        type="button"
        class="badge badge-green px-2.5 py-1 lg:px-3.5 lg:py-1.5 min-w-[54px] justify-center rounded-lg text-xs lg:text-[13px] font-bold transition flex items-center gap-1 cursor-pointer shadow-sm shrink-0"
        :title="prefs.t('lang_toggle_tooltip')"
        @click="rotateLang"
      >
        <span>🌐</span>
        <span>{{ currentLangBadge }}</span>
      </button>

      <!-- 4. Internet Status Badge -->
      <button
        type="button"
        class="badge px-2.5 py-1 sm:min-w-[104px] justify-center rounded-md text-xs font-semibold cursor-pointer select-none transition shrink-0"
        :class="system.internet?.online ? 'badge-green' : 'badge-red'"
        :title="prefs.t('recheck_net_tooltip')"
        :disabled="checkingNet"
        @click="recheckInternet"
      >
        <span
          class="w-1.5 h-1.5 rounded-full shrink-0"
          :class="checkingNet ? 'bg-sky-400 animate-ping' : (system.internet?.online ? 'bg-emerald-500' : 'bg-rose-500')"
        ></span>
        <span class="hidden sm:inline font-mono text-[11px] text-center">
          {{ checkingNet ? prefs.t('checking') : wanModeLabel }}
        </span>
      </button>

      <!-- External Public IP (Desktop) -->
      <button
        type="button"
        class="hidden xl:flex items-center justify-center gap-1.5 px-2.5 py-1 min-w-[148px] rounded-md bg-white/5 border border-white/10 text-gray-300 hover:border-sky-500/40 text-xs font-mono transition cursor-pointer shrink-0"
        :title="system.internet?.isp ? `ISP: ${system.internet.isp}` : prefs.t('ext_ip')"
        @click="copyExternalIp"
      >
        <span class="text-gray-400 shrink-0">IP:</span>
        <span class="text-sky-500 font-semibold min-w-[108px] text-center truncate">
          {{ copiedIp ? prefs.t('copied') : (system.internet?.external_ip || '—') }}
        </span>
      </button>

      <!-- 5. User Profile Menu -->
      <button
        type="button"
        class="px-2 py-1 rounded-md bg-white/5 border border-white/10 text-gray-300 hover:text-rose-500 text-xs font-mono font-bold transition cursor-pointer"
        :title="prefs.t('logout_tooltip')"
        @click="signOut"
      >
        root ✕
      </button>
    </div>
  </header>
</template>
