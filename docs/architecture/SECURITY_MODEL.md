# SECURITY MODEL — R3P HOME SERVER OS

## 1. Principles
1. **Zero External Access by Default:** No administration ports (SSH, API, Aria2 RPC) are exposed to WAN without explicit, authenticated user action.
2. **No Plaintext Passwords or Tokens:** All secrets are hashed with SHA-256 or bcrypt. Session tokens are generated from cryptographic randomness (`/dev/urandom`).
3. **Least Privilege & Process Isolation:** Application modules and user-defined scripts execute under non-root system accounts whenever supported by OpenWrt userland.
4. **Command Injection Prevention:** All system interactions use typed Go structs with argument array tokenization (`exec.Command(cmd, arg1, arg2)`). Direct shell string evaluation (`sh -c`) is strictly forbidden.

---

## 2. Authentication & Session Management

- **API Access Token:** 64-character hexadecimal token (256 bits of entropy) created on initial device boot. Stored in root-only file `/etc/r3p/api-token` (file permissions `0600`).
- **Session Authentication:**
  - Login requires `POST /api/v1/auth/session` with the token.
  - Verification uses constant-time comparison (`crypto/subtle.ConstantTimeCompare`) to defeat timing attacks.
  - Successful authentication issues an encrypted, HTTP-Only, SameSite=Strict cookie (`r3p_session`).
  - Sessions automatically expire after 8 hours of inactivity.
- **Brute-Force Rate Limiting:** Consecutive failed authentication attempts trigger exponential backoff and IP-level throttling.

---

## 3. Role-Based Access Control (RBAC)

| Role | Permissions | Scope |
|---|---|---|
| **Administrator** | Full read/write access | System, Firmware upgrade, SSH keys, Network, Wireless, Firewall |
| **Operator** | Read/write operational features | Download Bot, NAS file browsing, Service restarts |
| **Read-Only / Guest**| Read-only metrics | Dashboard, Network monitor, Interface statistics |

---

## 4. Hardware Safety Gates
- **NAND Protection:** Direct writes to bootloader partitions (`mtd0` Bootloader, `mtd2` Bdata, `mtd3` Factory) are blocked in firmware by MTD driver flags.
- **Fail-Safe Sysupgrade:** Sysupgrade operations must pass image magic number checks (`sysupgrade -T`), checksum validations, and free RAM margin checks prior to flashing.
