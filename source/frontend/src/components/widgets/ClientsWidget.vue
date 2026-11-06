<script setup lang="ts">
import { useNetworkStore } from '@/stores/network'
import StatusBadge from '@/components/ui/StatusBadge.vue'

const store = useNetworkStore()

function ifaceIcon(iface: string) {
  if (iface === 'wlan0' || iface === 'wlan1') return '📶'
  if (iface.startsWith('br-lan') || iface === 'eth1') return '🔌'
  return '💻'
}
</script>

<template>
  <div class="glass-card p-5 flex flex-col gap-4">
    <div class="flex items-center justify-between">
      <p class="text-xs font-semibold uppercase tracking-widest" style="color: rgba(255,255,255,0.4);">Active DHCP Leases</p>
      <span
        class="text-xs font-bold px-2 py-0.5 rounded-full"
        style="background: rgba(99,179,237,0.15); color: var(--accent-blue);"
      >
        {{ store.clientsAvailable ? `${store.clientCount} active` : '—' }}
      </span>
    </div>

    <ul v-if="store.clientsAvailable && store.clients.length > 0" class="space-y-2">
      <li
        v-for="client in store.clients.slice(0, 5)"
        :key="client.mac"
        class="flex items-center gap-3 py-1"
      >
        <span class="text-base">{{ ifaceIcon(client.interface) }}</span>
        <div class="flex-1 min-w-0">
          <p class="text-sm font-medium truncate text-white">{{ client.hostname || client.ip }}</p>
          <p class="text-xs truncate" style="color: rgba(255,255,255,0.35);">{{ client.ip }} · DHCP lease</p>
        </div>
        <StatusBadge :status="client.leaseActive ? 'online' : 'offline'" :label="client.leaseActive ? 'ACTIVE' : 'EXPIRED'" />
      </li>
    </ul>

    <div v-else class="text-center py-4 text-sm" style="color: rgba(255,255,255,0.3);">
      {{ store.clientsAvailable ? 'No active DHCP leases' : 'DHCP lease data unavailable' }}
    </div>
  </div>
</template>
