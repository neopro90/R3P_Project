<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const username = ref('root')
const password = ref('')
const submitting = ref(false)

async function submit() {
  if (submitting.value) return
  submitting.value = true
  try {
    await auth.login({ username: username.value, password: password.value })
  } catch {
    // Auth error shown by store
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="fixed inset-0 z-50 w-screen h-screen flex items-center justify-center p-4 bg-[#070a0f] overflow-y-auto">
    <div class="pro-card w-full max-w-[380px] p-6 bg-[#0f1422] border border-white/10 rounded-2xl shadow-2xl shrink-0">
      <div class="flex items-center gap-3 mb-5 pb-4 border-b border-white/10">
        <div class="w-9 h-9 rounded-lg bg-sky-500 flex items-center justify-center text-white font-bold text-sm tracking-wider shadow">
          R3
        </div>
        <div>
          <h1 class="text-sm font-bold text-white tracking-wide">R3P HOME SERVER OS</h1>
          <p class="text-[11px] text-gray-400">MediaTek MT7621AT · 512MB RAM · 256MB NAND</p>
        </div>
      </div>

      <form class="space-y-3.5" @submit.prevent="submit">
        <div>
          <label class="block text-[11px] font-semibold text-gray-400 uppercase tracking-wider mb-1" for="username">
            Username
          </label>
          <input
            id="username"
            v-model="username"
            type="text"
            autocomplete="username"
            required
            class="w-full pro-input mono"
            placeholder="root"
          />
        </div>

        <div>
          <label class="block text-[11px] font-semibold text-gray-400 uppercase tracking-wider mb-1" for="password">
            Password
          </label>
          <input
            id="password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            required
            class="w-full pro-input mono"
            placeholder="••••••••"
          />
        </div>

        <p v-if="auth.error" role="alert" class="text-xs text-red-400 bg-red-500/10 border border-red-500/20 px-2.5 py-1.5 rounded">
          {{ auth.error }}
        </p>

        <button
          type="submit"
          :disabled="submitting || !password"
          class="w-full btn-primary justify-center py-2 text-[12.5px] font-semibold disabled:opacity-50"
        >
          {{ submitting ? 'Authenticating…' : 'Sign In to Server' }}
        </button>
      </form>

      <div class="mt-4 pt-3 border-t border-white/5 flex items-center justify-between text-[11px] text-gray-500">
        <span>Default: <code class="mono text-gray-400">root / root</code></span>
        <span>OpenWrt 25.12.5</span>
      </div>
    </div>
  </main>
</template>
