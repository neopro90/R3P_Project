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
  <header class="h-11 px-3.5 bg-[#0d121e] border-b border-[var(--border-subtle)] flex items-center justify-between shrink-0 select-none gap-2 overflow-x-auto">
    <!-- Breadcrumb / Title -->
    <div class="flex items-center gap-2 min-w-0">
      <span class="text-[11px] font-bold text-sky-400 uppercase tracking-wider shrink-0">HASS·R3P</span>
      <span class="text-gray-500 shrink-0">/</span>
      <h1 class="text-[12.5px] font-semibold text-gray-200 truncate">{{ currentTitle }}</h1>
    </div>

    <!-- Live Telemetry, Hass.io Style Switcher, Theme Toggle & 1-Click Language Switcher -->
    <div class="flex items-center gap-1.5 text-[11px] shrink-0">
      <!-- 1. 3 Home Assistant (Hass.io) Theme Presets -->
      <div class="flex items-center bg-black/25 border border-white/10 rounded-lg p-0.5 gap-0.5" title="Chọn 1 trong 3 bộ giao diện phong cách Home Assistant (Hass.io)">
        <button
          v-for="ht in hassThemes"
          :key="ht.id"
          type="button"
          class="px-2 py-0.5 rounded-md text-[10.5px] font-semibold transition flex items-center gap-1 cursor-pointer"
          :class="prefs.state.hassStyle === ht.id
            ? 'bg-sky-500 text-white shadow-sm'
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
        class="flex items-center gap-1 px-2.5 py-1 rounded-lg bg-black/25 border border-white/10 hover:border-sky-500/40 text-gray-200 font-semibold transition cursor-pointer"
        title="Đổi giao diện Sáng / Tối (Light / Dark Mode)"
        @click="prefs.toggleTheme()"
      >
        <span>{{ prefs.state.theme === 'light' ? '☀️' : '🌙' }}</span>
        <span class="hidden sm:inline text-[10.5px]">{{ prefs.state.theme === 'light' ? prefs.t('theme_light') : prefs.t('theme_dark') }}</span>
      </button>

      <!-- 3. Single 1-Click Rotating Language Button (VI ➔ EN ➔ CN ➔ VI) -->
      <button
        type="button"
        class="px-2.5 py-1 rounded-lg border border-emerald-500/40 bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/20 text-[10.5px] font-bold transition flex items-center gap-1.5 cursor-pointer shadow-sm"
        title="Nhấn để đổi ngôn ngữ (Xoay vòng: VI ➔ EN ➔ CN)"
        @click="rotateLang"
      >
        <span>🌐</span>
        <span class="mono tracking-wider font-extrabold">{{ currentLangBadge }}</span>
      </button>

      <!-- Internet Connectivity Status -->
      <button
        class="flex items-center gap-1.5 px-2.5 py-1 rounded-lg border transition cursor-pointer"
        :class="system.internet?.online
          ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400 hover:bg-emerald-500/20'
          : 'bg-amber-500/10 border-amber-500/30 text-amber-400 hover:bg-amber-500/20'"
        :title="system.internet?.online
          ? `Internet Online via ${system.internet.wan_iface || 'WAN'} (${system.internet.wan_local_ip || 'DHCP'}) · Gateway ${system.internet.gateway || '—'} · Click to refresh`
          : 'No upstream Internet detected · Click to re-check'"
        @click="recheckInternet"
      >
        <span
          class="w-1.5 h-1.5 rounded-full"
          :class="[
            system.internet?.online ? 'bg-emerald-400 animate-pulse' : 'bg-amber-400',
            checkingNet ? 'animate-ping' : ''
          ]"
        ></span>
        <span class="font-semibold tracking-wide text-[10.5px]">
          {{ checkingNet ? prefs.t('checking') : (system.internet?.online ? prefs.t('internet_online') : prefs.t('internet_offline')) }}
        </span>
        <span v-if="system.internet?.online" class="mono text-[10px] opacity-90 hidden sm:inline">
          · {{ wanModeLabel }}
          <template v-if="system.internet.latency_ms > 0">({{ system.internet.latency_ms }}ms)</template>
        </span>
      </button>

      <!-- External Public IP -->
      <button
        class="flex items-center gap-1.5 px-2 py-1 rounded-lg bg-black/25 border border-white/10 text-gray-300 hover:border-sky-500/40 transition cursor-pointer"
        :title="system.internet?.isp ? `ISP: ${system.internet.isp} (${system.internet.country || ''}) · Click to copy External IP` : 'External Public IP · Click to copy'"
        @click="copyExternalIp"
      >
        <span class="text-gray-500 text-[10px] font-semibold">{{ prefs.t('ext_ip') }}</span>
        <span class="mono font-medium" :class="system.internet?.external_ip && system.internet.external_ip !== '—' ? 'text-sky-400' : 'text-gray-500'">
          {{ copiedIp ? prefs.t('copied') : (system.internet?.external_ip || '—') }}
        </span>
      </button>

      <!-- CPU & RAM -->
      <div class="hidden md:flex items-center gap-1.5 px-2 py-1 rounded-lg bg-black/20 border border-white/5 text-gray-400">
        <span class="text-gray-500 text-[10px]">CPU</span>
        <span class="mono text-sky-400 font-medium">{{ (system.cpu || 0).toFixed(0) }}%</span>
        <span class="text-gray-600">·</span>
        <span class="text-gray-500 text-[10px]">RAM</span>
        <span class="mono text-emerald-400 font-medium">{{ (system.ram || 0).toFixed(0) }}%</span>
      </div>

      <!-- User Chip & Logout -->
      <div class="flex items-center gap-1 pl-1 border-l border-white/10">
        <span class="mono text-[11px] text-gray-300 font-medium hidden sm:inline">root</span>
        <button
          class="p-1 rounded text-gray-400 hover:text-red-400 hover:bg-red-500/10 transition cursor-pointer"
          title="Sign out"
          @click="signOut"
        >
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
          </svg>
        </button>
      </div>
    </div>
  </header>
</template>
