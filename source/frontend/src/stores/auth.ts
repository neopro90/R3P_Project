import { defineStore } from 'pinia'
import { ref } from 'vue'
import { ApiError, apiRequest } from '@/services/api'

interface SessionStatus {
  authenticated: boolean
}

export const useAuthStore = defineStore('auth', () => {
  const authenticated = ref(false)
  const checking = ref(true)
  const error = ref<string | null>(null)

  async function checkSession() {
    checking.value = true
    try {
      const session = await apiRequest<SessionStatus>('/auth/session')
      authenticated.value = session.authenticated
      error.value = null
    } catch (cause) {
      authenticated.value = false
      error.value = cause instanceof ApiError && cause.status === 401
        ? null
        : cause instanceof Error ? cause.message : 'Could not verify the router session.'
    } finally {
      checking.value = false
    }
  }

  async function login(token: string) {
    error.value = null
    try {
      await apiRequest<SessionStatus>('/auth/session', {
        method: 'POST',
        body: JSON.stringify({ token }),
      })
      authenticated.value = true
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : 'Sign-in failed.'
      throw cause
    }
  }

  async function logout() {
    error.value = null
    try {
      await apiRequest<SessionStatus>('/auth/session', { method: 'DELETE' })
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : 'Sign-out failed.'
      throw cause
    } finally {
      authenticated.value = false
    }
  }

  function clearSession() {
    authenticated.value = false
  }

  return { authenticated, checking, error, checkSession, login, logout, clearSession }
})
