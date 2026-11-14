import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useSystemStore } from '@/stores/system'
import { useAuthStore } from '@/stores/auth'

describe('Pinia system & auth stores', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('populates useSystemStore from SSE updates and /system + /memory refresh', async () => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/system')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              ok: true,
              data: {
                hostname: 'R3P-Home',
                kernel: '6.6.100',
                arch: 'mipsle',
                version: 'v1.1.25',
                hardware: 'Xiaomi Mi Router 3 Pro',
                lan_ip: '192.168.2.1',
                timezone: 'ICT-7',
                firmware_version: 'OpenWrt R3P',
                openwrt: { DISTRIB_RELEASE: '25.12' },
                cpu_aggregate_pct: 12.5,
                uptime_seconds: 3600,
                load_avg: { load1: 0.25, load5: 0.2, load15: 0.15 },
                temperatures: [{ label: 'SoC', temp_celsius: 51.2 }],
              },
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        )
      }
      if (url.endsWith('/api/v1/memory')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              ok: true,
              data: {
                total_kb: 442704,
                physical_total_kb: 524288,
                reserved_kb: 81584,
                used_kb: 110676,
                used_percent: 25.0,
              },
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        )
      }
      return Promise.resolve(new Response('not found', { status: 404 }))
    })
    vi.stubGlobal('fetch', fetchMock)

    const system = useSystemStore()
    await system.refresh()

    expect(system.hostname).toBe('R3P-Home')
    expect(system.lanIp).toBe('192.168.2.1')
    expect(system.timezone).toBe('ICT-7')
    expect(system.appVersion).toBe('v1.1.25')
    expect(system.ramPhysical).toBe(512)
    expect(system.ramTotal).toBe(432)
    expect(system.ramReserved).toBe(80)
    expect(system.ramUsed).toBe(108)
    expect(system.ram).toBe(25)
    expect(system.temp).toBe(51.2)

    const beforeTs = Date.now()
    system.updateFromSSE({
      cpu: 42.5,
      uptime: 7200,
    })
    expect(system.cpu).toBe(42.5)
    expect(system.uptime).toBe(7200)
    expect(system.connected).toBe(true)
    expect(system.lastUpdate).toBeGreaterThanOrEqual(beforeTs)
  })

  it('handles login, session check 401 without noisy error, logout, and clearSession in useAuthStore', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ok: false, error: 'Unauthorized' }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ok: true, data: { authenticated: true } }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ok: true, data: { authenticated: false } }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    vi.stubGlobal('fetch', fetchMock)

    const auth = useAuthStore()
    await auth.checkSession()
    expect(auth.authenticated).toBe(false)
    expect(auth.checking).toBe(false)
    expect(auth.error).toBeNull()

    await auth.login({ username: 'root', password: 'secret' })
    expect(auth.authenticated).toBe(true)
    expect(auth.error).toBeNull()

    await auth.logout()
    expect(auth.authenticated).toBe(false)

    auth.authenticated = true
    auth.clearSession()
    expect(auth.authenticated).toBe(false)
  })
})
