<script setup lang="ts">
import { useUiPreferences, type AccentColor, type ThemeMode } from '@/composables/useUiPreferences'

const preferences = useUiPreferences()

const themes: Array<{ value: ThemeMode; label: string; detail: string }> = [
  { value: 'dark', label: 'Dark', detail: 'Dark glass surfaces' },
  { value: 'light', label: 'Light', detail: 'Higher contrast surfaces' },
  { value: 'system', label: 'System', detail: 'Follow this browser device' },
]
const accents: Array<{ value: AccentColor; label: string; color: string }> = [
  { value: 'blue', label: 'Sky', color: '#63b3ed' },
  { value: 'teal', label: 'Teal', color: '#38b2ac' },
  { value: 'purple', label: 'Violet', color: '#9f7aea' },
  { value: 'green', label: 'Green', color: '#68d391' },
  { value: 'orange', label: 'Amber', color: '#ed8936' },
]
</script>

<template>
  <section class="h-full overflow-y-auto p-5 md:p-6 space-y-6">
    <header>
      <h2 class="text-xl font-semibold text-white">Personalization & platform</h2>
      <p class="mt-1 text-sm text-gray-400">Appearance is stored in this browser. Hardware capabilities are reported conservatively.</p>
    </header>

    <section class="glass-card p-5 space-y-5">
      <div>
        <h3 class="font-semibold text-white">Appearance</h3>
        <p class="mt-1 text-xs text-gray-500">The dashboard can use dark, light, or system appearance and a custom accent.</p>
      </div>
      <fieldset class="grid gap-3 md:grid-cols-3">
        <legend class="sr-only">Theme</legend>
        <label
          v-for="theme in themes"
          :key="theme.value"
          class="preference-option"
          :class="{ 'preference-option-selected': preferences.state.theme === theme.value }"
        >
          <input v-model="preferences.state.theme" type="radio" name="theme" :value="theme.value" @change="preferences.applyTheme" />
          <span>
            <span class="block text-sm font-medium text-white">{{ theme.label }}</span>
            <span class="mt-1 block text-xs text-gray-400">{{ theme.detail }}</span>
          </span>
        </label>
      </fieldset>
      <fieldset class="flex flex-wrap gap-3">
        <legend class="mb-3 w-full text-sm text-gray-300">Accent color</legend>
        <label v-for="accent in accents" :key="accent.value" class="accent-option">
          <input v-model="preferences.state.accent" type="radio" name="accent" :value="accent.value" @change="preferences.applyTheme" />
          <span class="accent-swatch" :style="{ backgroundColor: accent.color }"></span>
          <span class="text-xs text-gray-300">{{ accent.label }}</span>
        </label>
      </fieldset>
    </section>

    <section class="glass-card p-5 space-y-4">
      <div>
        <h3 class="font-semibold text-white">Mini-server readiness</h3>
        <p class="mt-1 text-xs text-gray-500">Presence in a build is not proof of active hardware configuration.</p>
      </div>
      <ul class="divide-y divide-white/5">
        <li class="readiness-row">
          <div>
            <p class="text-sm text-white">Memory and cache telemetry</p>
            <p class="text-xs text-gray-500">MemAvailable, buffers, cached pages, and swap are available from the system API.</p>
          </div>
          <span class="status-pill status-pill-online">API available</span>
        </li>
        <li class="readiness-row">
          <div>
            <p class="text-sm text-white">zram swap</p>
            <p class="text-xs text-gray-500">No zram package or active zram configuration is included in the current custom image.</p>
          </div>
          <span class="status-pill status-pill-muted">Not configured</span>
        </li>
        <li class="readiness-row">
          <div>
            <p class="text-sm text-white">Hardware flow offloading (PPE)</p>
            <p class="text-xs text-gray-500">The nftables offload module is packaged; active firewall settings and hardware acceleration need target verification.</p>
          </div>
          <span class="status-pill status-pill-muted">Runtime unverified</span>
        </li>
        <li class="readiness-row">
          <div>
            <p class="text-sm text-white">USB 3.0 interference shield</p>
            <p class="text-xs text-gray-500">Automatic radio/channel mitigation is not enabled. It requires measurement and device-specific validation first.</p>
          </div>
          <span class="status-pill status-pill-muted">Not implemented</span>
        </li>
        <li class="readiness-row">
          <div>
            <p class="text-sm text-white">Smart-home hub</p>
            <p class="text-xs text-gray-500">Home Assistant / MQTT packages and service integration are not part of this firmware profile.</p>
          </div>
          <span class="status-pill status-pill-muted">Catalog planned</span>
        </li>
      </ul>
    </section>

    <p class="text-xs text-amber-300">
      Hardware acceleration, radio interference mitigation, zram, and smart-home services must be measured and tested on the target before being enabled.
    </p>
  </section>
</template>
