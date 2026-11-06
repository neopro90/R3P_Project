<script setup lang="ts">
import { computed } from 'vue'
import { useNetworkStore } from '@/stores/network'

const network = useNetworkStore()
const activeClients = computed(() => network.clients.filter((client) => client.leaseActive))
const wan = computed(() => network.interfaces.find((iface) => iface.type === 'wan'))

function clientPosition(index: number) {
  const count = Math.min(activeClients.value.length, 6)
  const start = 80
  const end = 920
  return count <= 1 ? 500 : start + (index * (end - start)) / (count - 1)
}
</script>

<template>
  <section class="glass-card p-5 space-y-4">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="text-sm font-semibold text-white">Network topology</h2>
        <p class="mt-1 text-xs text-gray-400">Live interface data and DHCP leases; lease activity is not a Wi-Fi association.</p>
      </div>
      <span class="text-xs text-gray-400">
        {{ network.clientsAvailable ? `${activeClients.length} active leases` : 'Client data unavailable' }}
      </span>
    </div>

    <div class="topology-canvas">
      <svg viewBox="0 0 1000 300" role="img" aria-labelledby="topology-title topology-description">
        <title id="topology-title">Router network topology</title>
        <desc id="topology-description">
          Internet connects to the router. Active DHCP leases are shown below without assuming their connection type.
        </desc>
        <defs>
          <linearGradient id="topology-link" x1="0" x2="1">
            <stop offset="0%" stop-color="var(--accent-primary)" stop-opacity=".2" />
            <stop offset="100%" stop-color="var(--accent-primary)" stop-opacity=".85" />
          </linearGradient>
        </defs>
        <path d="M500 55v48M500 160v45M500 205H80M500 205H920" class="topology-line" />
        <g v-if="activeClients.length">
          <path
            v-for="(_, index) in activeClients.slice(0, 6)"
            :key="`edge-${index}`"
            :d="`M${clientPosition(index)} 205v22`"
            class="topology-line"
          />
        </g>
        <g class="topology-node">
          <rect x="420" y="12" width="160" height="44" rx="14" />
          <text x="500" y="39" text-anchor="middle">Internet / WAN</text>
          <text x="500" y="74" text-anchor="middle" class="topology-caption">
            {{ wan?.ipv4 || (network.interfaces.length ? 'WAN IP unavailable' : 'Interface data unavailable') }}
          </text>
        </g>
        <g class="topology-node topology-node-router">
          <rect x="410" y="103" width="180" height="57" rx="16" />
          <text x="500" y="128" text-anchor="middle">R3P Gateway</text>
          <text x="500" y="146" text-anchor="middle" class="topology-caption">MT7621</text>
        </g>
        <text x="500" y="193" text-anchor="middle" class="topology-caption">DHCP leases</text>
        <g v-for="(client, index) in activeClients.slice(0, 6)" :key="client.mac" class="topology-node topology-client">
          <rect :x="clientPosition(index) - 65" y="229" width="130" height="54" rx="13" />
          <text :x="clientPosition(index)" y="251" text-anchor="middle">{{ client.hostname || client.ip }}</text>
          <text :x="clientPosition(index)" y="269" text-anchor="middle" class="topology-caption">{{ client.ip }}</text>
        </g>
      </svg>
    </div>
    <p v-if="!network.clientsAvailable || activeClients.length === 0" class="text-xs text-gray-500">
      {{ network.clientsAvailable ? 'No active DHCP leases to map.' : 'Topology is partial until router client data is available.' }}
    </p>
    <p v-else-if="activeClients.length > 6" class="text-xs text-gray-500">
      Showing 6 of {{ activeClients.length }} active leases. Open DHCP Leases for the full list.
    </p>
  </section>
</template>
