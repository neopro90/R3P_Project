<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const token = ref('')
const submitting = ref(false)

async function submit() {
  if (!token.value || submitting.value) return
  submitting.value = true
  try {
    await auth.login(token.value)
    token.value = ''
  } catch {
    // The store exposes the API error to the form.
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="min-h-full flex items-center justify-center p-6">
    <form class="glass-card w-full max-w-md p-8 space-y-5" @submit.prevent="submit">
      <div>
        <p class="text-xs uppercase tracking-widest mb-2" style="color: var(--accent-blue);">R3P Management</p>
        <h1 class="text-2xl font-semibold text-white">Sign in</h1>
        <p class="text-sm mt-2 text-gray-400">
          Enter the API access token configured on the router. No default credential is provided.
        </p>
      </div>
      <label class="block space-y-2 text-sm text-gray-300" for="api-token">
        Access token
        <input
          id="api-token"
          v-model="token"
          type="password"
          autocomplete="current-password"
          required
          class="w-full rounded-lg border border-white/10 bg-black/20 px-3 py-2 text-white outline-none focus:border-sky-400"
        />
      </label>
      <p v-if="auth.error" role="alert" class="text-sm text-red-300">{{ auth.error }}</p>
      <button
        type="submit"
        :disabled="submitting || !token"
        class="w-full rounded-lg px-4 py-2 font-medium text-slate-950 disabled:cursor-not-allowed disabled:opacity-50"
        style="background: var(--accent-blue);"
      >
        {{ submitting ? 'Signing in…' : 'Sign in' }}
      </button>
      <p class="text-xs text-gray-500">Use HTTPS for the management API outside local development.</p>
    </form>
  </main>
</template>
