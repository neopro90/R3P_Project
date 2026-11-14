<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useUiPreferences } from '@/composables/useUiPreferences'

const auth = useAuthStore()
const { t } = useUiPreferences()
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
    <!-- Card is strictly constrained to 380px via inline style & login-card class -->
    <div
      class="pro-card login-card p-6 bg-[#0f1422] border border-white/10 rounded-2xl shadow-2xl shrink-0"
      style="max-width: 380px !important; width: 100% !important; margin: auto !important;"
    >
      <div class="flex items-center gap-3 mb-5 pb-4 border-b border-white/10">
        <div class="w-10 h-10 rounded-xl bg-gradient-to-tr from-sky-600 to-cyan-400 flex items-center justify-center keep-white font-black text-base tracking-wider shadow-lg shrink-0">
          R3
        </div>
        <div class="min-w-0">
          <h1 class="text-sm font-bold text-white tracking-wide truncate">R3P HOME SERVER OS</h1>
          <p class="text-[11px] text-gray-400 truncate">{{ t('login_subtitle') }}</p>
        </div>
      </div>

      <form class="space-y-4" @submit.prevent="submit">
        <div>
          <label class="block text-[11px] font-semibold text-gray-400 uppercase tracking-wider mb-1.5" for="username">
            {{ t('login_username') }}
          </label>
          <input
            id="username"
            v-model="username"
            type="text"
            autocomplete="username"
            required
            class="w-full pro-input mono text-sm py-2 px-3"
            placeholder="root"
          />
        </div>

        <div>
          <label class="block text-[11px] font-semibold text-gray-400 uppercase tracking-wider mb-1.5" for="password">
            {{ t('login_password') }}
          </label>
          <input
            id="password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            required
            class="w-full pro-input mono text-sm py-2 px-3"
            placeholder="••••••••"
          />
        </div>

        <p v-if="auth.error" role="alert" class="text-xs text-red-400 bg-red-500/10 border border-red-500/20 px-2.5 py-1.5 rounded">
          {{ auth.error }}
        </p>

        <button
          type="submit"
          :disabled="submitting || !password"
          class="w-full btn-primary justify-center py-2.5 text-[13px] font-bold disabled:opacity-50 shadow-md cursor-pointer transition-all"
        >
          {{ submitting ? t('login_submitting') : t('login_submit') }}
        </button>
      </form>

      <div class="mt-4 pt-3 border-t border-white/5 flex items-center justify-between text-[11px] text-gray-400">
        <span>{{ t('login_hint') }}</span>
        <span class="mono text-sky-400">R3P · OpenWrt</span>
      </div>
    </div>
  </main>
</template>
