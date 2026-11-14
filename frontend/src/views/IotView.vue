<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '@/services/api'
import { useUiPreferences } from '@/composables/useUiPreferences'

const { t } = useUiPreferences()

interface IoTDevice {
  id: string
  name: string
  ip: string
  mac: string
  protocol: string
  topic: string
  online: boolean
  latency: string
  last_seen: string
}

interface IoTBroker {
  host: string
  port: number
  username: string
  topic_prefix: string
  enabled: boolean
}

const devices = ref<IoTDevice[]>([])
const broker = ref<IoTBroker>({
  host: '192.168.2.1',
  port: 1883,
  username: '',
  topic_prefix: 'r3p/home',
  enabled: true,
})
const brokerAlive = ref(false)
const loading = ref(false)
const probing = ref(false)
const notice = ref<string | null>(null)
let noticeTimer: ReturnType<typeof setTimeout> | null = null
function showNotice(msg: string, ms = 4000) {
  notice.value = msg
  if (noticeTimer) clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => { notice.value = null }, ms)
}

const showAddDevice = ref(false)
const newDev = ref({
  name: '',
  ip: '192.168.2.',
  mac: '',
  protocol: 'MQTT / ICMP',
  topic: '',
})

async function loadIoT() {
  loading.value = true
  try {
    const res = await apiRequest<{ devices: IoTDevice[]; broker: IoTBroker; broker_alive?: boolean }>('/iot')
    if (res) {
      devices.value = res.devices || []
      if (res.broker) broker.value = res.broker
      brokerAlive.value = Boolean(res.broker_alive)
    }
  } catch (err: any) {
    showNotice('Error loading IoT: ' + (err?.message || err), 5000)
  } finally {
    loading.value = false
  }
}

async function addDevice() {
  if (!newDev.value.name || !newDev.value.ip) return
  try {
    const res = await apiRequest<{ devices: IoTDevice[]; broker_alive?: boolean }>('/iot', {
      method: 'POST',
      body: JSON.stringify(newDev.value),
    })
    if (res?.devices) devices.value = res.devices
    if (res?.broker_alive !== undefined) brokerAlive.value = Boolean(res.broker_alive)
    showAddDevice.value = false
    newDev.value = { name: '', ip: '192.168.2.', mac: '', protocol: 'MQTT / ICMP', topic: '' }
    showNotice('IoT device registered and probed!')
  } catch (err: any) {
    showNotice('Add device failed: ' + (err?.message || err), 5000)
  }
}

async function probeAllDevices() {
  probing.value = true
  showNotice('Probing all registered IoT endpoints via ICMP…', 10000)
  try {
    const res = await apiRequest<{ devices: IoTDevice[]; broker_alive?: boolean }>('/iot/ping', { method: 'POST' })
    if (res?.devices) devices.value = res.devices
    if (res?.broker_alive !== undefined) brokerAlive.value = Boolean(res.broker_alive)
    showNotice('Reachability probe completed!')
  } catch (err: any) {
    showNotice('Probe failed: ' + (err?.message || err), 5000)
  } finally {
    probing.value = false
  }
}

async function deleteDevice(id: string) {
  try {
    const res = await apiRequest<{ devices: IoTDevice[]; broker_alive?: boolean }>(`/iot?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
    if (res?.devices) devices.value = res.devices
    if (res?.broker_alive !== undefined) brokerAlive.value = Boolean(res.broker_alive)
    showNotice('Device removed from registry!')
  } catch (err: any) {
    showNotice('Delete failed: ' + (err?.message || err), 5000)
  }
}

async function saveBroker() {
  try {
    const res = await apiRequest<{ broker?: IoTBroker; broker_alive?: boolean }>('/iot', {
      method: 'PUT',
      body: JSON.stringify(broker.value),
    })
    if (res?.broker) broker.value = res.broker
    if (res?.broker_alive !== undefined) brokerAlive.value = Boolean(res.broker_alive)
    showNotice('MQTT Bridge configuration saved!')
  } catch (err: any) {
    showNotice('Broker save error: ' + (err?.message || err), 5000)
  }
}

onMounted(() => {
  void loadIoT()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-3 space-y-3 bg-[#080b11]">
    <!-- Floating Toast Notification (Never shifts layout) -->
    <div v-if="notice" class="r3p-toast r3p-toast-info">
      <div class="flex items-start justify-between gap-3">
        <span class="r3p-toast-text">{{ notice }}</span>
        <button type="button" class="text-sky-300 hover:text-white shrink-0 font-bold" @click="notice = null">✕</button>
      </div>
    </div>

    <!-- Top Stats -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-2">
      <div class="stat-mini">
        <div class="text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">{{ t('iot_mqtt_bridge') }}</div>
        <div class="mono text-sm font-bold truncate" :class="brokerAlive ? 'text-emerald-400' : (broker.enabled ? 'text-amber-400' : 'text-gray-400')">
          {{ broker.enabled ? (brokerAlive ? 'ALIVE (TCP/Broker)' : 'CONFIGURED (NO BROKER)') : 'DISABLED' }}
        </div>
        <div class="text-[10px] text-gray-500 truncate">{{ broker.host }}:{{ broker.port }}</div>
      </div>
      <div class="stat-mini">
        <div class="text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">{{ t('iot_topic_ns') }}</div>
        <div class="mono text-sm font-bold text-sky-400 truncate">{{ broker.topic_prefix }}/#</div>
        <div class="text-[10px] text-gray-500 truncate">Home Assistant / MQTT</div>
      </div>
      <div class="stat-mini">
        <div class="text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">Registered Devices</div>
        <div class="mono text-sm font-bold text-purple-400 truncate">{{ devices.length }} Endpoints</div>
        <div class="text-[10px] text-gray-500 truncate">{{ devices.filter(d => d.online).length }} Online</div>
      </div>
      <div class="stat-mini">
        <div class="text-[10.5px] text-gray-400 uppercase tracking-wider mb-1">Health Probe</div>
        <button type="button" class="btn-primary text-[11px] w-full justify-center py-1 mt-0.5" :disabled="probing" @click="probeAllDevices">
          {{ probing ? 'Pinging All…' : '⚡ Probe All Devices' }}
        </button>
      </div>
    </div>

    <!-- Connected IoT Device Registry -->
    <div class="pro-card">
      <div class="pro-card-header">
        <span class="pro-card-title">IoT & Smart Device Registry ({{ devices.length }})</span>
        <div class="flex items-center gap-2">
          <button type="button" class="btn-secondary text-[11px] py-1 min-w-[92px] justify-center" :disabled="loading" @click="loadIoT">
            {{ loading ? 'Loading…' : 'Refresh ↻' }}
          </button>
          <button type="button" class="btn-primary text-[11px] py-1 min-w-[132px] justify-center" @click="showAddDevice = !showAddDevice">
            {{ showAddDevice ? 'Cancel' : '+ Register Device' }}
          </button>
        </div>
      </div>

      <!-- Add Device Form -->
      <form v-if="showAddDevice" class="mb-3 p-2.5 rounded bg-black/40 border border-sky-500/30 grid grid-cols-1 sm:grid-cols-6 gap-2 items-end" @submit.prevent="addDevice">
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Device Name</label>
          <input v-model="newDev.name" type="text" required class="w-full pro-input" placeholder="R3P-Sensor-01" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">IPv4 Address</label>
          <input v-model="newDev.ip" type="text" required class="w-full pro-input mono" placeholder="192.168.2.150" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Protocol</label>
          <select v-model="newDev.protocol" class="w-full pro-input mono">
            <option value="MQTT / ICMP">MQTT / ICMP</option>
            <option value="HTTP / ICMP">HTTP / ICMP</option>
            <option value="ICMP">ICMP Only</option>
          </select>
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">MAC (Optional)</label>
          <input v-model="newDev.mac" type="text" class="w-full pro-input mono" placeholder="aa:bb:cc:11:22:33" />
        </div>
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">MQTT Topic</label>
          <input v-model="newDev.topic" type="text" class="w-full pro-input mono" placeholder="r3p/home/sensor1" />
        </div>
        <button type="submit" class="btn-primary justify-center py-1.5">Add & Probe</button>
      </form>

      <div class="overflow-x-auto">
        <table class="pro-table">
          <thead>
            <tr>
              <th>Device ID</th>
              <th>Name</th>
              <th>IPv4 Address</th>
              <th>MAC Address</th>
              <th>Protocol</th>
              <th>MQTT Topic / Endpoint</th>
              <th>Status</th>
              <th>Latency</th>
              <th>Last Seen</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!devices.length">
              <td colspan="10" class="text-center text-gray-500 py-4">
                No IoT devices registered. Click "+ Register Device" to add an endpoint.
              </td>
            </tr>
            <tr v-for="d in devices" :key="d.id">
              <td class="mono font-semibold text-sky-400">{{ d.id }}</td>
              <td class="font-medium text-gray-200 max-w-[160px] truncate" :title="d.name">{{ d.name }}</td>
              <td class="mono text-gray-300">{{ d.ip }}</td>
              <td class="mono text-gray-400">{{ d.mac || '—' }}</td>
              <td><span class="badge badge-blue text-[10px] mono">{{ d.protocol || 'MQTT / ICMP' }}</span></td>
              <td class="mono text-purple-400 max-w-[180px] truncate" :title="d.topic">{{ d.topic }}</td>
              <td>
                <span :class="d.online ? 'badge badge-green' : 'badge badge-red'">
                  {{ d.online ? 'ONLINE' : 'OFFLINE' }}
                </span>
              </td>
              <td class="mono text-emerald-400">{{ d.latency }}</td>
              <td class="mono text-gray-400">{{ d.last_seen }}</td>
              <td>
                <button type="button" class="btn-danger text-[10px] py-0.5 px-2 min-w-[64px] justify-center" @click="deleteDevice(d.id)">
                  Remove
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- MQTT Broker Configuration -->
    <form class="pro-card" @submit.prevent="saveBroker">
      <div class="pro-card-header">
        <span class="pro-card-title">MQTT Broker / Home Assistant Bridge Settings</span>
        <label class="flex items-center gap-2 text-xs text-gray-300 cursor-pointer select-none">
          <input v-model="broker.enabled" type="checkbox" class="accent-sky-500" />
          <span>Bridge Enabled</span>
        </label>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-5 gap-3 items-end text-[12px]">
        <div>
          <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Broker Host / IP</label>
          <input v-model="broker.host" type="text" required class="w-full pro-input mono" />
        </div>
        <div>
          <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Broker Port</label>
          <input v-model.number="broker.port" type="number" min="1" max="65535" required class="w-full pro-input mono" />
        </div>
        <div>
          <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Broker Username</label>
          <input v-model="broker.username" type="text" class="w-full pro-input mono" placeholder="optional" />
        </div>
        <div>
          <label class="block text-[10.5px] text-gray-400 uppercase mb-1">Topic Prefix</label>
          <input v-model="broker.topic_prefix" type="text" required class="w-full pro-input mono" />
        </div>
        <button type="submit" class="btn-primary justify-center py-1.5">Save Bridge Settings</button>
      </div>
    </form>
  </div>
</template>
