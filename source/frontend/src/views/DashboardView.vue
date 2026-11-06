<script setup lang="ts">
import { computed, ref, type Component } from 'vue'
import CpuWidget from '@/components/widgets/CpuWidget.vue'
import MemWidget from '@/components/widgets/MemWidget.vue'
import NetworkWidget from '@/components/widgets/NetworkWidget.vue'
import ClientsWidget from '@/components/widgets/ClientsWidget.vue'
import TopologyWidget from '@/components/widgets/TopologyWidget.vue'
import InterfacesWidget from '@/components/widgets/InterfacesWidget.vue'
import ModulePipeline from '@/components/widgets/ModulePipeline.vue'
import { useNetworkStore } from '@/stores/network'
import { useSystemStore } from '@/stores/system'
import {
  useUiPreferences,
  type DashboardWidgetId,
} from '@/composables/useUiPreferences'

const system = useSystemStore()
const network = useNetworkStore()
const preferences = useUiPreferences()
const editing = ref(false)

const widgetComponents: Record<DashboardWidgetId, Component> = {
  cpu: CpuWidget,
  memory: MemWidget,
  network: NetworkWidget,
  clients: ClientsWidget,
  topology: TopologyWidget,
  interfaces: InterfacesWidget,
}

const widgetTitles: Record<DashboardWidgetId, string> = {
  cpu: 'CPU monitor',
  memory: 'Memory & swap',
  network: 'Network traffic',
  clients: 'DHCP leases',
  topology: 'Network topology',
  interfaces: 'Interface health',
}

const visibleWidgets = computed(() =>
  preferences.state.widgets.filter((id) => !preferences.state.hiddenWidgets.includes(id)),
)

function toggleWidget(id: DashboardWidgetId) {
  const hidden = preferences.state.hiddenWidgets
  preferences.state.hiddenWidgets = hidden.includes(id)
    ? hidden.filter((item) => item !== id)
    : [...hidden, id]
}

const errors = computed(() => [system.error, network.error].filter(Boolean))
</script>

<template>
  <section class="h-full overflow-y-auto p-5 md:p-6 space-y-5">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.2em] text-sky-300">R3P · mini-server</p>
        <h2 class="mt-1 text-xl font-semibold text-white">System overview</h2>
        <p class="mt-1 text-sm text-gray-400">
          {{ system.dataAvailable ? `${system.hostname} · ${system.firmwareVersion}` : 'Waiting for router telemetry' }}
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <RouterLink class="secondary-button" to="/apps">Services & apps</RouterLink>
        <button class="secondary-button" :aria-expanded="editing" @click="editing = !editing">
          {{ editing ? 'Done' : 'Customize dashboard' }}
        </button>
      </div>
    </header>

    <ModulePipeline />

    <div v-if="errors.length" role="status" class="glass-card border-amber-400/20 p-3 text-sm text-amber-200">
      Some router data is unavailable: {{ errors.join(' · ') }}
    </div>

    <section v-if="editing" class="glass-card p-4" aria-label="Dashboard layout settings">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 class="text-sm font-semibold text-white">Dashboard layout</h3>
          <p class="mt-1 text-xs text-gray-500">Order and visibility are saved in this browser.</p>
        </div>
        <button class="text-xs text-sky-300 hover:text-white" @click="preferences.reset">Reset layout</button>
      </div>
      <ol class="grid gap-2 md:grid-cols-2">
        <li
          v-for="(id, index) in preferences.state.widgets"
          :key="id"
          class="widget-preference-row"
        >
          <label class="flex min-w-0 flex-1 items-center gap-3">
            <input :checked="!preferences.state.hiddenWidgets.includes(id)" type="checkbox" @change="toggleWidget(id)" />
            <span class="truncate text-sm text-gray-200">{{ widgetTitles[id] }}</span>
          </label>
          <button
            class="icon-button"
            :disabled="index === 0"
            :aria-label="`Move ${widgetTitles[id]} up`"
            @click="preferences.moveWidget(id, -1)"
          >↑</button>
          <button
            class="icon-button"
            :disabled="index === preferences.state.widgets.length - 1"
            :aria-label="`Move ${widgetTitles[id]} down`"
            @click="preferences.moveWidget(id, 1)"
          >↓</button>
        </li>
      </ol>
    </section>

    <div v-if="visibleWidgets.length" class="dashboard-grid">
      <div
        v-for="id in visibleWidgets"
        :key="id"
        class="dashboard-widget"
        :class="id === 'topology' || id === 'interfaces' ? 'dashboard-widget-wide' : ''"
      >
        <component :is="widgetComponents[id]" />
      </div>
    </div>
    <div v-else class="glass-card p-8 text-center text-sm text-gray-400">
      All dashboard widgets are hidden. Use Customize dashboard to restore one.
    </div>
  </section>
</template>
