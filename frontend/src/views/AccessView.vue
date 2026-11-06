<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'

interface StaticLease {
  id: string
  name: string
  mac: string
  ip: string
}

interface ActiveLease {
  expiry: number
  mac: string
  ip: string
  hostname: string
  oui_vendor: string
}

const sshConfig = ref({
  port: '22',
  password_auth: true,
  root_password_auth: true,
})

const dhcpConfig = ref({
  start: '100',
  limit: '150',
  leasetime: '12h',
})

const staticLeases = ref<StaticLease[]>([])
const activeLeases = ref<ActiveLease[]>([])
const nowUnix = ref(Math.floor(Date.now() / 1000))
const loading = ref(false)
const notice = ref<string | null>(null)

const newPassword = ref('')
const confirmPassword = ref('')
const savingPassword = ref(false)

const showAddStatic = ref(false)
const newStatic = ref({ name: '', mac: '', ip: '192.168.2.' })

async function loadAccess() {
  loading.value = true
  try {
    const res = await apiRequest<{
      ssh: typeof sshConfig.value
      dhcp_lan: typeof dhcpConfig.value
      static_leases: StaticLease[]
      active_leases: ActiveLease[]
      now_unix: number
    }>('/access')
    if (res) {
      if (res.ssh) sshConfig.value = res.ssh
      if (res.dhcp_lan) {
        dhcpConfig.value = {
          start: res.dhcp_lan.start || '100',
          limit: res.dhcp_lan.limit || '150',
          leasetime: res.dhcp_lan.leasetime || '12h',
        }
      }
      staticLeases.value = res.static_leases || []
      activeLeases.value = res.active_leases || []
      nowUnix.value = res.now_unix || Math.floor(Date.now() / 1000)
    }
  } catch (err: any) {
    notice.value = 'Error loading access config: ' + (err?.message || err)
  } finally {
    loading.value = false
  }
}

async function saveSSH() {
  try {
    await apiRequest('/access/ssh', {
      method: 'PUT',
      body: JSON.stringify(sshConfig.value),
    })
    notice.value = 'Dropbear SSH configuration committed and service reloaded!'
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    notice.value = 'SSH save error: ' + (err?.message || err)
  }
}

async function saveDHCP() {
  try {
    await apiRequest('/access/dhcp', {
      method: 'PUT',
      body: JSON.stringify(dhcpConfig.value),
    })
    notice.value = 'DHCP server pool saved and dnsmasq reloaded!'
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    notice.value = 'DHCP save error: ' + (err?.message || err)
  }
}

async function updatePassword() {
  if (!newPassword.value || newPassword.value !== confirmPassword.value) {
    notice.value = 'Error: Passwords do not match!'
    return
  }
  savingPassword.value = true
  try {
    await apiRequest('/access/password', {
      method: 'POST',
      body: JSON.stringify({ password: newPassword.value }),
    })
    newPassword.value = ''
    confirmPassword.value = ''
    notice.value = 'Root & Web Admin password updated successfully!'
    setTimeout(() => { notice.value = null }, 3500)
  } catch (err: any) {
    notice.value = 'Password update error: ' + (err?.message || err)
  } finally {
    savingPassword.value = false
  }
}

async function addStaticLease(item?: { name: string; mac: string; ip: string }) {
  const payload = item || newStatic.value
  if (!payload.mac || !payload.ip) return
  try {
    await apiRequest('/access/static-lease', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
    showAddStatic.value = false
    newStatic.value = { name: '', mac: '', ip: '192.168.2.' }
    notice.value = `Static lease bound for ${payload.mac} -> ${payload.ip}!`
    await loadAccess()
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    notice.value = 'Static lease error: ' + (err?.message || err)
  }
}

async function deleteStaticLease(id: string) {
  try {
    await apiRequest(`/access/static-lease?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
    notice.value = 'Static lease removed!'
    await loadAccess()
    setTimeout(() => { notice.value = null }, 3000)
  } catch (err: any) {
    notice.value = 'Error: ' + (err?.message || err)
  }
}

function formatExpiry(exp: number): string {
  if (!exp) return 'Static / Infinite'
  const diff = exp - nowUnix.value
  if (diff <= 0) return 'Expired'
  const h = Math.floor(diff / 3600)
  const m = Math.floor((diff % 3600) / 60)
  return `${h}h ${m}m`
}

onMounted(() => {
  void loadAccess()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <div v-if="notice" class="px-3 py-1.5 rounded bg-sky-500/15 border border-sky-500/30 text-sky-300 text-[11.5px] font-medium">
      {{ notice }}
    </div>

    <!-- Top 3 Interactive Configuration Cards: SSH, DHCP, Password -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
      <!-- SSH Dropbear Config -->
      <form class="pro-card flex flex-col justify-between" @submit.prevent="saveSSH">
        <div>
          <div class="pro-card-header">
            <span class="pro-card-title">SSH Server (Dropbear)</span>
            <span class="badge badge-green">RUNNING</span>
          </div>
          <div class="space-y-2 text-[12px]">
            <div class="flex items-center justify-between">
              <span class="text-gray-400">Listen Port</span>
              <input v-model="sshConfig.port" type="number" min="1" max="65535" class="pro-input w-24 mono text-right" />
            </div>
            <label class="flex items-center justify-between py-1 cursor-pointer">
              <span class="text-gray-400">Password Authentication</span>
              <input v-model="sshConfig.password_auth" type="checkbox" class="accent-sky-400" />
            </label>
            <label class="flex items-center justify-between py-1 cursor-pointer">
              <span class="text-gray-400">Allow Root Login</span>
              <input v-model="sshConfig.root_password_auth" type="checkbox" class="accent-sky-400" />
            </label>
          </div>
        </div>
        <div class="mt-3 pt-2 border-t border-white/5 flex justify-end">
          <button type="submit" class="btn-primary text-[11px]">Save SSH Config</button>
        </div>
      </form>

      <!-- DHCP Server Pool Config -->
      <form class="pro-card flex flex-col justify-between" @submit.prevent="saveDHCP">
        <div>
          <div class="pro-card-header">
            <span class="pro-card-title">DHCP Server (dnsmasq)</span>
            <span class="badge badge-green">192.168.2.1/24</span>
          </div>
          <div class="space-y-2 text-[12px]">
            <div class="flex items-center justify-between">
              <span class="text-gray-400">Start Offset (.X)</span>
              <input v-model="dhcpConfig.start" type="number" min="2" max="250" class="pro-input w-24 mono text-right" />
            </div>
            <div class="flex items-center justify-between">
              <span class="text-gray-400">Max Leases Limit</span>
              <input v-model="dhcpConfig.limit" type="number" min="1" max="250" class="pro-input w-24 mono text-right" />
            </div>
            <div class="flex items-center justify-between">
              <span class="text-gray-400">Lease Duration</span>
              <input v-model="dhcpConfig.leasetime" type="text" class="pro-input w-24 mono text-right" placeholder="12h" />
            </div>
          </div>
        </div>
        <div class="mt-3 pt-2 border-t border-white/5 flex justify-end">
          <button type="submit" class="btn-primary text-[11px]">Save DHCP Pool</button>
        </div>
      </form>

      <!-- Admin Password Change -->
      <form class="pro-card flex flex-col justify-between" @submit.prevent="updatePassword">
        <div>
          <div class="pro-card-header">
            <span class="pro-card-title">Admin & Root Credentials</span>
            <span class="badge badge-blue">WEB + SSH</span>
          </div>
          <div class="space-y-2 text-[12px]">
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">New Password (root)</label>
              <input v-model="newPassword" type="password" required minlength="4" class="w-full pro-input mono" placeholder="Enter new password" />
            </div>
            <div>
              <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Confirm Password</label>
              <input v-model="confirmPassword" type="password" required minlength="4" class="w-full pro-input mono" placeholder="Repeat new password" />
            </div>
          </div>
        </div>
        <div class="mt-3 pt-2 border-t border-white/5 flex justify-end">
          <button type="submit" :disabled="savingPassword" class="btn-primary text-[11px]">
            {{ savingPassword ? 'Updating…' : 'Update Root Password' }}
          </button>
        </div>
      </form>
    </div>

    <!-- Active DHCP Leases -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Active DHCP Client Leases ({{ activeLeases.length }})</span>
        <button class="btn-secondary text-[11px] py-1" :disabled="loading" @click="loadAccess">Refresh Leases ↻</button>
      </div>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Hostname</th>
              <th>Assigned IPv4</th>
              <th>MAC Address</th>
              <th>Vendor OUI</th>
              <th>Remaining Lease</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!activeLeases.length">
              <td colspan="6" class="text-center text-gray-500 py-3">No active client leases found</td>
            </tr>
            <tr v-for="c in activeLeases" :key="c.mac">
              <td class="font-medium text-gray-200">{{ c.hostname || 'Unknown Device' }}</td>
              <td class="mono text-sky-300">{{ c.ip }}</td>
              <td class="mono text-gray-400">{{ c.mac }}</td>
              <td><span class="badge badge-gray">{{ c.oui_vendor || 'Generic' }}</span></td>
              <td class="mono text-emerald-400">{{ formatExpiry(c.expiry) }}</td>
              <td>
                <button
                  class="btn-secondary text-[10px] py-0.5 px-2"
                  @click="addStaticLease({ name: c.hostname || 'Client', mac: c.mac, ip: c.ip })"
                >
                  Pin Static IP
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Static DHCP Bindings -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">Static DHCP Reservations ({{ staticLeases.length }})</span>
        <button class="btn-primary text-[11px]" @click="showAddStatic = !showAddStatic">
          {{ showAddStatic ? 'Cancel' : '+ Add Static Lease' }}
        </button>
      </div>

      <form v-if="showAddStatic" class="mb-3 p-2.5 rounded bg-black/40 border border-sky-500/30 grid grid-cols-1 sm:grid-cols-4 gap-2 items-end" @submit.prevent="addStaticLease()">
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Hostname</label>
          <input v-model="newStatic.name" type="text" required class="w-full pro-input" placeholder="NAS-Box" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">MAC Address</label>
          <input v-model="newStatic.mac" type="text" required class="w-full pro-input mono" placeholder="aa:bb:cc:dd:ee:ff" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Reserved IPv4</label>
          <input v-model="newStatic.ip" type="text" required class="w-full pro-input mono" placeholder="192.168.2.50" />
        </div>
        <button type="submit" class="btn-primary justify-center py-1.5">Save Reservation</button>
      </form>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Hostname</th>
              <th>MAC Address</th>
              <th>Reserved IP</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!staticLeases.length">
              <td colspan="4" class="text-center text-gray-500 py-3">
                No static DHCP reservations configured. Click "Pin Static IP" on any active client or "+ Add Static Lease".
              </td>
            </tr>
            <tr v-for="s in staticLeases" :key="s.id">
              <td class="font-medium text-gray-200">{{ s.name }}</td>
              <td class="mono text-gray-400">{{ s.mac }}</td>
              <td class="mono text-sky-300">{{ s.ip }}</td>
              <td>
                <button class="btn-danger text-[10px] py-0.5 px-2" @click="deleteStaticLease(s.id)">
                  Remove
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
