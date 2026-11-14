import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { apiRequest, apiFetchRaw, apiURL, ApiError } from '@/services/api'

describe('api service client', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('builds /api/v1 URL paths consistently', () => {
    expect(apiURL('/system')).toMatch(/\/api\/v1\/system$/)
  })

  it('unwraps ok envelope responses with credentials included', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true, data: { hostname: 'R3P-Router' } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const result = await apiRequest<{ hostname: string }>('/system')
    expect(result.hostname).toBe('R3P-Router')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toMatch(/\/api\/v1\/system$/)
    expect(init.credentials).toBe('include')
  })

  it('dispatches r3p:unauthorized and throws ApiError even on non-JSON 401 response', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response('Unauthorized gateway proxy response', {
        status: 401,
        headers: { 'Content-Type': 'text/plain' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    let fired = 0
    const handler = () => {
      fired += 1
    }
    window.addEventListener('r3p:unauthorized', handler)
    try {
      await expect(apiRequest('/network/status')).rejects.toBeInstanceOf(ApiError)
      expect(fired).toBe(1)
    } finally {
      window.removeEventListener('r3p:unauthorized', handler)
    }
  })

  it('throws ApiError with server message when JSON envelope has ok:false', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: false, error: 'Validation rejected' }), {
        status: 400,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiRequest('/firewall', { method: 'POST', body: '{}' })).rejects.toMatchObject({
      name: 'ApiError',
      status: 400,
      message: 'Validation rejected',
    })
  })

  it('extracts message cleanly from object error payloads in ok:false responses', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: false, error: { message: 'Nested schema error', code: 422 } }), {
        status: 422,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiRequest('/config', { method: 'PUT', body: '{}' })).rejects.toMatchObject({
      name: 'ApiError',
      status: 422,
      message: 'Nested schema error',
    })
  })

  it('apiFetchRaw dispatches r3p:unauthorized on 401 and returns raw response on 200', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response('denied', { status: 401 }))
      .mockResolvedValueOnce(new Response('tarball-bytes', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    let fired = 0
    const handler = () => {
      fired += 1
    }
    window.addEventListener('r3p:unauthorized', handler)
    try {
      const r1 = await apiFetchRaw('/system/backup')
      expect(r1.status).toBe(401)
      expect(fired).toBe(1)

      const r2 = await apiFetchRaw('/system/backup')
      expect(r2.status).toBe(200)
      expect(await r2.text()).toBe('tarball-bytes')
      expect(fired).toBe(1)
    } finally {
      window.removeEventListener('r3p:unauthorized', handler)
    }
  })
})
