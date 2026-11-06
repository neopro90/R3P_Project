# Changelog — R3P Home Server OS

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [1.0.0-rc1] - 2026-11-06

### Added
- Phase 3 System Architecture deliverables: `docs/architecture/SYSTEM_ARCHITECTURE.md`, `docs/architecture/DATA_MODEL.md`, `docs/architecture/SECURITY_MODEL.md`, `docs/api/API_SPECIFICATION.md`.
- Phase 4 Resource Budget & Package Compatibility assessment: `reports/architecture/PACKAGE_COMPATIBILITY.md`.
- Phase 5 Firmware Build & Validation Report: `reports/build/BASELINE_BUILD_REPORT.md`.
- Phase 7 Frontend Build Quality Report: `reports/build/FRONTEND_TEST_REPORT.md`.
- Phase 10 Security Hardening Audit: `reports/architecture/SECURITY_AUDIT.md`.
- Phase 11 Disaster Recovery Plan: `deployment/recovery/RECOVERY_PLAN.md`.
- Production firmware release artifacts in `releases/v1.0.0-rc1/`.

### Fixed
- **LAN IPv4 Netmask Routing Bug:** Fixed first-boot configuration in `build/files/etc/uci-defaults/99-r3p-setup` to explicitly set `network.lan.netmask='255.255.255.0'` and `network.lan.ipaddr='192.168.2.1'`, preventing netifd from defaulting to `/32` host-only subnet.
- **Dropbear Dual-Stack Binding:** Removed `Interface='lan'` restriction so SSH binds dual-stack (`0.0.0.0:22` and `[::]:22`), allowing immediate access over IPv4 and IPv6.
- **Go API Server Dual-Stack Binding:** Configured `R3P_BIND=""` in procd init script to enable simultaneous IPv4 and IPv6 socket listening.
