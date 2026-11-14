import { describe, it, expect, beforeEach, vi } from 'vitest'

describe('useUiPreferences composable', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    delete document.documentElement.dataset.theme
    delete document.documentElement.dataset.accent
    delete document.documentElement.dataset.hass
    document.documentElement.lang = ''
  })

  it('recovers cleanly from corrupt localStorage JSON', async () => {
    localStorage.setItem('r3p.ui-preferences.v2', '{broken-json')
    const { useUiPreferences } = await import('@/composables/useUiPreferences')
    const prefs = useUiPreferences()

    expect(prefs.state.theme).toBe('dark')
    expect(prefs.state.accent).toBe('blue')
    expect(prefs.state.lang).toBe('vi')
    expect(prefs.state.hassStyle).toBe('hass-mushroom')
  })

  it('applies theme, accent, hassStyle, and language preferences to DOM', async () => {
    const { useUiPreferences, allowedAccents } = await import('@/composables/useUiPreferences')
    const prefs = useUiPreferences()

    prefs.toggleTheme()
    expect(prefs.state.theme).toBe('light')
    expect(document.documentElement.dataset.theme).toBe('light')

    prefs.setAccent('teal')
    expect(prefs.state.accent).toBe('teal')
    expect(document.documentElement.dataset.accent).toBe('teal')

    const prevAccent = prefs.state.accent
    prefs.cycleAccent()
    expect(allowedAccents).toContain(prefs.state.accent)
    expect(prefs.state.accent).not.toBe(prevAccent)

    prefs.setHassStyle('hass-neon')
    expect(document.documentElement.dataset.hass).toBe('hass-neon')

    prefs.cycleLang()
    expect(prefs.state.lang).toBe('en')
    expect(document.documentElement.lang).toBe('en')
    prefs.cycleLang()
    expect(prefs.state.lang).toBe('cn')
    expect(document.documentElement.lang).toBe('zh-CN')

    prefs.setLang('en')
    expect(prefs.t('nav_dashboard')).toBe('Dashboard')
    prefs.setLang('cn')
    expect(prefs.t('nav_dashboard')).toBe('控制面板 (Dashboard)')
    prefs.setLang('vi')
    expect(prefs.t('nav_dashboard')).toBe('Bảng Điều Khiển')
    expect(prefs.t('nonexistent_translation_key')).toBe('nonexistent_translation_key')
  })

  it('maintains full non-empty VI, EN, and CN translation parity', async () => {
    const { translations } = await import('@/composables/useUiPreferences')
    const viKeys = Object.keys(translations.vi)
    expect(viKeys.length).toBeGreaterThan(80)
    for (const key of viKeys) {
      expect(translations.vi[key]?.trim().length).toBeGreaterThan(0)
      expect(translations.en[key]?.trim().length).toBeGreaterThan(0)
      expect(translations.cn[key]?.trim().length).toBeGreaterThan(0)
    }
  })

  it('hydrates and persists preferences via /api/v1/ui/preferences', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ok: true,
          data: {
            theme: 'light',
            sidebar_collapsed: true,
            updated_at: '2026-11-14T00:00:00Z',
            preferences: {
              accent: 'teal',
              lang: 'en',
              hassStyle: 'hass-neon',
            },
          },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    )
    vi.stubGlobal('fetch', fetchMock)

    const { useUiPreferences } = await import('@/composables/useUiPreferences')
    const prefs = useUiPreferences()
    await prefs.syncFromServer()

    expect(prefs.state.theme).toBe('light')
    expect(prefs.state.sidebarCollapsed).toBe(true)
    expect(prefs.state.accent).toBe('teal')
    expect(prefs.state.lang).toBe('en')
    expect(prefs.state.hassStyle).toBe('hass-neon')
    expect(document.documentElement.dataset.theme).toBe('light')
    prefs.setServerSync(false)
    vi.unstubAllGlobals()
  })
})
