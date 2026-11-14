export interface ApiErrorPayload {
  message?: string
  error?: string
  detail?: string
}

export interface ApiEnvelope<T> {
  ok: boolean
  data?: T
  error?: string | ApiErrorPayload
}

export class ApiError extends Error {
  readonly status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

const configuredBase = import.meta.env.VITE_API_BASE?.trim().replace(/\/+$/, '')
const apiOrigin = configuredBase || ''

export const apiURL = (path: string) => `${apiOrigin}/api/v1${path}`

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  const isBinaryBody =
    init.body instanceof FormData ||
    init.body instanceof Blob ||
    init.body instanceof ArrayBuffer
  if (init.body !== undefined && !isBinaryBody && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  let response: Response
  try {
    response = await fetch(apiURL(path), {
      ...init,
      headers,
      credentials: 'include',
    })
  } catch {
    throw new ApiError('Cannot reach the R3P API. Check the router connection.', 0)
  }

  if (response.status === 401) {
    window.dispatchEvent(new Event('r3p:unauthorized'))
  }

  let envelope: ApiEnvelope<T>
  try {
    envelope = (await response.json()) as ApiEnvelope<T>
  } catch {
    throw new ApiError(`The API returned an invalid response (HTTP ${response.status}).`, response.status)
  }

  if (!response.ok || !envelope.ok || envelope.data === undefined) {
    let message = `API request failed (HTTP ${response.status}).`
    if (typeof envelope.error === 'string' && envelope.error.trim()) {
      message = envelope.error
    } else if (envelope.error && typeof envelope.error === 'object') {
      const candidate = envelope.error.message ?? envelope.error.error ?? envelope.error.detail
      if (typeof candidate === 'string' && candidate.trim()) {
        message = candidate
      }
    }
    throw new ApiError(message, response.status)
  }
  return envelope.data
}

export async function apiFetchRaw(path: string, init: RequestInit = {}): Promise<Response> {
  let response: Response
  try {
    response = await fetch(apiURL(path), {
      ...init,
      credentials: 'include',
    })
  } catch {
    throw new ApiError('Cannot reach the R3P API. Check the router connection.', 0)
  }
  if (response.status === 401) {
    window.dispatchEvent(new Event('r3p:unauthorized'))
  }
  return response
}
