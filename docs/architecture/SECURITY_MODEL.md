# SECURITY MODEL — R3P HOME SERVER OS (`v1.1.25`)

## 1. Core Principles
1. **LAN Management & Explicit TLS (`:8443`):** The firmware init script generates a self-signed ECDSA certificate/key (`/etc/r3p/api.crt`, `/etc/r3p/api.key` mode `0600`) and serves both the SPA and `/api/v1/*` over TLS 1.2+ on port `8443` without plain HTTP fallback when TLS files are set.
2. **Same-Host CORS & Authenticated API:** Only `GET /api/v1/health` and `GET|POST /api/v1/auth/session` are accessible unauthenticated. Same-host origin checks block third-party cross-origin requests.
3. **Structured Input Validation & Secret Redaction:** Config endpoints (`network`, `firewall`, `access`, `adblock`, `iot`, `plugins`, `automation`, `config`) validate IPv4/netmask/port ranges/MAC/cron/preset allowlists via typed Go handlers (`exec.Command` argument arrays), create `.bak` snapshots before committing UCI changes, and roll back automatically on UCI syntax or commit errors.
4. **Admin AI Execution Control:** `AI Assistant` terminal/tool execution is strictly gated by server-side `auto_execute && allow_exec` configuration (`ai_config.json`) behind an authenticated admin session; requests can only disable execution, never escalate privileges above server policy.

---

## 2. Authentication & Session Management
- **Credentials Supported on `POST /api/v1/auth/session`:**
  - Linux `/etc/shadow` password verification (SHA-256 `$5$` / SHA-512 `$6$` crypt vectors implemented in `crypt_shadow.go`) with zero hardcoded default fallback credentials.
  - Optional 256-bit API access token (`/etc/r3p/api-token` mode `0600` or `R3P_API_TOKEN`) compared in constant time via `crypto/subtle.ConstantTimeCompare`.
- **Sessions & Cookies:**
  - Valid logins issue an `HttpOnly`, `SameSite=Strict` session cookie (`r3p_session`, `Secure` flag added on HTTPS connections) with 8-hour expiration.
  - Consecutive failed login attempts trigger per-IP HTTP `429` rate limiting.
- **Secret Handling in Responses & Backups:**
  - API keys, Telegram bot tokens, DDNS tokens, Google Drive OAuth secrets, and Wi-Fi passphrases are never returned in plaintext on `GET`; endpoints return masked tokens (`••••`) or `has_*` booleans and preserve stored secrets when masked values are resubmitted.
  - `GET /api/v1/system/backup` builds a sanitized config archive excluding `/etc/shadow`, `/etc/dropbear`, `/etc/r3p/api-token`, `/etc/r3p/r3p.db`, `/etc/r3p/r3p.db.lock`, `/etc/r3p/sessions.json`, `/etc/r3p/ai.json`, `/etc/r3p/downloader.json`, `/etc/r3p/telegram.json`, and `/etc/r3p/gdrive.json` (while embedding a redacted `etc/r3p/r3p-db-sanitized.json` snapshot) unless `?include_secrets=1` is explicitly requested.

---

## 3. SSRF, Path Traversal & OTA Integrity Gates
- **Outbound HTTP / SSRF Protection (`ssrf_http.go`, `aria2.go`, `ai_agent.go`):** Custom AdBlock sources, DDNS custom URLs, package `.apk`/`.ipk` downloads, download jobs, and public AI base URLs validate URL schemes, block embedded credentials, reject loopback/private/link-local/unspecified/multicast IPs, and enforce destination IP validation across HTTP redirects (`validatePublicDownloadHost`). (`validateAIBaseURL` permits local loopback/private LAN hosts such as Ollama on `127.0.0.1` or `192.168.x.x` while blocking link-local/metadata/multicast targets.)
- **File Explorer & NAS Guards (`file_explorer.go`, `handlers_ext.go`):** Paths normalize via POSIX `path.Clean` and reject sensitive secrets (`/etc/shadow`, `/etc/dropbear`, `/etc/r3p/api-token`, etc.), system roots, symlink ancestors (`hasSymlinkAncestor`), and dangerous inline SVG HTML/XSS previews (`.svg` excluded from inline image serving; uploads block shell scripts on package installs).
- **Protected Self-Service:** `/api/v1/services/control` and package removal reject disabling, stopping, or removing `r3p-api`.
- **Verified Firmware & Bundle OTA (`firmware_update.go`):**
  - Staged files verify SHA-256 digests (`fileSHA256`) and remote checksums when published (`verifyOptionalRemoteSHA256`).
  - `r3p_api_binary` updates require full ELF32 Little-Endian MIPS (`EM_MIPS == 8`) headers (`isMIPSLEELF32Binary`).
  - `os_bundle_targz` updates enforce `inspectTarGzSafety` (rejecting path traversal, symlinks when `allowSafeSymlinks=false`, and files outside `usr/`, `www/`, `etc/`, `sbin/`, `lib/`).
  - `force` mode never bypasses SHA-256, ELF32 MIPSLE, or tarball traversal safety gates, and deploys swap binaries atomically with `.old` backup rollback on failure.
