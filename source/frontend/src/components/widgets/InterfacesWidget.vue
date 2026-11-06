<script setup lang="ts">
import { useNetworkStore } from '@/stores/network'

const network = useNetworkStore()
</script>

<template>
  <section class="glass-card p-5 space-y-4">
    <div class="flex items-center justify-between gap-3">
      <h2 class="text-sm font-semibold text-white">Interface health</h2>
      <span class="text-xs text-gray-500">{{ network.interfaces.length || '—' }} reported</span>
    </div>
    <p v-if="!network.interfaces.length" class="text-sm text-gray-500">Interface telemetry unavailable.</p>
    <ul v-else class="grid gap-3 sm:grid-cols-2">
      <li v-for="iface in network.interfaces" :key="iface.name" class="interface-row">
        <div class="min-w-0">
          <p class="truncate text-sm font-medium text-white">{{ iface.name }}</p>
          <p class="mt-1 truncate text-xs text-gray-500">{{ iface.ipv4 || iface.type.toUpperCase() }}</p>
        </div>
        <span class="status-pill" :class="iface.up === undefined ? 'status-pill-muted' : iface.up ? 'status-pill-online' : 'status-pill-muted'">
          {{ iface.up === undefined ? 'Unknown' : iface.up ? 'Up' : 'Down' }}
        </span>
      </li>
    </ul>
  </section>
</template>
