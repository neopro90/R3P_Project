import { reactive, watch } from 'vue'

export type DashboardWidgetId =
  | 'cpu'
  | 'memory'
  | 'network'
  | 'clients'
  | 'topology'
  | 'interfaces'

export type ThemeMode = 'dark' | 'light' | 'system'
export type AccentColor = 'blue' | 'teal' | 'purple' | 'green' | 'orange'

interface Preferences {
  widgets: DashboardWidgetId[]
  hiddenWidgets: DashboardWidgetId[]
  theme: ThemeMode
  accent: AccentColor
}

const storageKey = 'r3p.ui-preferences.v1'
const allowedWidgets: DashboardWidgetId[] = [
  'cpu',
  'memory',
  'network',
  'clients',
  'topology',
  'interfaces',
]
const allowedThemes: ThemeMode[] = ['dark', 'light', 'system']
const allowedAccents: AccentColor[] = ['blue', 'teal', 'purple', 'green', 'orange']

const defaults: Preferences = {
  widgets: [...allowedWidgets],
  hiddenWidgets: [],
  theme: 'dark',
  accent: 'blue',
}

function loadPreferences(): Preferences {
  try {
    const raw = localStorage.getItem(storageKey)
    if (!raw) return { ...defaults, widgets: [...defaults.widgets], hiddenWidgets: [] }

    const saved: unknown = JSON.parse(raw)
    if (!saved || typeof saved !== 'object') return { ...defaults, widgets: [...defaults.widgets], hiddenWidgets: [] }

    const record = saved as Partial<Record<keyof Preferences, unknown>>
    const widgets = Array.isArray(record.widgets)
      ? record.widgets.filter((id): id is DashboardWidgetId =>
          typeof id === 'string' && allowedWidgets.includes(id as DashboardWidgetId),
        )
      : []
    const hiddenWidgets = Array.isArray(record.hiddenWidgets)
      ? record.hiddenWidgets.filter((id): id is DashboardWidgetId =>
          typeof id === 'string' && allowedWidgets.includes(id as DashboardWidgetId),
        )
      : []
    const ordered = [...new Set([...widgets, ...allowedWidgets])]
    return {
      widgets: ordered,
      hiddenWidgets: [...new Set(hiddenWidgets)],
      theme: allowedThemes.includes(record.theme as ThemeMode) ? record.theme as ThemeMode : 'dark',
      accent: allowedAccents.includes(record.accent as AccentColor) ? record.accent as AccentColor : 'blue',
    }
  } catch (cause) {
    console.warn('Could not load R3P UI preferences; using defaults.', cause)
    return { ...defaults, widgets: [...defaults.widgets], hiddenWidgets: [] }
  }
}

const state = reactive<Preferences>(loadPreferences())

watch(state, (preferences) => {
  try {
    localStorage.setItem(storageKey, JSON.stringify(preferences))
  } catch (cause) {
    console.error('Could not save R3P UI preferences.', cause)
  }
}, { deep: true })

export function useUiPreferences() {
  function moveWidget(id: DashboardWidgetId, offset: -1 | 1) {
    const current = state.widgets.indexOf(id)
    const next = current + offset
    if (current < 0 || next < 0 || next >= state.widgets.length) return
    const widgets = [...state.widgets]
    ;[widgets[current], widgets[next]] = [widgets[next], widgets[current]]
    state.widgets = widgets
  }

  function reset() {
    Object.assign(state, { ...defaults, widgets: [...defaults.widgets], hiddenWidgets: [] })
  }

  function applyTheme() {
    const root = document.documentElement
    const mode = state.theme === 'system'
      ? (window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark')
      : state.theme
    root.dataset.theme = mode
    root.style.setProperty('--accent-primary', `var(--accent-${state.accent})`)
  }

  return { state, moveWidget, reset, applyTheme }
}
