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
export type LangCode = 'vi' | 'en' | 'cn'
export type HassThemeStyle = 'hass-classic' | 'hass-mushroom' | 'hass-neon'

interface Preferences {
  widgets: DashboardWidgetId[]
  hiddenWidgets: DashboardWidgetId[]
  theme: ThemeMode
  accent: AccentColor
  lang: LangCode
  hassStyle: HassThemeStyle
}

const storageKey = 'r3p.ui-preferences.v2'
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
const allowedLangs: LangCode[] = ['vi', 'en', 'cn']
const allowedHassStyles: HassThemeStyle[] = ['hass-classic', 'hass-mushroom', 'hass-neon']

const defaults: Preferences = {
  widgets: [...allowedWidgets],
  hiddenWidgets: [],
  theme: 'dark',
  accent: 'blue',
  lang: 'vi',
  hassStyle: 'hass-mushroom',
}

export const translations: Record<LangCode, Record<string, string>> = {
  vi: {
    group_overview: 'Tổng Quan & AI',
    group_security: 'Bảo Mật & Truy Cập',
    group_services: 'Dịch Vụ & Lưu Trữ',
    group_maintenance: 'Hệ Thống & Bảo Trì',
    nav_dashboard: 'Bảng Điều Khiển',
    nav_ai: 'Trợ Lý AI (Root)',
    nav_network: 'Mạng (LAN/WAN)',
    nav_wireless: 'Wi-Fi (AP & Thu Sóng)',
    nav_adblock: 'Chặn Quảng Cáo DNS',
    nav_firewall: 'Tường Lửa & NAT',
    nav_access: 'Tài Khoản & DHCP',
    nav_config: 'Cấu Hình UCI',
    nav_downloads: 'Download Bot (ESP32)',
    nav_nas: 'NAS & USB 3.0',
    nav_explorer: 'Duyệt Tệp & Thư Mục',
    nav_apps: 'Kho Ứng Dụng',
    nav_automation: 'Tự Động Hóa & Cron',
    nav_iot: 'Thiết Bị IoT (MQTT)',
    nav_plugins: 'Tiện Ích Mở Rộng',
    nav_system: 'Hệ Thống & Firmware',
    title_dashboard: 'Tổng Quan Hệ Thống Hass.io R3P',
    title_ai: 'Trợ Lý AI Quản Trị Hệ Thống (Full Root & API)',
    title_network: 'Giám Sát & Cấu Hình Mạng LAN / WAN / Gateway',
    title_wireless: 'Trạm Phát Wi-Fi Dual-Band & Thu Wi-Fi Client (MT7615N)',
    title_adblock: 'Lá Chắn Chặn Quảng Cáo & Mã Độc Toàn Mạng (AdBlock DNS)',
    title_firewall: 'Quản Lý Tường Lửa, Cổng & NAT',
    title_access: 'Quản Lý Truy Cập, SSH & Máy Chủ DHCP',
    title_config: 'Trình Chỉnh Sửa Cấu Hình Hệ Thống (UCI)',
    title_downloads: 'Trung Tâm Tải Xuống ESP32-Download Localhost',
    title_nas: 'Lưu Trữ NAS, Format Ổ Cứng & Chia Sẻ USB 3.0',
    title_explorer: 'Quản Lý Tệp Hệ Thống & Cây Thư Mục USB (Trình Soạn Thảo)',
    title_apps: 'Kho Ứng Dụng OpenWrt & GitHub Repositories',
    title_automation: 'Lập Lịch Tự Động Hóa & Tiến Trình Cron',
    title_iot: 'Trung Tâm Điều Khiển Nhà Thông Minh IoT (MQTT)',
    title_plugins: 'Quản Lý Plugins Hệ Thống',
    title_system: 'Cập Nhật Firmware (GitHub / Thủ Công) & Nhật Ký Hệ Thống',
    internet_online: 'INTERNET: TRỰC TUYẾN',
    internet_offline: 'INTERNET: NGOẠI TUYẾN',
    checking: 'ĐANG KIỂM TRA...',
    ext_ip: 'IP NGOÀI',
    uptime: 'HOẠT ĐỘNG',
    copied: 'ĐÃ COPY!',
    r3p_link: 'KẾT NỐI R3P',
    disconnected: 'MẤT KẾT NỐI',
    theme_dark: 'Tối',
    theme_light: 'Sáng',
    hass_classic: 'Hass Lovelace',
    hass_mushroom: 'Hass Mushroom',
    hass_neon: 'Hass Energy',
  },
  en: {
    group_overview: 'Overview & AI',
    group_security: 'Security & Access',
    group_services: 'Services & Storage',
    group_maintenance: 'Maintenance',
    nav_dashboard: 'Dashboard',
    nav_ai: 'AI Assistant (Root)',
    nav_network: 'Network (LAN/WAN)',
    nav_wireless: 'Wireless (Wi-Fi)',
    nav_adblock: 'AdBlock DNS',
    nav_firewall: 'Firewall & NAT',
    nav_access: 'Access & Accounts',
    nav_config: 'Config (UCI)',
    nav_downloads: 'Download Bot (ESP32)',
    nav_nas: 'NAS & USB 3.0',
    nav_explorer: 'File Explorer & Tree',
    nav_apps: 'Applications',
    nav_automation: 'Automation & Cron',
    nav_iot: 'IoT (MQTT)',
    nav_plugins: 'Plugins',
    nav_system: 'System & Firmware',
    title_dashboard: 'Hass.io R3P Dashboard Overview',
    title_ai: 'R3P Root AI System Agent (API & Chat Console)',
    title_network: 'Network Monitor & LAN/WAN/Gateway Configuration',
    title_wireless: 'Wireless AP & Wi-Fi Client (MT7615N 4x4)',
    title_adblock: 'AdBlock DNS Shield & Gateway Setup Guide',
    title_firewall: 'Firewall & NAT Rules',
    title_access: 'Access, SSH & DHCP Server',
    title_config: 'System Configuration (UCI)',
    title_downloads: 'ESP32-Download Localhost Control Plane',
    title_nas: 'NAS Storage, Disk Format & USB 3.0 Dual-Role',
    title_explorer: 'System & USB Storage File Explorer (Code & Config Editor)',
    title_apps: 'OpenWrt Applications & GitHub Repositories',
    title_automation: 'Automation & Cron Scheduler',
    title_iot: 'IoT Smart Home Broker (MQTT)',
    title_plugins: 'Plugins Manager',
    title_system: 'Firmware Update (GitHub / Manual) & System Logs',
    internet_online: 'INTERNET: ONLINE',
    internet_offline: 'INTERNET: OFFLINE',
    checking: 'CHECKING...',
    ext_ip: 'EXT IP',
    uptime: 'UPTIME',
    copied: 'COPIED!',
    r3p_link: 'R3P LINK',
    disconnected: 'DISCONNECTED',
    theme_dark: 'Dark',
    theme_light: 'Light',
    hass_classic: 'Hass Lovelace',
    hass_mushroom: 'Hass Mushroom',
    hass_neon: 'Hass Energy',
  },
  cn: {
    group_overview: '总览与 AI 助手',
    group_security: '安全与访问控制',
    group_services: '服务与存储中心',
    group_maintenance: '系统与固件维护',
    nav_dashboard: '控制面板 (Dashboard)',
    nav_ai: 'AI 系统助手 (Root)',
    nav_network: '网络配置 (LAN/WAN)',
    nav_wireless: '无线网络 (2.4G/5G)',
    nav_adblock: '广告拦截 (AdBlock)',
    nav_firewall: '防火墙与端口转发',
    nav_access: '账户与 DHCP 服务',
    nav_config: 'UCI 系统配置',
    nav_downloads: '下载中心 (ESP32)',
    nav_nas: 'NAS 与 USB 3.0',
    nav_explorer: '文件管理与目录树',
    nav_apps: 'OpenWrt 应用商店',
    nav_automation: '自动化与定时任务',
    nav_iot: '物联网中心 (MQTT)',
    nav_plugins: '插件管理',
    nav_system: '固件更新与系统日志',
    title_dashboard: 'Hass.io R3P 智能家居与路由总览',
    title_ai: 'R3P Root AI 智能中枢 (API 与终端对话)',
    title_network: '网络监控与 LAN / WAN / 网关设置',
    title_wireless: '双频 Wi-Fi 发射与无线中继客户端 (MT7615N)',
    title_adblock: '全网 DNS 广告拦截与主网关 DNS 设置指南',
    title_firewall: '防火墙与 NAT 转发规则',
    title_access: '访问控制、SSH 与 DHCP 服务器',
    title_config: 'UCI 底层配置编辑器',
    title_downloads: 'ESP32-Download 本地下载控制中心',
    title_nas: 'NAS 存储、USB 3.0 格式化与网络共享',
    title_explorer: '系统与 USB 文件树管理器 (文本与脚本编辑器)',
    title_apps: 'OpenWrt 软件包与 GitHub 仓库管理',
    title_automation: '自动化任务与 Cron 计划',
    title_iot: 'IoT 智能设备管理 (MQTT)',
    title_plugins: '系统扩展插件',
    title_system: '固件升级 (GitHub 在线 / 手动上传) 与系统日志',
    internet_online: '互联网: 已连接',
    internet_offline: '互联网: 未连接',
    checking: '检测中...',
    ext_ip: '公网 IP',
    uptime: '运行时间',
    copied: '已复制!',
    r3p_link: '路由已连接',
    disconnected: '已断开',
    theme_dark: '深色',
    theme_light: '浅色',
    hass_classic: 'Hass 经典蓝',
    hass_mushroom: 'Hass 蘑菇卡片',
    hass_neon: 'Hass 能源霓虹',
  },
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
      theme: allowedThemes.includes(record.theme as ThemeMode) ? (record.theme as ThemeMode) : 'dark',
      accent: allowedAccents.includes(record.accent as AccentColor) ? (record.accent as AccentColor) : 'blue',
      lang: allowedLangs.includes(record.lang as LangCode) ? (record.lang as LangCode) : 'vi',
      hassStyle: allowedHassStyles.includes(record.hassStyle as HassThemeStyle)
        ? (record.hassStyle as HassThemeStyle)
        : 'hass-mushroom',
    }
  } catch (cause) {
    console.warn('Could not load R3P UI preferences; using defaults.', cause)
    return { ...defaults, widgets: [...defaults.widgets], hiddenWidgets: [] }
  }
}

const state = reactive<Preferences>(loadPreferences())

watch(
  state,
  (preferences) => {
    try {
      localStorage.setItem(storageKey, JSON.stringify(preferences))
    } catch (cause) {
      console.error('Could not save R3P UI preferences.', cause)
    }
  },
  { deep: true },
)

export function useUiPreferences() {
  function moveWidget(id: DashboardWidgetId, offset: -1 | 1) {
    const current = state.widgets.indexOf(id)
    const next = current + offset
    if (current < 0 || next < 0 || next >= state.widgets.length) return
    const widgets = [...state.widgets]
    ;[widgets[current], widgets[next]] = [widgets[next]!, widgets[current]!]
    state.widgets = widgets
  }

  function reset() {
    Object.assign(state, { ...defaults, widgets: [...defaults.widgets], hiddenWidgets: [] })
  }

  function toggleTheme() {
    state.theme = state.theme === 'light' ? 'dark' : 'light'
    applyTheme()
  }

  function cycleLang() {
    const order: LangCode[] = ['vi', 'en', 'cn']
    const idx = order.indexOf(state.lang)
    state.lang = order[(idx + 1) % order.length] || 'vi'
    applyTheme()
  }

  function setLang(l: LangCode) {
    state.lang = l
    applyTheme()
  }

  function setHassStyle(style: HassThemeStyle) {
    state.hassStyle = style
    applyTheme()
  }

  function cycleHassStyle() {
    const idx = allowedHassStyles.indexOf(state.hassStyle)
    state.hassStyle = allowedHassStyles[(idx + 1) % allowedHassStyles.length] || 'hass-mushroom'
    applyTheme()
  }

  function t(key: string): string {
    return translations[state.lang]?.[key] || translations.vi[key] || key
  }

  function applyTheme() {
    const root = document.documentElement
    const mode =
      state.theme === 'system'
        ? window.matchMedia('(prefers-color-scheme: light)').matches
          ? 'light'
          : 'dark'
        : state.theme
    root.dataset.theme = mode
    root.dataset.hass = state.hassStyle
    root.lang = state.lang === 'cn' ? 'zh-CN' : state.lang
  }

  return {
    state,
    moveWidget,
    reset,
    toggleTheme,
    cycleLang,
    setLang,
    setHassStyle,
    cycleHassStyle,
    t,
    applyTheme,
  }
}
