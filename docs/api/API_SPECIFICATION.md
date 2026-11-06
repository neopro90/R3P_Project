# API SPECIFICATION — R3P HOME SERVER OS

Base URL: `https://<router-ip>:8443/api/v1`

---

## 1. Authentication Endpoints

### 1.1. Check Session Status
- **Route:** `GET /auth/session`
- **Auth:** None (Cookie evaluated if present)
- **Response:**
  ```json
  { "ok": true, "data": { "authenticated": true } }
  ```

### 1.2. Create Session (Login)
- **Route:** `POST /auth/session`
- **Auth:** None
- **Body:**
  ```json
  { "token": "64_HEX_CHAR_TOKEN" }
  ```
- **Response:** `200 OK` + `Set-Cookie: r3p_session=...; HttpOnly; SameSite=Strict`

### 1.3. Terminate Session (Logout)
- **Route:** `DELETE /auth/session`
- **Response:** `200 OK` + Cookie expired.

---

## 2. System & Telemetry Endpoints

### 2.1. System Health
- **Route:** `GET /health`
- **Auth:** None
- **Response:**
  ```json
  {
    "ok": true,
    "data": {
      "status": "ok",
      "version": "0.2.0-phase1",
      "arch": "mipsle",
      "os": "linux",
      "uptime_seconds": 12450.2
    }
  }
  ```

### 2.2. Live Telemetry Stream (Server-Sent Events)
- **Route:** `GET /monitor/stream`
- **Auth:** Session Cookie
- **Format:** `text/event-stream` (Emits JSON telemetry payload every 1,000 ms)

### 2.3. System Hardware & Load
- **Route:** `GET /system`
- **Response:** CPU model, cores, frequency, load averages, memory distribution, board name.

### 2.4. Memory Metrics
- **Route:** `GET /memory`
- **Response:** RAM Total, Used, Free, Shared, Buffers, Cached.

---

## 3. Network & Wireless Endpoints

### 3.1. Network Interfaces
- **Route:** `GET /network`
- **Response:** Active interfaces, IP addresses, MACs, TX/RX bytes, link states.

### 3.2. Connected Clients (DHCP Leases + ARP)
- **Route:** `GET /clients`
- **Response:** Array of connected client devices, hostname, IP, MAC, connection interface.

### 3.3. Wireless Radio Status
- **Route:** `GET /wireless`
- **Response:** Radios (2.4GHz & 5GHz), SSIDs, encryption, channels, txpower, connected clients.

---

## 4. Storage & NAS Endpoints

### 4.1. Storage Devices
- **Route:** `GET /storage`
- **Response:** Block devices, mount points, filesystem types, used and available space.

### 4.2. File Browser
- **Route:** `GET /files?path=/mnt/usb/`
- **Response:** Directory listings, file names, sizes, permissions, last modified dates.

---

## 5. Download Bot Endpoints

### 5.1. List Downloads
- **Route:** `GET /downloads`
- **Response:** Active, waiting, and completed Aria2 download jobs with speed and progress.

### 5.2. Add Download URL
- **Route:** `POST /downloads`
- **Body:**
  ```json
  { "url": "https://example.com/file.zip", "destination": "/mnt/usb/downloads" }
  ```
- **Response:** `201 Created` with Aria2 GID.

---

## 6. Automation & Notification Endpoints

### 6.1. Telegram Configuration
- **Route:** `GET /telegram` | `PUT /telegram` | `DELETE /telegram`
- **PUT Body:**
  ```json
  { "bot_token": "...", "chat_id": "...", "notifications_enabled": true }
  ```
