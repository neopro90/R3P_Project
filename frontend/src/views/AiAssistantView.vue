<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { apiRequest } from '@/services/api'

interface AIConfig {
  provider: string
  api_key: string
  api_key_masked?: string
  has_api_key?: boolean
  base_url: string
  model: string
  allow_exec: boolean
  system_note: string
}

interface AICmdLog {
  command: string
  output: string
  exit_code: number
  duration_ms: number
}

interface AIChatMessage {
  role: 'user' | 'assistant'
  content: string
  timestamp: string
  commands?: AICmdLog[]
  tool_events?: AICmdLog[]
}

const loading = ref(false)
const sending = ref(false)
const savingConfig = ref(false)
const showConfig = ref(true) // Always open API Config by default per user request
const showApiKey = ref(false)
const feedback = ref('')
const errorMsg = ref('')

const config = ref<AIConfig>({
  provider: 'gemini',
  api_key: '',
  base_url: '',
  model: 'gemini-3-flash-preview',
  allow_exec: true,
  system_note: '',
})

const history = ref<AIChatMessage[]>([])
const systemContextPreview = ref('')
const showArchModal = ref(false)
const promptInput = ref('')
const chatContainer = ref<HTMLElement | null>(null)

const providerModels: Record<string, string[]> = {
  gemini: ['gemini-3-flash-preview', 'gemini-3.5-flash', 'gemini-3.1-flash-lite', 'gemini-2.5-flash'],
  openai: ['gpt-4o-mini', 'gpt-4o', 'o3-mini'],
  deepseek: ['deepseek-chat', 'deepseek-reasoner'],
  anthropic: ['claude-3-5-sonnet-latest', 'claude-3-5-haiku-latest'],
  groq: ['llama-3.3-70b-versatile', 'mixtral-8x7b-32768'],
  openrouter: ['google/gemini-2.5-flash', 'anthropic/claude-3.5-sonnet', 'deepseek/deepseek-chat'],
}

const quickPrompts = [
  { label: '⚡ Shell: Kiểm tra tổng thể (!uname & df -h)', text: '!uname -a && echo "---" && df -h && echo "---" && ip -br addr && echo "---" && ip route' },
  { label: 'Kiểm tra R3P & USB 3.0', text: 'Kiểm tra chi tiết trạng thái CPU, RAM, phân vùng ổ cứng USB 3.0 (/mnt/sda1) và tốc độ mạng hiện tại.' },
  { label: 'Kiểm tra ESP32-Download & yt-dlp', text: 'Kiểm tra trạng thái dịch vụ ESP32-Download (cổng 8787), phiên bản yt-dlp và danh sách file đã tải.' },
  { label: 'Xem cấu hình Wi-Fi & Client', text: 'Xem cấu hình Wi-Fi 2.4GHz, 5GHz, chế độ thu Wi-Fi Client (WWAN) và danh sách các thiết bị đang kết nối DHCP.' },
  { label: 'Tối ưu RAM & Dọn Cache', text: 'Giải phóng bộ nhớ pagecache (drop_caches), kiểm tra các tiến trình chiếm nhiều RAM nhất và báo cáo dung lượng trống.' },
]

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
      history: AIChatMessage[]
      system_context: string
    }>('/ai/config')
    if (res.config) {
      config.value = {
        ...res.config,
        api_key: '',
      }
    }
    history.value = res.history || []
    systemContextPreview.value = res.system_context || ''
    await scrollToBottom()
  } catch (e) {
    errorMsg.value = (e as Error).message
  } finally {
    loading.value = false
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
    feedback.value = clearKey
      ? 'Đã xóa API Key khỏi hệ thống!'
      : 'Đã lưu cấu hình AI API thành công! Bạn có thể trò chuyện ở cửa sổ Chat bên dưới.'
    await loadState()
    setTimeout(() => { feedback.value = '' }, 4000)
  } catch (e) {
    errorMsg.value = (e as Error).message
  } finally {
    savingConfig.value = false
  }
}

async function callBrowserLLMBridge(
  endpoint: string,
  apiKey: string,
  model: string,
  messages: Array<{ role: string; content: string }>,
  provider: string,
): Promise<string> {
  const modelsToTry = [model]
  if (provider === 'gemini') {
    for (const fb of ['gemini-3-flash-preview', 'gemini-3.5-flash', 'gemini-3.1-flash-lite']) {
      if (!modelsToTry.includes(fb)) modelsToTry.push(fb)
    }
  }
  let lastError = 'Unknown LLM error'
  for (const tryModel of modelsToTry) {
    const resp = await fetch(endpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(apiKey ? { Authorization: `Bearer ${apiKey}` } : {}),
      },
      body: JSON.stringify({
        model: tryModel,
        messages,
        temperature: 0.2,
      }),
    })
    const text = await resp.text()
    if (!resp.ok) {
      lastError = `HTTP ${resp.status} (${tryModel}): ${text.slice(0, 240)}`
      if (resp.status === 404 || resp.status === 503) continue
      throw new Error(lastError)
    }
    const parsed = JSON.parse(text)
    const content = parsed?.choices?.[0]?.message?.content
    if (content) return content
  }
  throw new Error(lastError)
}

async function sendMessage(customText?: string) {
  const msg = (customText ?? promptInput.value).trim()
  if (!msg || sending.value) return
  if (!customText) promptInput.value = ''
  errorMsg.value = ''
  sending.value = true

  history.value.push({
    role: 'user',
    content: msg,
    timestamp: new Date().toLocaleTimeString('vi-VN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
  })
  await scrollToBottom()

  try {
    const res = await apiRequest<{
      reply?: AIChatMessage
      history?: AIChatMessage[]
      needs_browser_bridge?: boolean
      bridge?: {
        provider: string
        endpoint: string
        api_key: string
        model: string
        messages: Array<{ role: string; content: string }>
      }
    }>('/ai/chat', {
      method: 'POST',
      body: JSON.stringify({
        message: msg,
        execute: config.value.allow_exec,
      }),
    })

    // Browser-Bridged AI Mode when Router has no direct WAN Internet yet
    if (res.needs_browser_bridge && res.bridge) {
      const b = res.bridge
      const aiRawText = await callBrowserLLMBridge(b.endpoint, b.api_key, b.model, b.messages, b.provider)
      const step1 = await apiRequest<{
        reply?: AIChatMessage
        history?: AIChatMessage[]
        needs_browser_followup?: boolean
        commands?: AICmdLog[]
        followup_user_prompt?: string
      }>('/ai/chat', {
        method: 'POST',
        body: JSON.stringify({
          browser_reply: aiRawText,
          execute: config.value.allow_exec,
        }),
      })

      if (step1.needs_browser_followup && step1.followup_user_prompt) {
        const nextMessages = [
          ...b.messages,
          { role: 'assistant', content: aiRawText },
          { role: 'user', content: step1.followup_user_prompt },
        ]
        const finalAiText = await callBrowserLLMBridge(b.endpoint, b.api_key, b.model, nextMessages, b.provider)
        const step2 = await apiRequest<{
          reply?: AIChatMessage
          history?: AIChatMessage[]
        }>('/ai/chat', {
          method: 'POST',
          body: JSON.stringify({
            browser_reply: finalAiText,
            prev_commands: step1.commands || [],
            execute: false,
          }),
        })
        if (step2.history) history.value = step2.history
        else if (step2.reply) history.value.push(step2.reply)
      } else if (step1.history) {
        history.value = step1.history
      } else if (step1.reply) {
        history.value.push(step1.reply)
      }
      await scrollToBottom()
      return
    }

    if (res.history) {
      history.value = res.history
    } else if (res.reply) {
      history.value.push(res.reply)
    }
    await scrollToBottom()
  } catch (e) {
    errorMsg.value = (e as Error).message
  } finally {
    sending.value = false
  }
}

async function clearChat() {
  try {
    await apiRequest('/ai/chat', { method: 'DELETE' })
    history.value = []
    feedback.value = 'Đã xóa lịch sử trò chuyện.'
    setTimeout(() => { feedback.value = '' }, 2500)
  } catch (e) {
    errorMsg.value = (e as Error).message
  }
}

function getCmdLogs(msg: AIChatMessage): AICmdLog[] {
  return msg.commands || msg.tool_events || []
}

onMounted(loadState)
</script>

<template>
  <div class="h-full overflow-y-auto p-3 flex flex-col gap-3 bg-[#080b11]">
    <!-- Top Header & Status Bar -->
    <div class="pro-card flex flex-wrap items-center justify-between gap-2 shrink-0">
      <div class="flex items-center gap-2.5">
        <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-sky-500 to-purple-600 flex items-center justify-center text-white font-bold text-xs shadow">
          AI
        </div>
        <div>
          <div class="flex flex-wrap items-center gap-2">
            <h2 class="text-sm font-bold text-white">R3P Root AI System Agent (API & Chat Console)</h2>
            <span class="badge" :class="config.has_api_key ? 'badge-green' : 'badge-orange'">
              {{ config.has_api_key ? `${config.provider.toUpperCase()} · ${config.model} (${config.api_key_masked})` : 'CHƯA LƯU API KEY (DÙNG !LỆNH SHELL ĐƯỢC NGAY)' }}
            </span>
            <span class="badge" :class="config.allow_exec ? 'badge-blue' : 'badge-purple'">
              {{ config.allow_exec ? 'ROOT SHELL EXEC: ON' : 'READ-ONLY CHAT' }}
            </span>
          </div>
          <p class="text-[11px] text-gray-400">
            Nắm toàn bộ cấu trúc R3P (OpenWrt 25.12.5, MT7621, USB 3.0 Storage/Tether, AdBlock, ESP32-Download, UCI, Firewall) · Gõ <code class="mono text-sky-300">!&lt;lệnh&gt;</code> để chạy trực tiếp lệnh root.
          </p>
        </div>
      </div>

      <div class="flex items-center gap-1.5">
        <button class="btn-secondary text-[11px]" @click="showArchModal = !showArchModal">
          {{ showArchModal ? 'Ẩn Kiến Trúc Hệ Thống' : '📐 Xem Kiến Trúc Hệ Thống AI Nắm Giữ' }}
        </button>
        <button class="btn-secondary text-[11px]" @click="showConfig = !showConfig">
          {{ showConfig ? '🔼 Thu Gọn Cài Đặt API' : '⚙ Mở Cài Đặt API' }}
        </button>
        <button class="btn-danger text-[11px]" @click="clearChat">
          🗑 Xóa Lịch Sử Chat
        </button>
      </div>
    </div>

    <!-- Feedback / Error Banners -->
    <div v-if="feedback" class="px-3 py-1.5 rounded bg-emerald-500/15 border border-emerald-500/30 text-emerald-300 text-xs shrink-0">
      {{ feedback }}
    </div>
    <div v-if="errorMsg" class="px-3 py-1.5 rounded bg-red-500/15 border border-red-500/30 text-red-300 text-xs shrink-0">
      {{ errorMsg }}
    </div>

    <!-- System Architecture Context Preview -->
    <div v-if="showArchModal" class="pro-card shrink-0 max-h-56 overflow-y-auto border border-sky-500/30">
      <div class="pro-card-header">
        <span class="pro-card-title text-sky-300">System Prompt & Live Telemetry Injected Into AI Agent</span>
        <button class="btn-secondary text-[10.5px] py-0.5 px-2" @click="showArchModal = false">✕ Đóng</button>
      </div>
      <pre class="mono text-[10.5px] text-gray-300 whitespace-pre-wrap bg-black/50 p-2.5 rounded border border-white/5">{{ systemContextPreview }}</pre>
    </div>

    <!-- SECTION 1 (TOP): API CONFIGURATION PANEL -->
    <div v-if="showConfig" class="pro-card shrink-0 border border-sky-500/30">
      <div class="pro-card-header">
        <span class="pro-card-title text-sky-400">⚙ Cấu Hình Nhúng API Cho AI Agent (Gemini / OpenAI / DeepSeek / Claude / Groq / OpenRouter)</span>
        <span class="badge" :class="config.has_api_key ? 'badge-green' : 'badge-orange'">
          {{ config.has_api_key ? `ĐÃ KẾT NỐI KEY: ${config.api_key_masked}` : 'CHƯA CÓ API KEY' }}
        </span>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-2.5">
        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Nhà Cung Cấp (Provider)</label>
          <select v-model="config.provider" class="pro-input text-xs w-full" @change="onProviderChange">
            <option value="gemini">Google Gemini API</option>
            <option value="openai">OpenAI / Compatible</option>
            <option value="deepseek">DeepSeek API</option>
            <option value="anthropic">Anthropic Claude</option>
            <option value="groq">Groq Cloud (Fast)</option>
            <option value="openrouter">OpenRouter.ai</option>
          </select>
        </div>

        <div>
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Model AI</label>
          <div class="flex gap-1">
            <input v-model="config.model" class="pro-input text-xs mono flex-1" placeholder="gemini-2.5-flash" />
            <select
              v-if="providerModels[config.provider]"
              v-model="config.model"
              class="pro-input text-xs w-32"
              title="Chọn nhanh model"
            >
              <option v-for="m in providerModels[config.provider]" :key="m" :value="m">{{ m }}</option>
            </select>
          </div>
        </div>

        <div class="md:col-span-2">
          <label class="block text-[10px] text-gray-400 uppercase mb-1">
            API Key
            <span v-if="config.has_api_key" class="text-emerald-400 ml-1">(Đang lưu: {{ config.api_key_masked }} — để trống nếu giữ nguyên)</span>
          </label>
          <div class="flex gap-1.5">
            <input
              v-model="config.api_key"
              :type="showApiKey ? 'text' : 'password'"
              class="pro-input text-xs mono flex-1"
              placeholder="Nhập API Key mới (AIzaSy... / sk-...) để cập nhật"
            />
            <button type="button" class="btn-secondary text-[11px]" @click="showApiKey = !showApiKey">
              {{ showApiKey ? 'Ẩn' : 'Hiện' }}
            </button>
            <button
              v-if="config.has_api_key"
              type="button"
              class="btn-danger text-[11px]"
              :disabled="savingConfig"
              @click="saveConfiguration(true)"
            >
              Xóa Key
            </button>
          </div>
        </div>

        <div class="md:col-span-2">
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Custom Base URL (Tùy chọn - để trống dùng mặc định)</label>
          <input
            v-model="config.base_url"
            class="pro-input text-xs mono w-full"
            placeholder="https://api.openai.com/v1 (hoặc Ollama / Local LLM Proxy)"
          />
        </div>

        <div class="md:col-span-2">
          <label class="block text-[10px] text-gray-400 uppercase mb-1">Chỉ thị bổ sung cho AI (System Note)</label>
          <input
            v-model="config.system_note"
            class="pro-input text-xs w-full"
            placeholder="VD: Luôn trả lời bằng tiếng Việt ngắn gọn, ưu tiên lưu file vào /mnt/sda1..."
          />
        </div>
      </div>

      <div class="mt-3 flex flex-wrap items-center justify-between gap-2 pt-2.5 border-t border-white/10">
        <label class="flex items-center gap-2 text-xs text-gray-200 cursor-pointer">
          <input v-model="config.allow_exec" type="checkbox" class="rounded accent-sky-500" />
          <span>Cho phép AI thực thi lệnh hệ thống với quyền <code class="mono text-sky-300">root</code> (<code class="mono text-emerald-300">&lt;exec_cmd&gt;</code>) để kiểm tra và chỉnh sửa toàn bộ Router</span>
        </label>
        <div class="flex items-center gap-2">
          <button type="button" class="btn-secondary text-xs" :disabled="sending" @click="sendMessage('!uname -a && uptime && df -h')">
            ⚡ Test Root Shell Ngay
          </button>
          <button type="button" class="btn-primary text-xs px-4" :disabled="savingConfig" @click="saveConfiguration(false)">
            {{ savingConfig ? 'Đang lưu...' : '💾 Lưu Cấu Hình API' }}
          </button>
        </div>
      </div>
    </div>

    <!-- SECTION 2 (BELOW API): CHAT WINDOW & COMMAND CONSOLE -->
    <div class="pro-card flex-1 flex flex-col min-h-[380px]">
      <div class="pro-card-header shrink-0">
        <span class="pro-card-title text-emerald-400">💬 Cửa Sổ Giao Tiếp AI & Thực Thi Lệnh Root Trực Tiếp</span>
        <div class="flex items-center gap-1.5 overflow-x-auto">
          <button
            v-for="qp in quickPrompts"
            :key="qp.label"
            class="px-2 py-0.5 rounded bg-white/[0.05] hover:bg-sky-500/20 border border-white/10 hover:border-sky-500/40 text-[10.5px] text-gray-300 hover:text-sky-300 transition whitespace-nowrap cursor-pointer"
            :disabled="sending"
            @click="sendMessage(qp.text)"
          >
            {{ qp.label }}
          </button>
        </div>
      </div>

      <!-- Messages List -->
      <div ref="chatContainer" class="flex-1 overflow-y-auto p-3 space-y-3 bg-black/35 rounded border border-white/5 min-h-[260px]">
        <div v-if="history.length === 0" class="h-full flex flex-col items-center justify-center text-center py-6 text-gray-400">
          <div class="w-11 h-11 rounded-2xl bg-sky-500/15 border border-sky-500/30 flex items-center justify-center text-sky-400 font-bold text-base mb-2.5">
            AI
          </div>
          <h3 class="text-xs font-semibold text-gray-200 mb-1">Cửa Sổ Chat Trợ Lý Quản Trị Hệ Thống R3P (Full Root Access)</h3>
          <p class="text-[11px] text-gray-400 max-w-lg mb-3">
            Nhập API Key ở bảng cấu hình phía trên để trò chuyện với AI bằng ngôn ngữ tự nhiên, hoặc gõ <code class="mono text-emerald-300">!&lt;lệnh_shell&gt;</code> ở ô nhập bên dưới để chạy lệnh Root trực tiếp ngay lập tức.
          </p>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 max-w-xl w-full text-left">
            <div class="p-2.5 rounded-lg bg-black/40 border border-white/5 text-[11px]">
              <div class="font-semibold text-sky-300 mb-0.5">💬 Giao tiếp AI tự động thực thi lệnh</div>
              <div class="text-gray-400">Yêu cầu AI kiểm tra dung lượng ổ cứng, đổi IP LAN/WAN, bật AdBlock, hoặc tải video bằng yt-dlp.</div>
            </div>
            <div class="p-2.5 rounded-lg bg-black/40 border border-white/5 text-[11px]">
              <div class="font-semibold text-emerald-300 mb-0.5">⚡ Chạy lệnh Root tức thì (<code class="mono">!lệnh</code>)</div>
              <div class="text-gray-400">Gõ <code class="mono text-emerald-300">!df -h</code>, <code class="mono text-emerald-300">!uci show network</code> hoặc bấm các nút Lệnh Nhanh phía trên.</div>
            </div>
          </div>
        </div>

        <div
          v-for="(msg, idx) in history"
          :key="idx"
          class="flex flex-col"
          :class="msg.role === 'user' ? 'items-end' : 'items-start'"
        >
          <div class="flex items-center gap-1.5 mb-1 px-1">
            <span class="text-[10px] font-bold uppercase tracking-wider" :class="msg.role === 'user' ? 'text-sky-400' : 'text-purple-400'">
              {{ msg.role === 'user' ? 'BẠN (ROOT ADMIN)' : `R3P AI AGENT (${config.provider.toUpperCase()})` }}
            </span>
            <span class="text-[10px] text-gray-500 mono">{{ msg.timestamp }}</span>
          </div>

          <div
            class="max-w-4xl rounded-xl p-3 text-xs leading-relaxed"
            :class="msg.role === 'user'
              ? 'bg-sky-500/20 border border-sky-500/35 text-gray-100'
              : 'bg-[#111827]/95 border border-white/10 text-gray-200 w-full'"
          >
            <!-- Executed Root Commands Log -->
            <div v-if="getCmdLogs(msg).length > 0" class="mb-2.5 space-y-2">
              <div
                v-for="(cmd, cIdx) in getCmdLogs(msg)"
                :key="cIdx"
                class="rounded-lg bg-black/70 border border-emerald-500/30 overflow-hidden"
              >
                <div class="px-2.5 py-1 bg-emerald-500/10 border-b border-emerald-500/20 flex items-center justify-between text-[10.5px]">
                  <span class="mono text-emerald-300 font-semibold">root@R3P:~# {{ cmd.command }}</span>
                  <span class="mono text-[10px]" :class="cmd.exit_code === 0 ? 'text-emerald-400' : 'text-red-400'">
                    exit={{ cmd.exit_code }} · {{ cmd.duration_ms }}ms
                  </span>
                </div>
                <pre class="p-2.5 mono text-[10.5px] text-gray-300 overflow-x-auto max-h-52 whitespace-pre-wrap">{{ cmd.output }}</pre>
              </div>
            </div>

            <!-- Message Content -->
            <div class="whitespace-pre-wrap break-words font-sans text-[12px]">{{ msg.content }}</div>
          </div>
        </div>

        <div v-if="sending" class="flex items-center gap-2 text-xs text-sky-300 p-2">
          <span class="w-2 h-2 rounded-full bg-sky-400 animate-ping"></span>
          <span>AI đang phân tích hệ thống và thực thi lệnh trên Router...</span>
        </div>
      </div>

      <!-- Chat Input Form -->
      <form class="mt-2.5 flex items-center gap-2 shrink-0" @submit.prevent="sendMessage()">
        <input
          v-model="promptInput"
          class="pro-input text-xs flex-1 py-2"
          placeholder="Nhập câu hỏi cho AI (VD: 'Kiểm tra dung lượng USB và cấu hình WAN') hoặc gõ !lệnh_shell (VD: !ip route) để chạy trực tiếp..."
          :disabled="sending"
        />
        <button type="submit" class="btn-primary text-xs px-4 py-2" :disabled="sending || !promptInput.trim()">
          {{ sending ? 'Đang xử lý...' : '➤ Gửi / Thực Thi' }}
        </button>
      </form>
    </div>
  </div>
</template>
