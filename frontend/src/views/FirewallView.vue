<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'

interface ZoneItem {
  id: string
  name: string
  input: string
  output: string
  forward: string
  masq: boolean
  mtu_fix: boolean
}

interface RedirectItem {
  id: string
  name: string
  src: string
  src_dport: string
  dest: string
  dest_ip: string
  dest_port: string
  proto: string
  enabled: boolean
}

interface RuleItem {
  id: string
  name: string
  src: string
  dest: string
  proto: string
  dest_port: string
  target: string
  family: string
  enabled: boolean
}

const defaults = ref<Record<string, string>>({})
const zones = ref<ZoneItem[]>([])
const redirects = ref<RedirectItem[]>([])
const rules = ref<RuleItem[]>([])
const conntrackCount = ref(0)
const conntrackMax = ref(16384)
const loading = ref(false)
const notice = ref<string | null>(null)

const showAddRedirect = ref(false)
const newRedir = ref({ name: '', proto: 'tcp', src_dport: '', dest_ip: '192.168.2.100', dest_port: '' })

const showAddRule = ref(false)
const newRule = ref({ name: '', src: 'wan', proto: 'tcp', dest_port: '', target: 'ACCEPT' })

async function loadFirewall() {
  loading.value = true
  try {
    const res = await apiRequest<{
      defaults: Record<string, string>
      zones: ZoneItem[]
      redirects: RedirectItem[]
      rules: RuleItem[]
      conntrack_count: number
      conntrack_max: number
    }>('/firewall')
    if (res) {
      defaults.value = res.defaults || {}
      zones.value = res.zones || []
      redirects.value = res.redirects || []
      rules.value = res.rules || []
      conntrackCount.value = res.conntrack_count || 0
      conntrackMax.value = res.conntrack_max || 16384
    }
  } catch (err: any) {
    notice.value = 'Error loading firewall: ' + (err?.message || err)
  } finally {
    loading.value = false
  }
}

async function addRedirect() {
  if (!newRedir.value.name || !newRedir.value.src_dport || !newRedir.value.dest_ip) return
  try {
    await apiRequest('/firewall/redirect', {
      method: 'POST',
      body: JSON.stringify(newRedir.value),
    })
    showAddRedirect.value = false
    newRedir.value = { name: '', proto: 'tcp', src_dport: '', dest_ip: '192.168.2.100', dest_port: '' }
    notice.value = 'Port forward added and fw4 reloaded!'
    await loadFirewall()
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    notice.value = 'Error: ' + (err?.message || err)
  }
}

async function toggleRedirect(r: RedirectItem) {
  try {
    await apiRequest('/firewall/redirect', {
      method: 'PATCH',
      body: JSON.stringify({ id: r.id, enabled: !r.enabled }),
    })
    await loadFirewall()
  } catch (err: any) {
    notice.value = 'Error: ' + (err?.message || err)
  }
}

async function deleteRedirect(id: string) {
  try {
    await apiRequest(`/firewall/redirect?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
    notice.value = 'Port forward deleted!'
    await loadFirewall()
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    notice.value = 'Error: ' + (err?.message || err)
  }
}

async function addRule() {
  if (!newRule.value.name) return
  try {
    await apiRequest('/firewall/rule', {
      method: 'POST',
      body: JSON.stringify(newRule.value),
    })
    showAddRule.value = false
    newRule.value = { name: '', src: 'wan', proto: 'tcp', dest_port: '', target: 'ACCEPT' }
    notice.value = 'Firewall rule added and fw4 reloaded!'
    await loadFirewall()
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    notice.value = 'Error: ' + (err?.message || err)
  }
}

async function toggleRule(r: RuleItem) {
  try {
    await apiRequest('/firewall/rule', {
      method: 'PATCH',
      body: JSON.stringify({ id: r.id, enabled: !r.enabled }),
    })
    await loadFirewall()
  } catch (err: any) {
    notice.value = 'Error: ' + (err?.message || err)
  }
}

async function deleteRule(id: string) {
  try {
    await apiRequest(`/firewall/rule?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
    notice.value = 'Traffic rule deleted!'
    await loadFirewall()
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    notice.value = 'Error: ' + (err?.message || err)
  }
}

onMounted(() => {
  void loadFirewall()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Firewall Overview Stats -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-2">
      <div class="stat-mini">
        <div class="text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">Firewall Engine</div>
        <div class="mono text-sm font-bold text-sky-400">nftables (fw4)</div>
        <div class="text-[10px] text-gray-500">Live UCI /etc/config/firewall</div>
      </div>
      <div class="stat-mini">
        <div class="text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">SYN Flood Guard</div>
        <div class="mono text-sm font-bold text-emerald-400">{{ defaults.syn_flood === '1' ? 'ENABLED' : 'DISABLED' }}</div>
        <div class="text-[10px] text-gray-500">Default Input: {{ defaults.input || 'REJECT' }}</div>
      </div>
      <div class="stat-mini">
        <div class="text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">Active Conntrack</div>
        <div class="mono text-sm font-bold text-purple-400">{{ conntrackCount }} / {{ conntrackMax }}</div>
        <div class="text-[10px] text-gray-500">nf_conntrack sessions</div>
      </div>
      <div class="stat-mini">
        <div class="text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">Rules & Redirects</div>
        <div class="mono text-sm font-bold text-amber-400">{{ rules.length }} Rules · {{ redirects.length }} DNAT</div>
        <div class="text-[10px] text-gray-500">{{ zones.length }} Active Zones</div>
      </div>
    </div>

    <div v-if="notice" class="px-3 py-1.5 rounded bg-sky-500/15 border border-sky-500/30 text-sky-300 text-[11.5px] font-medium">
      {{ notice }}
    </div>

    <!-- Zones Overview -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Firewall Security Zones (UCI)</span>
        <button class="btn-secondary text-[11px] py-1" :disabled="loading" @click="loadFirewall">Refresh ↻</button>
      </div>
      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Zone Name</th>
              <th>Input Policy</th>
              <th>Output Policy</th>
              <th>Forward Policy</th>
              <th>Masquerading (NAT)</th>
              <th>MSS Clamping</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="z in zones" :key="z.id">
              <td class="font-semibold text-gray-200 uppercase mono">{{ z.name }}</td>
              <td><span :class="z.input === 'ACCEPT' ? 'badge badge-green' : 'badge badge-red'">{{ z.input }}</span></td>
              <td><span class="badge badge-green">{{ z.output }}</span></td>
              <td><span :class="z.forward === 'ACCEPT' ? 'badge badge-green' : 'badge badge-red'">{{ z.forward }}</span></td>
              <td><span :class="z.masq ? 'badge badge-blue' : 'badge badge-gray'">{{ z.masq ? 'ENABLED (SNAT)' : 'OFF' }}</span></td>
              <td><span :class="z.mtu_fix ? 'badge badge-purple' : 'badge badge-gray'">{{ z.mtu_fix ? 'MTU FIX ON' : 'OFF' }}</span></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Port Forwarding Rules (DNAT) -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Port Forwarding / DNAT Redirections ({{ redirects.length }})</span>
        <button class="btn-primary text-[12px] py-1 px-3" @click="showAddRedirect = !showAddRedirect">
          {{ showAddRedirect ? 'Cancel' : '+ Add Port Forward' }}
        </button>
      </div>

      <!-- Add Redirect Form -->
      <form v-if="showAddRedirect" class="mb-3 p-3 rounded-lg bg-black/40 border border-sky-500/30 grid grid-cols-1 sm:grid-cols-6 gap-2.5 items-end text-xs" @submit.prevent="addRedirect">
        <div>
          <label class="block text-[12px] text-gray-300 font-semibold uppercase mb-1">Rule Name</label>
          <input v-model="newRedir.name" type="text" required class="w-full pro-input" placeholder="e.g. Web-Server" />
        </div>
        <div>
          <label class="block text-[12px] text-gray-300 font-semibold uppercase mb-1">Protocol</label>
          <select v-model="newRedir.proto" class="w-full pro-input mono">
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
            <option value="tcp udp">TCP + UDP</option>
          </select>
        </div>
        <div>
          <label class="block text-[12px] text-gray-300 font-semibold uppercase mb-1">WAN Port</label>
          <input v-model="newRedir.src_dport" type="number" min="1" max="65535" required class="w-full pro-input mono" placeholder="8080" />
        </div>
        <div>
          <label class="block text-[12px] text-gray-300 font-semibold uppercase mb-1">LAN Target IP</label>
          <input v-model="newRedir.dest_ip" type="text" required class="w-full pro-input mono" placeholder="192.168.2.100" />
        </div>
        <div>
          <label class="block text-[12px] text-gray-300 font-semibold uppercase mb-1">LAN Port</label>
          <input v-model="newRedir.dest_port" type="number" min="1" max="65535" class="w-full pro-input mono" placeholder="80" />
        </div>
        <button type="submit" class="btn-primary justify-center py-2 text-[12.5px] font-bold">Save DNAT</button>
      </form>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Protocol</th>
              <th>WAN Port</th>
              <th>Internal Target</th>
              <th>State</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!redirects.length">
              <td colspan="6" class="text-center text-gray-400 py-4 font-medium">
                No port forwarding rules configured. Click "+ Add Port Forward" to expose an internal service.
              </td>
            </tr>
            <tr v-for="pf in redirects" :key="pf.id">
              <td class="font-semibold text-gray-100 text-[13.5px]">{{ pf.name || pf.id }}</td>
              <td><span class="badge badge-blue mono uppercase text-[11px] font-bold">{{ pf.proto }}</span></td>
              <td class="mono font-bold text-sky-300 text-[13.5px]">{{ pf.src_dport }}</td>
              <td class="mono text-gray-200 text-[13.5px]">{{ pf.dest_ip }}:{{ pf.dest_port || pf.src_dport }}</td>
              <td>
                <span :class="pf.enabled ? 'badge badge-green' : 'badge badge-gray'">
                  {{ pf.enabled ? 'ACTIVE' : 'DISABLED' }}
                </span>
              </td>
              <td>
                <div class="flex items-center gap-2">
                  <button class="btn-secondary text-[11.5px] py-1 px-2.5 font-semibold cursor-pointer" @click="toggleRedirect(pf)">
                    {{ pf.enabled ? 'Disable' : 'Enable' }}
                  </button>
                  <button class="btn-danger text-[11.5px] py-1 px-2.5 font-semibold cursor-pointer" @click="deleteRedirect(pf.id)">
                    Delete
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Traffic Rules Table -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Firewall Traffic Rules ({{ rules.length }})</span>
        <button class="btn-primary text-[11px]" @click="showAddRule = !showAddRule">
          {{ showAddRule ? 'Cancel' : '+ Add Traffic Rule' }}
        </button>
      </div>

      <!-- Add Traffic Rule Form -->
      <form v-if="showAddRule" class="mb-3 p-2.5 rounded bg-black/40 border border-sky-500/30 grid grid-cols-1 sm:grid-cols-6 gap-2 items-end" @submit.prevent="addRule">
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Rule Name</label>
          <input v-model="newRule.name" type="text" required class="w-full pro-input" placeholder="e.g. Allow-WireGuard" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Source Zone</label>
          <select v-model="newRule.src" class="w-full pro-input mono">
            <option value="wan">WAN</option>
            <option value="lan">LAN</option>
          </select>
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Protocol</label>
          <select v-model="newRule.proto" class="w-full pro-input mono">
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
            <option value="icmp">ICMP</option>
          </select>
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Dest Port</label>
          <input v-model="newRule.dest_port" type="text" class="w-full pro-input mono" placeholder="51820" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Action Target</label>
          <select v-model="newRule.target" class="w-full pro-input mono">
            <option value="ACCEPT">ACCEPT</option>
            <option value="DROP">DROP</option>
            <option value="REJECT">REJECT</option>
          </select>
        </div>
        <button type="submit" class="btn-primary justify-center py-1.5">Save Rule</button>
      </form>

      <div class="overflow-x-auto max-h-64">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Rule Name</th>
              <th>Source</th>
              <th>Protocol</th>
              <th>Port</th>
              <th>Target</th>
              <th>State</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in rules" :key="r.id">
              <td class="font-medium text-gray-200">{{ r.name || r.id }}</td>
              <td class="mono uppercase text-gray-300">{{ r.src || 'any' }}</td>
              <td class="mono uppercase text-sky-300">{{ r.proto || 'all' }}</td>
              <td class="mono text-gray-300">{{ r.dest_port || 'any' }}</td>
              <td>
                <span :class="r.target === 'ACCEPT' ? 'badge badge-green' : 'badge badge-red'">
                  {{ r.target }}
                </span>
              </td>
              <td>
                <span :class="r.enabled ? 'badge badge-blue' : 'badge badge-gray'">
                  {{ r.enabled ? 'ENABLED' : 'OFF' }}
                </span>
              </td>
              <td>
                <div class="flex items-center gap-1.5">
                  <button class="btn-secondary text-[10px] py-0.5 px-2" @click="toggleRule(r)">
                    {{ r.enabled ? 'Disable' : 'Enable' }}
                  </button>
                  <button class="btn-danger text-[10px] py-0.5 px-2" @click="deleteRule(r.id)">
                    Delete
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
