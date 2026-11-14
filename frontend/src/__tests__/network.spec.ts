import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useNetworkStore } from '@/stores/network'

describe('Pinia useNetworkStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('refreshes interfaces, classifies interface types, and computes WAN IP & rates', async () => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/network')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              ok: true,
              data: {
                interfaces: [
                  {
                    iface: 'br-lan',
                    rx_bytes: 1048576,
                    tx_bytes: 524288,
                    rx_bytes_per_second: 1200,
                    tx_bytes_per_second: 600,
                    ipv4: '192.168.2.1',
                    ipv6: '',
                    mac: '00:11:22:33:44:55',
                    up: true,
                  },
                  {
                    iface: 'pppoe-wan',
                    rx_bytes: 2097152,
                    tx_bytes: 1048576,
                    rx_bytes_per_second: 4800,
                    tx_bytes_per_second: 1400,
                    ipv4: '203.0.113.10',
                    ipv6: '',
                    mac: 'aa:bb:cc:dd:ee:ff',
                    up: true,
                  },
                ],
              },
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        )
      }
      if (url.endsWith('/api/v1/clients')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              ok: true,
              data: {
                leases: [
                  {
                    ip: '192.168.2.100',
                    mac: '11:22:33:44:55:66',
                    hostname: 'workstation',
                    lease_active: true,
                    expiry: 1999999999,
                  },
                  {
                    ip: '192.168.2.101',
                    mac: '11:22:33:44:55:77',
                    hostname: 'stale-client',
                    lease_active: false,
                    expiry: 1000,
                  },
                ],
              },
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        )
      }
      return Promise.resolve(new Response('not found', { status: 404 }))
    })
    vi.stubGlobal('fetch', fetchMock)

    const net = useNetworkStore()
    await net.refreshInterfaces()
    await net.refreshClients()

    expect(net.interfaces).toHaveLength(2)
    expect(net.interfaces[0]?.type).toBe('lan')
    expect(net.interfaces[1]?.type).toBe('wan')
    expect(net.wanIface?.name).toBe('pppoe-wan')
    expect(net.wanIp).toBe('203.0.113.10')
    expect(net.totalRx).toBe(1048576 + 2097152)
    expect(net.totalTx).toBe(524288 + 1048576)

    expect(net.clientsAvailable).toBe(true)
    expect(net.clientCount).toBe(1)

    net.updateTrafficRates(25000, 12000)
    expect(net.trafficAvailable).toBe(true)
    expect(net.totalRxRate).toBe(25000)
    expect(net.totalTxRate).toBe(12000)
    expect(net.rxHistory.at(-1)).toBe(25000)
    expect(net.txHistory.at(-1)).toBe(12000)
    expect(net.formatBytes(1048576)).toContain('MB')
  })
})
