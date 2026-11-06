# SOURCE FILE INDEX — R3P HOME SERVER OS

## 1. Backend Source Tree (`source/backend/`)
```
source/backend/
├── aria2.go           - Aria2 JSON-RPC adapter & process controller
├── aria2_test.go      - Unit tests for Aria2 interface
├── auth.go            - Token authentication & session lifecycle manager
├── auth_test.go       - Unit tests for constant-time auth & session expiry
├── cert.go            - Embedded TLS X.509 certificate generator
├── cert_test.go       - Tests for self-signed cert generation
├── files_test.go      - Storage and photo safety tests
├── go.mod             - Go module definition (standard library only, zero external deps)
├── main.go            - Primary API router, HTTP server & system handlers
├── metrics_test.go    - Telemetry & procfs parsing tests
├── photo.go           - Photo thumbnailing & media gallery backend
├── photo_test.go      - Media handling unit tests
├── telegram.go        - Notification bridge & webhook handler
└── README.md          - Backend architecture and build guide
```

## 2. Frontend Source Tree (`source/frontend/`)
```
source/frontend/
├── package.json       - npm dependencies & scripts (Vue 3, Vite, TypeScript)
├── tsconfig.json      - Strict TypeScript compiler options
├── vite.config.ts     - Build configuration, alias mapping & proxy setup
├── index.html         - Single-page application root entrypoint
└── src/
    ├── main.ts        - Vue 3 bootstrap & router mounting
    ├── App.vue        - Root component with theme & toast providers
    ├── router/        - Vue Router 4 view definitions (13 core tabs)
    ├── components/    - Reusable Glassmorphism UI components (Cards, Gauges, Tables)
    ├── views/         - Tab views (Dashboard, Network, Wireless, Firewall, Storage, etc.)
    └── assets/        - SVG icons, styling, and typography
```

## 3. Deployment Scripts (`deployment/scripts/`)
```
deployment/scripts/
├── build-custom.sh    - Custom firmware generation wrapper
├── verify-custom.sh   - Image integrity & sysupgrade safety validator
├── wsl-setup.sh       - WSL 2 build runtime bootstrap script
├── flash-router.sh    - Staged sysupgrade deployment automation
├── check_luci.py      - Verification script ensuring no LuCI residue
├── inspect_router.py  - Remote telemetry & hardware audit helper
├── scan_ipv6.py       - Dual-stack socket availability scanner
└── test_auth.py       - API session login & token acceptance test
```

## 4. Hardware Reference Logs (`root`)
```
├── OEM bootlog.md     - Original Xiaomi stock firmware serial bootlog
├── OpenWrt bootlog.md - Early OpenWrt serial console output
├── Hardware.md        - Xiaomi Mi Router 3 Pro physical hardware specification
└── Flash Layout.md    - NAND partition table map (mtd0 - mtd13)
```
