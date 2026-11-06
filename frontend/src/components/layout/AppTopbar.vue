<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useSystemStore } from '@/stores/system'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const system = useSystemStore()
const auth = useAuthStore()
const checkingNet = ref(false)
const copiedIp = ref(false)

const pageTitles: Record<string, string> = {
  '/': 'Dashboard Overview',
  '/network': 'Network Monitor',
  '/wireless': 'Wireless AP & Wi-Fi Client (MT7615N 4x4)',
  '/firewall': 'Firewall & NAT',
  '/access': 'Access & Accounts',
  '/config': 'System Config (UCI)',
  '/downloads': 'Download Bot (ESP32 Linux Host + yt-dlp)',
  '/nas': 'NAS & USB 3.0 Dual-Role (Storage / Tether)',
  '/ai': 'AI Root System Agent',
  '/apps': 'Applications & Services',
  '/automation': 'Automation & Scheduler',
  '/iot': 'IoT (MQTT)',
  '/plugins': 'Plugins Manager',
  '/system': 'System & Diagnostics',
}

const currentTitle = computed(() => pageTitles[route.path] || 'Home Server OS')

const wanModeLabel = computed(() => {
  const m = system.internet?.wan_mode || ''
  if (m === 'usb_tether') return 'USB TETHER'
  if (m === 'wifi_client') return 'WIFI CLIENT'
  if (m === 'ethernet_wan') return 'WAN ETH'
  if (system.internet?.wan_iface) return system.internet.wan_iface.toUpperCase()
  return 'WAN'
})

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
  <header class="h-10 px-4 bg-[#0d121e] border-b border-[rgba(255,255,255,0.08)] flex items-center justify-between shrink-0 select-none gap-2">
    <!-- Breadcrumb / Title -->
    <div class="flex items-center gap-2 min-w-0">
      <span class="text-[11px] font-semibold text-gray-500 uppercase tracking-wider shrink-0">R3P</span>
      <span class="text-gray-600 shrink-0">/</span>
      <h1 class="text-[12.5px] font-semibold text-gray-200 truncate">{{ currentTitle }}</h1>
    </div>

    <!-- Live Telemetry & Internet Status Bar -->
    <div class="flex items-center gap-2 text-[11px] shrink-0">
      <!-- Internet Connectivity Status -->
      <button
        class="flex items-center gap-1.5 px-2 py-0.5 rounded border transition cursor-pointer"
        :class="system.internet?.online
          ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300 hover:bg-emerald-500/20'
          : 'bg-amber-500/10 border-amber-500/30 text-amber-300 hover:bg-amber-500/20'"
        :title="system.internet?.online
          ? `Internet Online via ${system.internet.wan_iface || 'WAN'} (${system.internet.wan_local_ip || 'DHCP'}) · Gateway ${system.internet.gateway || '—'} · Click to refresh`
          : 'No upstream Internet detected (Plug in WAN cable, USB Tether phone/4G, or connect Wireless Client) · Click to re-check'"
        @click="recheckInternet"
      >
        <span
          class="w-1.5 h-1.5 rounded-full"
          :class="[
            system.internet?.online ? 'bg-emerald-400 animate-pulse' : 'bg-amber-400',
            checkingNet ? 'animate-ping' : ''
          ]"
        ></span>
        <span class="font-semibold tracking-wide">
          {{ checkingNet ? 'CHECKING...' : (system.internet?.online ? 'INTERNET: ONLINE' : 'INTERNET: OFFLINE') }}
        </span>
        <span v-if="system.internet?.online" class="mono text-[10px] text-emerald-400/90 hidden sm:inline">
          · {{ wanModeLabel }}
          <template v-if="system.internet.latency_ms > 0">({{ system.internet.latency_ms }}ms)</template>
        </span>
      </button>

      <!-- External Public IP -->
      <button
        class="flex items-center gap-1.5 px-2 py-0.5 rounded bg-black/25 border border-white/10 text-gray-300 hover:border-sky-500/40 transition cursor-pointer"
        :title="system.internet?.isp ? `ISP: ${system.internet.isp} (${system.internet.country || ''}) · Click to copy External IP` : 'External Public IP · Click to copy'"
        @click="copyExternalIp"
      >
        <span class="text-gray-500 text-[10px] font-semibold">EXT IP</span>
        <span class="mono font-medium" :class="system.internet?.external_ip && system.internet.external_ip !== '—' ? 'text-sky-300' : 'text-gray-500'">
          {{ copiedIp ? 'COPIED!' : (system.internet?.external_ip || '—') }}
        </span>
      </button>

      <!-- Uptime -->
      <div class="hidden lg:flex items-center gap-1.5 px-2 py-0.5 rounded bg-black/20 border border-white/5 text-gray-400">
        <span class="text-gray-500 text-[10px]">UPTIME</span>
        <span class="mono text-gray-200 font-medium">{{ system.uptimeFormatted || '0m' }}</span>
      </div>

      <!-- CPU -->
      <div class="hidden md:flex items-center gap-1.5 px-2 py-0.5 rounded bg-black/20 border border-white/5 text-gray-400">
        <span class="text-gray-500 text-[10px]">CPU</span>
        <span class="mono text-sky-400 font-medium">{{ (system.cpu || 0).toFixed(1) }}%</span>
      </div>

      <!-- RAM -->
      <div class="hidden md:flex items-center gap-1.5 px-2 py-0.5 rounded bg-black/20 border border-white/5 text-gray-400">
        <span class="text-gray-500 text-[10px]">RAM</span>
        <span class="mono text-emerald-400 font-medium">{{ (system.ram || 0).toFixed(0) }}%</span>
      </div>

      <!-- Router LAN Link -->
      <div class="flex items-center gap-1.5 px-2 py-0.5 rounded text-[10.5px]" :class="system.connected ? 'text-emerald-400 bg-emerald-500/10 border border-emerald-500/20' : 'text-red-400 bg-red-500/10 border border-red-500/20'">
        <span class="w-1.5 h-1.5 rounded-full" :class="system.connected ? 'bg-emerald-400' : 'bg-red-400'"></span>
        <span>{{ system.connected ? 'R3P LINK' : 'DISCONNECTED' }}</span>
      </div>

      <!-- User Chip & Logout -->
      <div class="flex items-center gap-1.5 pl-1.5 border-l border-white/10">
        <span class="mono text-[11px] text-gray-300 font-medium">root</span>
        <button
          class="p-1 rounded text-gray-400 hover:text-red-400 hover:bg-red-500/10 transition"
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
