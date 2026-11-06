# R3P API

The Go API is intended for the R3P (`linux/mipsle`, soft-float) target. Build from this directory:

```powershell
$env:GOOS = "linux"
$env:GOARCH = "mipsle"
$env:GOMIPS = "softfloat"
$env:CGO_ENABLED = "0"
go build -ldflags="-s -w" -trimpath -o ..\output\r3p-api .
```

The firmware build uses `build/cross-compile-go-v2.sh` to strip the MIPSLE binary and UPX-pack it with `--best --lzma`. `scripts/build-custom.sh` also handles an unpacked API binary idempotently before staging; `scripts/verify-custom.sh` runs `upx -t` and checks that the packaged API matches the staged binary. UPX is a build-time dependency only. The compressed binary's execution/startup/RAM behavior has not been tested on the router.

## Authentication

All `/api/v1` routes except `/api/v1/health` and the session-creation `POST /api/v1/auth/session` require an authenticated session. Set `R3P_API_TOKEN` or provide a non-empty token file at `/etc/r3p/api-token` (override its path with `R3P_API_TOKEN_FILE`). There is no built-in/default credential. The client submits the token once; the server issues an eight-hour HttpOnly, SameSite=Strict cookie. `DELETE /api/v1/auth/session` revokes the session.

Use the API only on a trusted management network and enable HTTPS before deployment. Set `R3P_TLS_CERT` and `R3P_TLS_KEY` together to make the Go listener use TLS 1.2 or newer. If neither is configured, it serves plain HTTP; authentication does not encrypt the token or traffic, so do not expose that configuration to an untrusted network.

The firmware init service creates a random 256-bit API token on first boot and stores it in `/etc/r3p/api-token` with root-only permissions. It also writes a copy to `/root/r3p-api-token` so an administrator can retrieve it over local root SSH after securing SSH access. The token is never baked into the image or printed to logs. The init service runs `r3p-api generate-cert CERT_PATH KEY_PATH` to create a self-signed ECDSA TLS certificate and private key on first boot; the private key is root-only. The firmware serves the SPA and API together at `https://<router-host>:8443`. Browsers must be configured to trust the router certificate (or explicitly accept its warning) before entering credentials; use a trusted certificate for production.

Cross-origin requests are allowed only when the request Origin and API Host have the same hostname. Wildcard CORS is not enabled.

## Read-only USB file browser

- `GET /api/v1/files?mount=<mount-point>&path=<relative-path>` lists a directory.
- `GET /api/v1/files/download?mount=<mount-point>&path=<relative-path>` streams a regular file as an attachment.
- `GET /api/v1/files/preview?mount=<mount-point>&path=<relative-path>` streams supported raster images inline for the Photo Library.

All routes require the authenticated session. The mount point must match a currently mounted `/dev/sd*` or `/dev/mmcblk*` volume below `/mnt/` or `/media/`. Paths are relative to that mount, parent traversal and backslash separators are rejected, symlink entries are skipped from listings, and each directory response is capped at 500 entries. Preview is limited to JPEG, PNG, GIF, WebP, BMP, and AVIF; SVG/HTML are not served inline. File operations are read-only; there are no upload, overwrite, or delete routes.

The browser dashboard layout, theme, and automation drafts are local browser preferences. Automation drafts are not sent to or executed by the router. Diagnostic endpoints expose fixed read-only checks; the API does not provide a general shell or arbitrary command endpoint.

## Mini-server module status

- `GET /api/v1/modules` reports observed process/package/storage state for SSH, DNS, adblock-fast, Telegram, aria2, ksmbd, and Photo Library. `active`, `enabled`, `installed`, and `not-installed` have different meanings; service process presence does not prove complete health.
- OpenWrt firmware includes `dropbear`, `dnsmasq`, `adblock-fast`, `aria2`, `ksmbd-server`, and `kmod-fs-ksmbd`. `ksmbd` is not enabled and no SMB share is created by default. Adblock Fast being installed/enabled does not independently prove that DNS filtering is active.
- `/etc/init.d/r3p-aria2` starts aria2 with JSON-RPC restricted to loopback. A random first-boot RPC secret is stored root-only in `/etc/r3p/aria2-rpc-token`; the frontend never sees it.

## Downloads and Telegram Auto Download

- `GET /api/v1/downloads` returns aria2 active, waiting, and recent stopped jobs.
- `POST /api/v1/downloads` accepts `{"url":"…","mount":"…","path":"…"}` and queues the URL in an existing directory on a mounted removable USB volume.
- `GET /api/v1/telegram` returns configuration state and the chat ID, never the bot token.
- `PUT /api/v1/telegram` stores the bot token, allowlisted chat ID, and existing USB destination; this operation requires HTTPS. `DELETE /api/v1/telegram` removes the saved token and stops polling, also over HTTPS.
- Telegram long polling accepts text/caption URLs only from the configured chat ID. HTTP(S) and magnet links are extracted and queued; private/local literal IPs, local hostnames, and domains resolving to non-public addresses are rejected before queueing; redirects are disabled, and queued files stay in the selected USB folder. aria2 resolves domain names when it later performs the transfer, so these checks do not guarantee protection from DNS rebinding; use trusted URLs, a private trusted chat, and outbound firewall restrictions.
- Telegram credentials are written atomically with owner-only permissions to `/etc/r3p/telegram.json`; they are not returned, logged, or stored in browser preferences.

Telegram Bot API and aria2 are external services and were not contacted by local tests. Actual download, Telegram polling, DNS filtering, SMB share setup, and USB runtime require validation on a controlled R3P. The security checks reduce exposure but do not replace outbound firewall policy or device testing.
