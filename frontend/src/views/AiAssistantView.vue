<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { apiRequest } from '@/services/api'
import { useUiPreferences } from '@/composables/useUiPreferences'

const { t } = useUiPreferences()

interface AIConfig {
  provider: string
  api_key: string
  api_key_masked?: string
  has_api_key?: boolean
  base_url: string
  model: string
  allow_exec: boolean
  auto_execute: boolean
  telegram_ai_enabled: boolean
  auto_remember: boolean
  active_session_id: string
  system_note: string
}

interface AICmdLog {
  command: string
  output: string
  exit_code: number
  duration_ms: number
}

interface AIToolAction {
  tool: string
  command: string
  output: string
  success: boolean
}

interface AIChatMessage {
  role: 'user' | 'assistant'
  content: string
  timestamp: string
  source?: string
  commands?: AICmdLog[]
  tool_events?: AIToolAction[]
}

interface AIMemoryEntry {
  id: string
  category: string
  content: string
  created_at: string
}

interface AIChatSession {
  id: string
  title: string
  created_at: string
  updated_at: string
  messages: AIChatMessage[]
}

const loading = ref(false)
const sending = ref(false)
const savingConfig = ref(false)
const showConfig = ref(false)
const showApiKey = ref(false)
const feedback = ref('')
const errorMsg = ref('')

const config = ref<AIConfig>({
  provider: 'gemini',
  api_key: '',
  base_url: '',
  model: 'gemini-3.1-flash-lite',
  allow_exec: true,
  auto_execute: true,
  telegram_ai_enabled: true,
  auto_remember: true,
  active_session_id: 'default',
  system_note: '',
})

const sessions = ref<AIChatSession[]>([])
const activeSessionId = ref('default')
const history = ref<AIChatMessage[]>([])
const memories = ref<AIMemoryEntry[]>([])
const systemBlueprint = ref('')
const showBlueprintModal = ref(false)
const showNewMemoryModal = ref(false)
const newMemoryCategory = ref('preference')
const newMemoryContent = ref('')

const promptInput = ref('')
const chatContainer = ref<HTMLElement | null>(null)
const activeCommandTab = ref<'quick' | 'network' | 'storage' | 'adblock' | 'download' | 'maintenance'>('quick')

const providerModels: Record<string, string[]> = {
  gemini: ['gemini-3.1-flash-lite', 'gemini-3.1-flash-lite-preview', 'gemini-flash-latest', 'gemini-3-flash-preview'],
  openai: ['gpt-4o-mini', 'gpt-4o', 'o3-mini'],
  deepseek: ['deepseek-chat', 'deepseek-reasoner'],
  anthropic: ['claude-3-7-sonnet-latest', 'claude-3-5-haiku-latest'],
  groq: ['llama-3.3-70b-versatile', 'mixtral-8x7b-32768'],
  openrouter: ['google/gemini-2.5-flash', 'anthropic/claude-3.5-sonnet', 'deepseek/deepseek-chat'],
}

interface QuickCommandItem {
  id?: string
  label: string
  cmd: string
}

const defaultQuickCommands: Record<string, QuickCommandItem[]> = {
  quick: [
    { label: '🩺 Kiểm tra tổng quan Router', cmd: '!uname -a && uptime && df -h /overlay /mnt/sda1 && free -m' },
    { label: '📊 Tải CPU & Tiến trình top', cmd: '!top -b -n 1 | head -n 25' },
    { label: '🌐 IP các cổng mạng', cmd: '!ip -br addr && echo "---" && ip route' },
    { label: '📜 30 dòng log hệ thống gần nhất', cmd: '!logread | tail -n 30' },
  ],
  network: [
    { label: '📡 Quét Wi-Fi 2.4G & 5G xung quanh', cmd: '!iwinfo phy1 scan 2>/dev/null | head -n 35; iwinfo phy0 scan 2>/dev/null | head -n 35' },
    { label: '📶 Cấu hình Wi-Fi hiện tại (UCI)', cmd: '!uci show wireless' },
    { label: '👥 Danh sách thiết bị kết nối DHCP', cmd: '!cat /tmp/dhcp.leases' },
    { label: '⚡ Ping kiểm tra Internet (Google)', cmd: '!ping -c 4 8.8.8.8' },
    { label: '🔎 Tra cứu DNS qua nslookup', cmd: '!nslookup google.com 127.0.0.1' },
  ],
  storage: [
    { label: '💾 Chi tiết ổ đĩa USB 3.0 & Phân vùng', cmd: '!lsblk -o NAME,FSTYPE,SIZE,MOUNTPOINT,LABEL' },
    { label: '🚀 Tốc độ I/O ổ đĩa HDD 500GB', cmd: '!dd if=/dev/sda1 of=/dev/null bs=1M count=100 2>&1' },
    { label: '📁 Xem file trong ổ cứng USB', cmd: '!ls -lah /mnt/sda1' },
    { label: '⚡ Thông số tinh chỉnh USB Queue', cmd: '!cat /sys/block/sda/queue/read_ahead_kb; cat /sys/block/sda/queue/max_sectors_kb' },
  ],
  adblock: [
    { label: '🛡️ Trạng thái AdBlock & Số domain', cmd: '!cat /etc/r3p/adblock.json | grep total_blocked; wc -l /tmp/r3p-adblock.hosts' },
    { label: '🔍 Kiểm tra cổng DNS 53 listening', cmd: '!netstat -tlpn | grep 53' },
    { label: '🚫 Thử phân giải tên miền quảng cáo', cmd: '!nslookup pagead2.googlesyndication.com 127.0.0.1' },
  ],
  download: [
    { label: '⬇️ Phiên bản yt-dlp & aria2c', cmd: '!yt-dlp --version; aria2c --version | head -n 1' },
    { label: '📂 Thư mục & File video đã tải', cmd: '!grep outputDirectory /etc/r3p/downloader.json 2>/dev/null; ls -lah /mnt/sda1/downloads 2>/dev/null || ls -lah /overlay/downloads' },
    { label: '🤖 Kiểm tra dịch vụ r3p-api (80/8443)', cmd: '!netstat -tlpn | grep r3p-api' },
  ],
  maintenance: [
    { label: '🧹 Giải phóng RAM (Drop Caches)', cmd: '!sync && echo 3 > /proc/sys/vm/drop_caches && free -m' },
    { label: '🔄 Khởi động lại dịch vụ r3p-api', cmd: '!/etc/init.d/r3p-api restart' },
    { label: '📶 Khởi động lại Wi-Fi subsystem', cmd: '!wifi reload' },
    { label: '📦 Kiểm tra kho ứng dụng opkg', cmd: '!opkg list-installed | wc -l' },
  ],
}

const quickCommandGroups = ref<Record<string, QuickCommandItem[]>>(JSON.parse(JSON.stringify(defaultQuickCommands)))

// Quick command modal state
const showCommandModal = ref(false)
const isEditingCommand = ref(false)
const commandEditCategory = ref('quick')
const commandEditIndex = ref(-1)
const commandForm = ref({
  category: 'quick',
  label: '',
  cmd: '',
})

// Blueprint modal state
const blueprintEditMode = ref(false)
const savingBlueprint = ref(false)
const blueprintFeedback = ref('')
const blueprintFileInput = ref<HTMLInputElement | null>(null)

function onProviderChange() {
  const models = providerModels[config.value.provider]
  if (models && models.length > 0) {
    config.value.model = models[0] ?? ''
  }
}

async function scrollToBottom() {
  await nextTick()
  if (chatContainer.value) {
    chatContainer.value.scrollTop = chatContainer.value.scrollHeight
  }
}

async function loadState() {
  loading.value = true
  try {
    const res = await apiRequest<{
      config: AIConfig
      sessions: AIChatSession[]
      history: AIChatMessage[]
      memories: AIMemoryEntry[]
      active_session_id: string
    }>('/ai/config')
    if (res.config) {
      config.value = {
        ...res.config,
        api_key: '',
      }
    }
    sessions.value = res.sessions || []
    activeSessionId.value = res.active_session_id || 'default'
    history.value = res.history || []
    memories.value = res.memories || []
    await scrollToBottom()
  } catch (e) {
    errorMsg.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function switchSession(id: string) {
  loading.value = true
  try {
    const res = await apiRequest<{
      active_session_id: string
      sessions: AIChatSession[]
      history: AIChatMessage[]
    }>('/ai/sessions', {
      method: 'PUT',
      body: JSON.stringify({ session_id: id }),
    })
    activeSessionId.value = res.active_session_id
    sessions.value = res.sessions
    history.value = res.history
    await scrollToBottom()
  } catch (e) {
    errorMsg.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function createNewSession() {
  loading.value = true
  try {
    const res = await apiRequest<{
      active_session_id: string
      sessions: AIChatSession[]
      history: AIChatMessage[]
    }>('/ai/sessions', {
      method: 'POST',
      body: JSON.stringify({ title: `Phiên #${sessions.value.length + 1}` }),
    })
    activeSessionId.value = res.active_session_id
    sessions.value = res.sessions
    history.value = res.history
    await scrollToBottom()
  } catch (e) {
    errorMsg.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function deleteSession(id: string) {
  if (sessions.value.length <= 1) {
    errorMsg.value = 'Không thể xóa phiên duy nhất còn lại.'
    return
  }
  if (!confirm('Bạn có chắc chắn muốn xóa phiên trò chuyện này?')) return
  loading.value = true
  try {
    const res = await apiRequest<{
      active_session_id: string
      sessions: AIChatSession[]
      history: AIChatMessage[]
    }>(`/ai/sessions?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
    activeSessionId.value = res.active_session_id
    sessions.value = res.sessions
    history.value = res.history
    await scrollToBottom()
  } catch (e) {
    errorMsg.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function loadBlueprint() {
  try {
    blueprintFeedback.value = ''
    blueprintEditMode.value = false
    const res = await apiRequest<{ content: string }>('/ai/blueprint')
    systemBlueprint.value = res.content || ''
    showBlueprintModal.value = true
  } catch (e) {
    errorMsg.value = (e as Error).message
  }
}

async function saveBlueprint() {
  savingBlueprint.value = true
  blueprintFeedback.value = ''
  try {
    await apiRequest('/ai/blueprint', {
      method: 'PUT',
      body: JSON.stringify({ content: systemBlueprint.value }),
    })
    blueprintFeedback.value = 'Đã lưu bản thiết kế hệ thống thành công!'
    setTimeout(() => { blueprintFeedback.value = '' }, 4000)
  } catch (e) {
    blueprintFeedback.value = 'Lỗi lưu: ' + (e as Error).message
  } finally {
    savingBlueprint.value = false
  }
}

async function resetBlueprint() {
  if (!confirm('Khôi phục bản thiết kế hệ thống về nội dung mặc định của nhà sản xuất?')) return
  savingBlueprint.value = true
  blueprintFeedback.value = ''
  try {
    const res = await apiRequest<{ content: string }>('/ai/blueprint', {
      method: 'PUT',
      body: JSON.stringify({ reset: true }),
    })
    systemBlueprint.value = res.content || ''
    blueprintFeedback.value = 'Đã khôi phục bản thiết kế mặc định!'
    setTimeout(() => { blueprintFeedback.value = '' }, 4000)
  } catch (e) {
    blueprintFeedback.value = 'Lỗi khôi phục: ' + (e as Error).message
  } finally {
    savingBlueprint.value = false
  }
}

function downloadBlueprint() {
  const blob = new Blob([systemBlueprint.value], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'SYSTEM_BLUEPRINT.md'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

function triggerBlueprintUpload() {
  blueprintFileInput.value?.click()
}

function onBlueprintFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return

  const reader = new FileReader()
  reader.onload = (e) => {
    const text = (e.target?.result as string) || ''
    if (text.trim()) {
      systemBlueprint.value = text
      blueprintEditMode.value = true
      blueprintFeedback.value = `Đã nạp nội dung từ "${file.name}". Bấm "Lưu Thay Đổi" để áp dụng lên Router.`
    }
    input.value = ''
  }
  reader.readAsText(file)
}

// Quick Commands Handlers
function openAddCommandModal() {
  isEditingCommand.value = false
  commandEditIndex.value = -1
  commandForm.value = {
    category: activeCommandTab.value || 'quick',
    label: '',
    cmd: '',
  }
  showCommandModal.value = true
}

function openEditCommandModal(category: string, index: number) {
  isEditingCommand.value = true
  commandEditCategory.value = category
  commandEditIndex.value = index
  const item = quickCommandGroups.value[category]?.[index]
  if (item) {
    commandForm.value = {
      category,
      label: item.label,
      cmd: item.cmd,
    }
    showCommandModal.value = true
  }
}

async function saveCommandItem() {
  const label = commandForm.value.label.trim()
  const cmd = commandForm.value.cmd.trim()
  const cat = commandForm.value.category.trim() || 'quick'
  if (!label || !cmd) return

  if (!quickCommandGroups.value[cat]) {
    quickCommandGroups.value[cat] = []
  }

  if (isEditingCommand.value && commandEditIndex.value >= 0) {
    const oldCat = commandEditCategory.value
    if (oldCat !== cat) {
      quickCommandGroups.value[oldCat]?.splice(commandEditIndex.value, 1)
      quickCommandGroups.value[cat].push({ label, cmd })
    } else {
      quickCommandGroups.value[cat][commandEditIndex.value] = { label, cmd }
    }
  } else {
    quickCommandGroups.value[cat].push({ label, cmd })
  }

  activeCommandTab.value = cat as any
  showCommandModal.value = false
  await persistQuickCommands()
}

async function deleteCommandItem(category: string, index: number) {
  if (!confirm('Bạn có chắc muốn xóa lệnh này không?')) return
  quickCommandGroups.value[category]?.splice(index, 1)
  await persistQuickCommands()
}

async function resetDefaultCommands() {
  if (!confirm('Khôi phục toàn bộ bảng lệnh kiểm tra về mặc định ban đầu?')) return
  try {
    const res = await apiRequest<{ commands: Record<string, QuickCommandItem[]> }>('/ai/quick-commands', {
      method: 'POST',
      body: JSON.stringify({ reset: true }),
    })
    quickCommandGroups.value = res.commands || JSON.parse(JSON.stringify(defaultQuickCommands))
  } catch {
    quickCommandGroups.value = JSON.parse(JSON.stringify(defaultQuickCommands))
  }
}

async function persistQuickCommands() {
  try {
    await apiRequest('/ai/quick-commands', {
      method: 'PUT',
      body: JSON.stringify({ commands: quickCommandGroups.value }),
    })
  } catch (e) {
    console.error('Failed to save quick commands to router:', e)
    localStorage.setItem('r3p_quick_commands', JSON.stringify(quickCommandGroups.value))
  }
}

async function loadQuickCommands() {
  try {
    const res = await apiRequest<{ commands: Record<string, QuickCommandItem[]> }>('/ai/quick-commands')
    if (res.commands && Object.keys(res.commands).length > 0) {
      quickCommandGroups.value = res.commands
      return
    }
  } catch {
    const saved = localStorage.getItem('r3p_quick_commands')
    if (saved) {
      try {
        quickCommandGroups.value = JSON.parse(saved)
        return
      } catch {}
    }
  }
}

async function addMemory() {
  const content = newMemoryContent.value.trim()
  if (!content) return
  try {
    const res = await apiRequest<{ memories: AIMemoryEntry[] }>('/ai/memory', {
      method: 'POST',
      body: JSON.stringify({
        category: newMemoryCategory.value,
        content,
      }),
    })
    memories.value = res.memories || []
    newMemoryContent.value = ''
    showNewMemoryModal.value = false
  } catch (e) {
    errorMsg.value = (e as Error).message
  }
}

async function deleteMemory(id: string) {
  try {
    const res = await apiRequest<{ memories: AIMemoryEntry[] }>(`/ai/memory?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
    memories.value = res.memories || []
  } catch (e) {
    errorMsg.value = (e as Error).message
  }
}

async function saveConfiguration(clearKey = false) {
  savingConfig.value = true
  feedback.value = ''
  errorMsg.value = ''
  try {
    await apiRequest('/ai/config', {
      method: 'PUT',
      body: JSON.stringify({
        ...config.value,
        clear_key: clearKey,
      }),
    })
    feedback.value = 'Đã lưu cấu hình AI thành công!'
    config.value.has_api_key = !clearKey && (config.value.api_key.trim() !== '' || config.value.has_api_key)
    setTimeout(() => { feedback.value = '' }, 4000)
    await loadState()
  } catch (e) {
    errorMsg.value = (e as Error).message
  } finally {
    savingConfig.value = false
  }
}

async function sendMessage(textToSend?: string) {
  const text = (textToSend ?? promptInput.value).trim()
  if (!text || sending.value) return

  sending.value = true
  promptInput.value = ''
  errorMsg.value = ''

  history.value.push({
    role: 'user',
    content: text,
    timestamp: new Date().toLocaleTimeString(),
    source: 'web',
  })
  await scrollToBottom()

  try {
    const res = await apiRequest<{
      reply: AIChatMessage
      history: AIChatMessage[]
      commands?: AICmdLog[]
      tool_events?: AIToolAction[]
      needs_browser_bridge?: boolean
      bridge?: any
      memories?: AIMemoryEntry[]
    }>('/ai/chat', {
      method: 'POST',
      body: JSON.stringify({
        session_id: activeSessionId.value,
        message: text,
        execute: config.value.allow_exec,
      }),
    })

    if (res.memories) {
      memories.value = res.memories
    }

    // Handle Browser Bridge fallback when router WAN is disconnected
    if (res.needs_browser_bridge && res.bridge) {
      history.value.push({
        role: 'assistant',
        content: '📡 Router chưa có mạng WAN trực tiếp, đang kích hoạt cầu nối qua trình duyệt máy tính...',
        timestamp: new Date().toLocaleTimeString(),
      })
      await scrollToBottom()
      try {
        let bridgeMessages: Array<{ role: string; content: string }> = Array.isArray(res.bridge.messages)
          ? [...res.bridge.messages]
          : []
        let prevCommands: AICmdLog[] = []
        let finalContHistory: AIChatMessage[] | null = null
        let bridgeTurn = Number((res as any).bridge_turn ?? 1)

        for (let turn = 0; turn < 3; turn++) {
          const bridgeKey = config.value.api_key.trim()
          if (!bridgeKey) {
            showConfig.value = true
            throw new Error('Router đang mất kết nối WAN. Vui lòng nhập API Key vào ô API Key ở mục Cài đặt phía trên để dùng Browser Bridge (máy chủ không xuất key plaintext ra trình duyệt).')
          }
          const bridgeResp = await fetch(res.bridge.endpoint, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              Authorization: `Bearer ${bridgeKey}`,
            },
            body: JSON.stringify({
              model: res.bridge.model,
              messages: bridgeMessages,
              temperature: 0.2,
            }),
          })
          const bridgeData = await bridgeResp.json().catch(() => ({}))
          if (!bridgeResp.ok) {
            const msg = bridgeData?.error?.message || bridgeData?.message || `HTTP ${bridgeResp.status}`
            throw new Error(msg)
          }
          const rawContent = bridgeData?.choices?.[0]?.message?.content || ''
          if (!rawContent) {
            throw new Error('Phản hồi từ Browser Bridge rỗng hoặc sai định dạng')
          }

          const contRes = await apiRequest<{
            reply?: AIChatMessage
            history?: AIChatMessage[]
            memories?: AIMemoryEntry[]
            needs_browser_followup?: boolean
            followup_user_prompt?: string
            bridge_turn?: number
            commands?: AICmdLog[]
          }>('/ai/chat', {
            method: 'POST',
            body: JSON.stringify({
              session_id: activeSessionId.value,
              browser_reply: rawContent,
              execute: config.value.allow_exec,
              prev_commands: prevCommands,
              bridge_turn: bridgeTurn,
            }),
          })
          if (contRes.memories) memories.value = contRes.memories
          if (contRes.needs_browser_followup && contRes.followup_user_prompt) {
            bridgeTurn = Number(contRes.bridge_turn ?? (bridgeTurn + 1))
            prevCommands = contRes.commands || prevCommands
            bridgeMessages = [
              ...bridgeMessages,
              { role: 'assistant', content: rawContent },
              { role: 'user', content: contRes.followup_user_prompt },
            ]
            continue
          }
          if (Array.isArray(contRes.history)) {
            finalContHistory = contRes.history
          }
          break
        }
        if (finalContHistory) {
          history.value = finalContHistory
        }
      } catch (bridgeErr: any) {
        errorMsg.value = `Lỗi Browser Bridge: ${bridgeErr?.message || bridgeErr}`
        history.value.push({
          role: 'assistant',
          content: `❌ Lỗi gọi API qua cầu nối trình duyệt: ${bridgeErr?.message || bridgeErr}`,
          timestamp: new Date().toLocaleTimeString(),
        })
      }
    } else if (Array.isArray(res.history)) {
      history.value = res.history
    }
  } catch (e) {
    errorMsg.value = (e as Error).message
  } finally {
    sending.value = false
    await scrollToBottom()
  }
}

function executeQuickCommand(cmd: string) {
  void sendMessage(cmd)
}

async function clearChat() {
  if (!confirm('Bạn có muốn xóa sạch lịch sử trong phiên này?')) return
  errorMsg.value = ''
  try {
    await apiRequest('/ai/chat', { method: 'DELETE' })
    await loadState()
    feedback.value = 'Đã xóa lịch sử trò chuyện trong phiên hiện tại.'
    setTimeout(() => { feedback.value = '' }, 3000)
  } catch (e) {
    errorMsg.value = (e as Error).message
  }
}

onMounted(async () => {
  await Promise.all([
    loadState(),
    loadQuickCommands(),
  ])
})
</script>

<template>
  <div class="h-full flex flex-col space-y-3 p-2 sm:p-3 bg-[#070a12] overflow-hidden max-w-full">
    <!-- Top Configuration Accordion Bar -->
    <div class="pro-card p-3 shrink-0">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2 min-w-0">
          <span class="text-lg">🤖</span>
          <div>
            <div class="flex items-center gap-2">
              <span class="text-xs font-bold text-gray-200 uppercase tracking-wider">{{ t('ai_config_heading') }}</span>
              <span
                class="badge text-[9.5px]"
                :class="config.has_api_key ? 'badge-green' : 'badge-orange'"
              >
                {{ config.has_api_key ? t('ai_key_ready') : t('ai_key_missing') }}
              </span>
              <span class="badge badge-blue text-[9.5px]">MODEL: {{ config.model }}</span>
            </div>
            <div class="text-[10.5px] text-gray-500 truncate hidden sm:block">
              {{ t('ai_subtitle') }}
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2 shrink-0">
          <button
            type="button"
            class="btn-secondary py-1 px-2.5 text-xs flex items-center gap-1.5"
            @click="loadBlueprint"
          >
            <span>📜</span>
            <span class="hidden md:inline">Xem Blueprint Hệ Thống</span>
          </button>
          <button
            type="button"
            class="btn-primary py-1 px-2.5 min-w-[144px] text-xs flex items-center gap-1.5"
            @click="showConfig = !showConfig"
          >
            <span>⚙️</span>
            <span>{{ showConfig ? 'Thu Gọn Cài Đặt' : 'Mở Cấu Hình API' }}</span>
          </button>
        </div>
      </div>

      <!-- Global Feedback / Error Banners (Always Visible) -->
      <div v-if="feedback" class="mt-2 p-2.5 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-emerald-300 text-xs">
        ✅ {{ feedback }}
      </div>
      <div v-if="errorMsg" class="mt-2 p-2.5 rounded-lg bg-rose-500/10 border border-rose-500/30 text-rose-300 text-xs">
        ⚠️ {{ errorMsg }}
      </div>

      <!-- Expandable API Configuration Form -->
      <div v-if="showConfig" class="mt-3 pt-3 border-t border-white/10 space-y-3">

        <div class="grid grid-cols-1 md:grid-cols-4 gap-2.5">
          <div>
            <label class="block text-[10px] text-gray-400 uppercase font-semibold mb-1">Nhà Cung Cấp (Provider)</label>
            <select v-model="config.provider" class="w-full pro-input text-xs" @change="onProviderChange">
              <option value="gemini">Google Gemini (Khuyên dùng)</option>
              <option value="openai">OpenAI (GPT-4o / o3-mini)</option>
              <option value="deepseek">DeepSeek (V3 / R1)</option>
              <option value="anthropic">Anthropic (Claude 3.7)</option>
              <option value="groq">Groq (Llama 3.3 siêu tốc)</option>
              <option value="openrouter">OpenRouter (Tổng hợp)</option>
            </select>
          </div>

          <div>
            <label class="block text-[10px] text-gray-400 uppercase font-semibold mb-1">Mô Hình (Model)</label>
            <div class="flex items-center gap-1.5">
              <input
                v-model="config.model"
                type="text"
                list="ai-models-list"
                class="w-full pro-input mono text-xs"
                placeholder="gemini-3.1-flash-lite..."
              />
              <datalist id="ai-models-list">
                <option v-for="m in (providerModels[config.provider] || [])" :key="m" :value="m">{{ m }}</option>
              </datalist>
            </div>
            <!-- Quick model pills for Gemini -->
            <div v-if="config.provider === 'gemini'" class="flex flex-wrap gap-1 mt-1 text-[9.5px]">
              <button
                type="button"
                class="px-1.5 py-0.5 rounded cursor-pointer transition-colors"
                :class="config.model === 'gemini-3.1-flash-lite' ? 'bg-sky-500/20 text-sky-400 border border-sky-500/40 font-bold' : 'bg-white/5 text-gray-400 hover:text-white'"
                @click="config.model = 'gemini-3.1-flash-lite'"
                title="Khuyên dùng cho Free Tier: Không bị giới hạn 20 req/ngày"
              >
                ⭐ 3.1-flash-lite (Khuyên dùng)
              </button>
              <button
                type="button"
                class="px-1.5 py-0.5 rounded cursor-pointer transition-colors"
                :class="config.model === 'gemini-3.1-flash-lite-preview' ? 'bg-sky-500/20 text-sky-400 border border-sky-500/40 font-bold' : 'bg-white/5 text-gray-400 hover:text-white'"
                @click="config.model = 'gemini-3.1-flash-lite-preview'"
                title="Bản preview 3.1-flash-lite"
              >
                3.1-flash-lite-preview
              </button>
              <button
                type="button"
                class="px-1.5 py-0.5 rounded cursor-pointer transition-colors"
                :class="config.model === 'gemini-flash-latest' ? 'bg-sky-500/20 text-sky-400 border border-sky-500/40 font-bold' : 'bg-white/5 text-gray-400 hover:text-white'"
                @click="config.model = 'gemini-flash-latest'"
                title="Bản Flash mới nhất"
              >
                flash-latest
              </button>
              <button
                type="button"
                class="px-1.5 py-0.5 rounded cursor-pointer transition-colors"
                :class="config.model === 'gemini-3-flash-preview' ? 'bg-amber-500/20 text-amber-300 border border-amber-500/40 font-bold' : 'bg-white/5 text-gray-400 hover:text-white'"
                @click="config.model = 'gemini-3-flash-preview'"
                title="Hạn mức thấp: chỉ 20 lượt/ngày trên Free Tier"
              >
                3-flash-preview (20 req)
              </button>
            </div>
          </div>

          <div class="md:col-span-2">
            <label class="block text-[10px] text-gray-400 uppercase font-semibold mb-1">
              API Key (Nhập key để lưu vào hệ thống)
            </label>
            <div class="flex items-center gap-1.5">
              <input
                v-model="config.api_key"
                :type="showApiKey ? 'text' : 'password'"
                class="w-full pro-input mono text-xs"
                :placeholder="config.has_api_key ? 'Đã lưu API Key (nhập key mới để thay đổi)' : 'Dán API Key tại đây...'"
              />
              <button
                type="button"
                class="btn-secondary py-1.5 px-2 min-w-[46px] text-xs shrink-0"
                @click="showApiKey = !showApiKey"
              >
                {{ showApiKey ? 'Ẩn' : 'Hiện' }}
              </button>
            </div>
          </div>
        </div>

        <div class="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-white/5">
          <div class="flex items-center gap-4 text-xs">
            <label class="flex items-center gap-1.5 text-gray-300 cursor-pointer select-none">
              <input v-model="config.allow_exec" type="checkbox" class="rounded accent-sky-500" />
              <span>Cấp toàn quyền Root tự động thực thi lệnh</span>
            </label>
            <label class="flex items-center gap-1.5 text-gray-300 cursor-pointer select-none">
              <input v-model="config.telegram_ai_enabled" type="checkbox" class="rounded accent-emerald-500" />
              <span>Bật phản hồi AI qua Telegram Bot</span>
            </label>
          </div>

          <div class="flex items-center gap-2">
            <button
              v-if="config.has_api_key"
              type="button"
              class="btn-danger py-1 px-2.5 text-xs"
              :disabled="savingConfig"
              @click="saveConfiguration(true)"
            >
              Xóa Key Đã Lưu
            </button>
            <button
              type="button"
              class="btn-primary py-1 px-3 min-w-[138px] text-xs"
              :disabled="savingConfig"
              @click="saveConfiguration(false)"
            >
              {{ savingConfig ? 'Đang lưu...' : 'Lưu Cấu Hình API' }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- MAIN 2-COLUMN WORKSPACE -->
    <div class="flex-1 grid grid-cols-1 lg:grid-cols-12 gap-3 min-h-0 overflow-hidden">
      <!-- CỘT TRÁI (35% width / 4-cols on lg): Quản Lý Phiên, Bộ Nhớ AI & Bảng Lệnh Sẵn Có -->
      <div class="lg:col-span-4 flex flex-col space-y-3 min-h-0 overflow-y-auto pr-1">
        <!-- 1. Quản lý Phiên (Multi-Session Switcher) -->
        <div class="pro-card p-3 space-y-2 shrink-0">
          <div class="flex items-center justify-between">
            <span class="text-[13px] font-bold text-gray-200 uppercase tracking-wider flex items-center gap-1.5">
              <span>💬</span>
              <span>Phiên Làm Việc ({{ sessions.length }})</span>
            </span>
            <button
              type="button"
              class="btn-primary py-1 px-2.5 text-[12px] font-bold flex items-center gap-1"
              title="Tạo phiên trò chuyện mới"
              @click="createNewSession"
            >
              + Phiên Mới
            </button>
          </div>

          <div class="flex items-center gap-1.5 overflow-x-auto py-1">
            <button
              v-for="sess in sessions"
              :key="sess.id"
              type="button"
              class="px-3 py-1.5 rounded-lg border text-[13px] font-semibold whitespace-nowrap transition flex items-center gap-1.5 shrink-0"
              :class="activeSessionId === sess.id
                ? 'bg-sky-500/20 border-sky-500/50 text-sky-300'
                : 'bg-black/25 border-white/10 text-gray-300 hover:text-gray-100'"
              @click="switchSession(sess.id)"
            >
              <span>{{ sess.title }}</span>
              <span
                v-if="sessions.length > 1 && activeSessionId === sess.id"
                class="text-rose-400 hover:text-rose-200 ml-1 font-bold"
                title="Xóa phiên này"
                @click.stop="deleteSession(sess.id)"
              >
                ✕
              </span>
            </button>
          </div>
        </div>

        <!-- 2. Bộ Nhớ Dài Hạn (AI Long-Term Memory) -->
        <div class="pro-card p-3 space-y-2 shrink-0">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1.5">
              <span class="text-base">🧠</span>
              <span class="text-[13px] font-bold text-gray-200 uppercase tracking-wider">
                Bộ Nhớ AI ({{ memories.length }})
              </span>
            </div>
            <button
              type="button"
              class="btn-secondary py-1 px-2.5 text-[12px] font-semibold"
              @click="showNewMemoryModal = true"
            >
              + Ghi Nhớ
            </button>
          </div>

          <div class="space-y-1.5 max-h-40 overflow-y-auto">
            <div
              v-for="m in memories"
              :key="m.id"
              class="p-2.5 rounded-lg bg-black/30 border border-white/5 text-[12.5px] flex items-start justify-between gap-1.5 group"
            >
              <div class="min-w-0">
                <div class="text-gray-200 leading-snug break-words">{{ m.content }}</div>
                <div class="text-[11px] text-gray-400 mt-1 mono font-medium">
                  {{ m.category }} · {{ m.created_at }}
                </div>
              </div>
              <button
                type="button"
                class="text-gray-400 hover:text-rose-400 opacity-0 group-hover:opacity-100 transition shrink-0 font-bold px-1"
                title="Xóa ghi nhớ"
                @click="deleteMemory(m.id)"
              >
                ✕
              </button>
            </div>
          </div>
        </div>

        <!-- 3. BẢNG LỆNH KIỂM TRA & CÀI ĐẶT HỆ THỐNG SẴN CÓ (QUICK ACTIONS) -->
        <div class="pro-card p-3 flex-1 flex flex-col min-h-0 space-y-2.5">
          <div class="flex items-center justify-between gap-2">
            <span class="text-[13px] font-bold text-gray-200 uppercase tracking-wider flex items-center gap-1.5">
              <span>⚡</span>
              <span>Bảng Lệnh Kiểm Tra Sẵn Có</span>
            </span>
            <div class="flex items-center gap-1.5">
              <button
                type="button"
                class="px-2.5 py-1 rounded-md text-[12px] font-bold bg-sky-500/20 text-sky-300 border border-sky-500/40 hover:bg-sky-500/30 transition flex items-center gap-1 cursor-pointer shadow-sm"
                title="Thêm đoạn lệnh kiểm tra mới (Quyền Root)"
                @click="openAddCommandModal"
              >
                <span>+</span>
                <span>Thêm</span>
              </button>
              <button
                type="button"
                class="px-2 py-1 rounded-md text-[11.5px] font-medium bg-white/5 text-gray-400 hover:text-gray-100 hover:bg-white/10 transition cursor-pointer border border-white/5"
                title="Khôi phục danh sách lệnh mặc định ban đầu"
                @click="resetDefaultCommands"
              >
                ↻ Mặc định
              </button>
            </div>
          </div>

          <!-- Category Selector Tabs -->
          <div class="flex items-center gap-1.5 overflow-x-auto pb-1 text-[12px]">
            <button
              v-for="(label, key) in {
                quick: '🩺 Kiểm tra',
                network: '📡 Mạng & Wi-Fi',
                storage: '💾 Lưu trữ',
                adblock: '🛡️ AdBlock',
                download: '⬇️ Download',
                maintenance: '⚙️ Bảo trì',
              }"
              :key="key"
              type="button"
              class="px-2.5 py-1.5 rounded-lg border font-semibold whitespace-nowrap transition cursor-pointer"
              :class="activeCommandTab === key
                ? 'bg-sky-500/20 border-sky-500/50 text-sky-300'
                : 'bg-black/25 border-white/10 text-gray-400 hover:text-gray-200'"
              @click="activeCommandTab = key"
            >
              {{ label }}
            </button>
          </div>

          <!-- Commands list for selected tab -->
          <div class="space-y-1.5 overflow-y-auto flex-1 pr-0.5">
            <div
              v-for="(item, idx) in (quickCommandGroups[activeCommandTab] || [])"
              :key="idx"
              class="w-full p-2.5 rounded-lg border border-white/10 bg-black/20 hover:bg-sky-500/10 hover:border-sky-500/30 transition group flex items-center justify-between gap-2.5"
            >
              <div
                class="min-w-0 flex-1 cursor-pointer"
                title="Bấm để đưa lệnh vào hộp thoại và thực thi"
                @click="executeQuickCommand(item.cmd)"
              >
                <div class="text-[13.5px] font-semibold text-gray-100 group-hover:text-sky-300 transition truncate">
                  {{ item.label }}
                </div>
                <div class="text-[12px] mono text-emerald-400/90 group-hover:text-emerald-300 truncate mt-0.5 font-medium">
                  {{ item.cmd }}
                </div>
              </div>

              <!-- Root Action Buttons: Sửa, Xóa, Chạy -->
              <div class="flex items-center gap-1 shrink-0">
                <button
                  type="button"
                  class="p-1.5 rounded-md text-gray-400 hover:text-sky-300 hover:bg-sky-500/20 border border-transparent hover:border-sky-500/30 transition cursor-pointer text-xs"
                  title="Chỉnh sửa nội dung lệnh này"
                  @click.stop="openEditCommandModal(activeCommandTab, idx)"
                >
                  ✏️
                </button>
                <button
                  type="button"
                  class="p-1.5 rounded-md text-gray-400 hover:text-rose-400 hover:bg-rose-500/20 border border-transparent hover:border-rose-500/30 transition cursor-pointer text-xs"
                  title="Xóa lệnh này khỏi danh mục"
                  @click.stop="deleteCommandItem(activeCommandTab, idx)"
                >
                  🗑️
                </button>
                <button
                  type="button"
                  class="p-1.5 px-2 rounded-md bg-sky-500/20 text-sky-300 hover:bg-sky-500/40 hover:text-white border border-sky-500/40 transition cursor-pointer font-bold text-xs"
                  title="Chạy lệnh ngay lập tức"
                  @click.stop="executeQuickCommand(item.cmd)"
                >
                  ▶
                </button>
              </div>
            </div>

            <div
              v-if="!(quickCommandGroups[activeCommandTab] && quickCommandGroups[activeCommandTab].length)"
              class="text-center py-6 text-xs text-gray-500 italic"
            >
              Chưa có lệnh nào trong mục này. Bấm nút "+ Thêm" bên trên để tạo mới.
            </div>
          </div>
        </div>
      </div>

      <!-- CỘT PHẢI (65% width / 8-cols on lg): Cửa Sổ Giao Tiếp AI & Terminal Root Chat -->
      <div class="lg:col-span-8 pro-card p-0 flex flex-col min-h-0 overflow-hidden border border-sky-500/20 shadow-xl">
        <!-- Chat Header -->
        <div class="px-4 py-3 bg-black/30 border-b border-white/10 flex items-center justify-between shrink-0">
          <div class="flex items-center gap-2">
            <span class="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse"></span>
            <span class="text-[14px] font-bold text-gray-100">
              Cửa Sổ Giao Tiếp & Terminal Root (root@R3P)
            </span>
            <span class="text-[12px] text-gray-400 mono hidden sm:inline">
              Gõ !&lt;lệnh&gt; hoặc /sh &lt;lệnh&gt; để thực thi shell trực tiếp
            </span>
          </div>

          <div class="flex items-center gap-2">
            <button
              type="button"
              class="btn-secondary py-1 px-3 text-[12px] font-semibold"
              title="Xóa lịch sử chat trong phiên này"
              @click="clearChat"
            >
              🧹 Xóa chat
            </button>
          </div>
        </div>

        <!-- Chat Messages Container -->
        <div ref="chatContainer" class="flex-1 p-3.5 overflow-y-auto space-y-3.5 bg-[#080b13]">
          <div
            v-for="(msg, idx) in history"
            :key="idx"
            class="space-y-1.5"
            :class="msg.role === 'user' ? 'flex flex-col items-end' : 'flex flex-col items-start'"
          >
            <!-- Sender Badge -->
            <div class="flex items-center gap-1.5 text-[11.5px] text-gray-400 px-1 font-medium">
              <span class="font-bold uppercase tracking-wider" :class="msg.role === 'user' ? 'text-sky-400' : 'text-emerald-400'">
                {{ msg.role === 'user' ? 'Quản Trị Viên (User)' : 'Root AI Agent' }}
              </span>
              <span v-if="msg.source === 'telegram'" class="px-1.5 py-0.2 rounded bg-sky-500/20 text-sky-300 font-bold">
                📱 TELEGRAM
              </span>
              <span>· {{ msg.timestamp }}</span>
            </div>

            <!-- Message Bubble -->
            <div
              class="max-w-[92%] min-w-0 overflow-hidden rounded-xl px-4 py-3 text-[13.5px] leading-relaxed"
              :class="msg.role === 'user'
                ? 'bg-sky-600/20 border border-sky-500/40 text-gray-100 rounded-tr-none'
                : 'bg-black/40 border border-white/10 text-gray-100 rounded-tl-none space-y-2.5'"
            >
              <div class="whitespace-pre-wrap break-words select-text font-sans">{{ msg.content }}</div>

              <!-- Executed Shell Command Results (Logs) -->
              <div v-if="msg.commands && msg.commands.length > 0" class="mt-2.5 space-y-2 pt-2.5 border-t border-white/10">
                <div class="text-[12px] font-bold text-amber-400 uppercase tracking-wider flex items-center gap-1.5">
                  <span>⚡ Lệnh Hệ Thống Đã Chạy Trên Router:</span>
                </div>
                <div
                  v-for="(cmd, cIdx) in msg.commands"
                  :key="cIdx"
                  class="p-2.5 rounded-lg bg-black/70 border border-white/10 mono text-[12px] min-w-0 overflow-hidden"
                >
                  <div class="flex items-center justify-between gap-2 text-emerald-400 font-bold mb-1.5">
                    <span class="text-[13px] truncate">$ {{ cmd.command }}</span>
                    <span class="text-[11px] text-gray-400 font-normal shrink-0">
                      {{ cmd.duration_ms }}ms · Exit {{ cmd.exit_code }}
                    </span>
                  </div>
                  <pre class="text-gray-200 whitespace-pre-wrap break-words max-h-56 overflow-y-auto text-[12px] leading-relaxed font-mono">{{ cmd.output }}</pre>
                </div>
              </div>
            </div>
          </div>

          <div v-if="sending" class="flex items-center gap-2 text-[13px] text-sky-400 animate-pulse p-2">
            <span>🤖 AI đang suy nghĩ & thực thi lệnh trên Router R3P...</span>
          </div>
        </div>

        <!-- Chat Input Bar -->
        <div class="p-3 bg-black/40 border-t border-white/10 shrink-0">
          <form class="flex items-center gap-2.5" @submit.prevent="sendMessage()">
            <div class="relative flex-1">
              <input
                v-model="promptInput"
                type="text"
                class="w-full pro-input py-2.5 pl-3.5 pr-14 text-[13.5px] font-sans"
                placeholder="Nhập yêu cầu bằng tiếng Việt (hoặc gõ !lệnh để chạy root shell trực tiếp)..."
                :disabled="sending"
              />
              <span
                v-if="promptInput.startsWith('!')"
                class="absolute right-3 top-2.5 text-[11px] font-bold text-amber-400 mono px-1.5 py-0.5 rounded bg-amber-500/10 border border-amber-500/30"
                title="Chế độ thực thi lệnh root shell trực tiếp"
              >
                ROOT!
              </span>
            </div>

            <button
              type="submit"
              class="btn-primary py-2.5 px-5 text-[13px] font-bold shrink-0 flex items-center gap-1.5 shadow-md"
              :disabled="!promptInput.trim() || sending"
            >
              <span>Gửi</span>
              <span>➔</span>
            </button>
          </form>
        </div>
      </div>
    </div>

    <!-- System Blueprint Modal (/etc/r3p/SYSTEM_BLUEPRINT.md) -->
    <div
      v-if="showBlueprintModal"
      class="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-3"
    >
      <div class="pro-card max-w-4xl w-full max-h-[92vh] flex flex-col border-sky-500/40 p-4 space-y-3 shadow-2xl">
        <!-- Header with Title & Mode Switcher -->
        <div class="flex items-center justify-between border-b border-white/10 pb-2.5">
          <div class="flex items-center gap-2.5">
            <span class="text-xl">📜</span>
            <div>
              <h3 class="text-sm font-bold text-gray-100 uppercase tracking-wide flex items-center gap-2">
                <span>Bản Thiết Kế Hệ Thống</span>
                <span class="badge badge-blue text-[10px] font-mono">SYSTEM_BLUEPRINT.md</span>
              </h3>
              <p class="text-[11.5px] text-gray-400 font-mono">Đường dẫn: /etc/r3p/SYSTEM_BLUEPRINT.md</p>
            </div>
          </div>

          <div class="flex items-center gap-2">
            <!-- Mode Switcher: Xem Trước vs Chỉnh Sửa Trực Tiếp -->
            <div class="flex items-center bg-black/40 border border-white/10 rounded-lg p-0.5 text-xs">
              <button
                type="button"
                class="px-3 py-1 rounded transition font-semibold cursor-pointer"
                :class="!blueprintEditMode ? 'bg-sky-500/20 text-sky-300 font-bold border border-sky-500/40 shadow-sm' : 'text-gray-400 hover:text-gray-200'"
                @click="blueprintEditMode = false"
              >
                👁️ Xem Trước
              </button>
              <button
                type="button"
                class="px-3 py-1 rounded transition font-semibold cursor-pointer"
                :class="blueprintEditMode ? 'bg-sky-500/20 text-sky-300 font-bold border border-sky-500/40 shadow-sm' : 'text-gray-400 hover:text-gray-200'"
                @click="blueprintEditMode = true"
              >
                ✏️ Chỉnh Sửa Trực Tiếp
              </button>
            </div>
            <button class="text-gray-400 hover:text-white text-base px-2 py-1 cursor-pointer" @click="showBlueprintModal = false">✕</button>
          </div>
        </div>

        <div v-if="blueprintFeedback" class="p-2.5 rounded-lg bg-emerald-500/20 border border-emerald-500/40 text-emerald-300 text-xs flex items-center gap-2">
          <span>✅</span>
          <span>{{ blueprintFeedback }}</span>
        </div>

        <div class="text-[12px] text-gray-400 leading-relaxed">
          Tài liệu tham chiếu cốt lõi để AI Agent hiểu toàn bộ kiến trúc phần cứng MT7621AT, phân vùng SLC NAND Flash (/overlay), ổ đĩa SSD USB 3.0 (/mnt/sda1), sơ đồ mạng và Wi-Fi của Xiaomi Mi Router 3 Pro.
        </div>

        <!-- Hidden input for file upload -->
        <input
          ref="blueprintFileInput"
          type="file"
          accept=".md,.markdown,.txt"
          class="hidden"
          @change="onBlueprintFileSelected"
        />

        <!-- Content Area: Editor vs Preview -->
        <div class="flex-1 min-h-[360px] max-h-[58vh] flex flex-col">
          <textarea
            v-if="blueprintEditMode"
            v-model="systemBlueprint"
            rows="18"
            class="w-full flex-1 p-3.5 bg-black/70 font-mono text-[12.5px] text-emerald-300 border border-white/10 rounded-lg focus:outline-none focus:border-sky-500 leading-relaxed resize-none shadow-inner"
            placeholder="Nội dung file Markdown /etc/r3p/SYSTEM_BLUEPRINT.md..."
          ></textarea>
          <div
            v-else
            class="flex-1 overflow-y-auto p-3.5 bg-black/40 rounded-lg border border-white/5"
          >
            <pre class="text-gray-200 mono text-[12px] whitespace-pre-wrap leading-relaxed">{{ systemBlueprint }}</pre>
          </div>
        </div>

        <!-- Footer Actions Bar: Download, Upload, Reset, Save -->
        <div class="flex flex-wrap items-center justify-between gap-2 border-t border-white/10 pt-3">
          <div class="flex items-center gap-2">
            <button
              type="button"
              class="btn-secondary py-1.5 px-3 text-xs flex items-center gap-1.5 cursor-pointer"
              title="Tải file SYSTEM_BLUEPRINT.md về máy tính"
              @click="downloadBlueprint"
            >
              <span>📥</span>
              <span>Tải Xuống (.md)</span>
            </button>
            <button
              type="button"
              class="btn-secondary py-1.5 px-3 text-xs flex items-center gap-1.5 cursor-pointer"
              title="Upload file mới từ máy tính để thay thế Blueprint"
              @click="triggerBlueprintUpload"
            >
              <span>📤</span>
              <span>Upload File Mới</span>
            </button>
            <button
              type="button"
              class="text-xs text-gray-400 hover:text-amber-400 transition underline decoration-dotted ml-2 cursor-pointer"
              title="Khôi phục nội dung gốc ban đầu"
              @click="resetBlueprint"
            >
              ↻ Khôi phục mặc định
            </button>
          </div>

          <div class="flex items-center gap-2">
            <button
              v-if="blueprintEditMode"
              type="button"
              class="btn-primary py-1.5 px-4 text-xs font-bold flex items-center gap-1.5 cursor-pointer shadow-md"
              :disabled="savingBlueprint"
              @click="saveBlueprint"
            >
              <span>💾</span>
              <span>{{ savingBlueprint ? 'Đang lưu...' : 'Lưu Thay Đổi Lên Router' }}</span>
            </button>
            <button
              type="button"
              class="btn-secondary py-1.5 px-3.5 text-xs cursor-pointer"
              @click="showBlueprintModal = false"
            >
              Đóng
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Modal Thêm / Chỉnh Sửa Lệnh Kiểm Tra Sẵn Có -->
    <div
      v-if="showCommandModal"
      class="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-3"
    >
      <div class="pro-card max-w-md w-full border-sky-500/40 p-4 space-y-3 shadow-2xl">
        <div class="flex items-center justify-between border-b border-white/10 pb-2">
          <h3 class="text-sm font-bold text-gray-100 flex items-center gap-1.5">
            <span>⚡</span>
            <span>{{ isEditingCommand ? 'Chỉnh Sửa Lệnh Kiểm Tra' : 'Thêm Lệnh Kiểm Tra Mới' }}</span>
          </h3>
          <button class="text-gray-400 hover:text-white text-base cursor-pointer" @click="showCommandModal = false">✕</button>
        </div>

        <div>
          <label class="block text-xs font-semibold text-gray-400 mb-1">Danh mục lệnh:</label>
          <select v-model="commandForm.category" class="w-full pro-input text-xs">
            <option value="quick">🩺 Kiểm tra tổng quan</option>
            <option value="network">📡 Mạng & Wi-Fi</option>
            <option value="storage">💾 Lưu trữ & USB</option>
            <option value="adblock">🛡️ AdBlock & DNS</option>
            <option value="download">⬇️ Download & Media</option>
            <option value="maintenance">⚙️ Bảo trì & Tối ưu</option>
          </select>
        </div>

        <div>
          <label class="block text-xs font-semibold text-gray-400 mb-1">Tiêu đề / Tên mô tả lệnh:</label>
          <input
            v-model="commandForm.label"
            type="text"
            class="w-full pro-input text-xs"
            placeholder="Ví dụ: 🌐 Kiểm tra IP Public ngoài Internet"
          />
        </div>

        <div>
          <label class="block text-xs font-semibold text-gray-400 mb-1">Đoạn lệnh Shell (bắt đầu bằng !):</label>
          <textarea
            v-model="commandForm.cmd"
            rows="3"
            class="w-full bg-[#070b12] text-gray-200 text-xs p-2.5 rounded-lg border border-white/10 focus:border-sky-500 focus:outline-none mono leading-relaxed"
            placeholder="!curl -s ifconfig.me"
          ></textarea>
          <p class="text-[11px] text-gray-400 mt-1">
            Gõ tiền tố <code class="mono text-emerald-400">!</code> để gửi trực tiếp đến shell Linux của Router.
          </p>
        </div>

        <div class="flex justify-end gap-2 pt-2 border-t border-white/10">
          <button type="button" class="btn-secondary text-xs cursor-pointer" @click="showCommandModal = false">Hủy</button>
          <button
            type="button"
            class="btn-primary text-xs font-bold cursor-pointer"
            :disabled="!commandForm.label.trim() || !commandForm.cmd.trim()"
            @click="saveCommandItem"
          >
            {{ isEditingCommand ? 'Cập Nhật Lệnh' : 'Thêm Vào Bảng' }}
          </button>
        </div>
      </div>
    </div>

    <!-- New Memory Modal -->
    <div
      v-if="showNewMemoryModal"
      class="fixed inset-0 bg-black/75 backdrop-blur-sm z-50 flex items-center justify-center p-3"
    >
      <div class="pro-card max-w-md w-full border-sky-500/40 p-4 space-y-3">
        <div class="flex items-center justify-between border-b border-white/10 pb-2">
          <h3 class="text-sm font-bold text-gray-200">Thêm Ghi Nhớ Dài Hạn Cho AI</h3>
          <button type="button" class="text-gray-400 hover:text-white cursor-pointer" @click="showNewMemoryModal = false">✕</button>
        </div>

        <div>
          <label class="block text-xs font-semibold text-gray-400 mb-1">Phân loại ghi nhớ:</label>
          <select v-model="newMemoryCategory" class="w-full pro-input text-xs">
            <option value="system_rule">Quy tắc hệ thống (System Rule)</option>
            <option value="preference">Sở thích người dùng (Preference)</option>
            <option value="network_note">Ghi chú mạng / IP (Network Note)</option>
            <option value="fact">Thông tin khác (Fact)</option>
          </select>
        </div>

        <div>
          <label class="block text-xs font-semibold text-gray-400 mb-1">Nội dung ghi nhớ:</label>
          <textarea
            v-model="newMemoryContent"
            rows="4"
            class="w-full bg-[#070b12] text-gray-200 text-xs p-2.5 rounded-lg border border-white/10 focus:border-sky-500 focus:outline-none"
            placeholder="Ví dụ: Ổ cứng USB gắn ngoài là 500GB ext4 dùng lưu media tải về..."
          ></textarea>
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button type="button" class="btn-secondary text-xs" @click="showNewMemoryModal = false">Hủy</button>
          <button
            type="button"
            class="btn-primary text-xs"
            :disabled="!newMemoryContent.trim()"
            @click="addMemory"
          >
            Lưu Ghi Nhớ
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
