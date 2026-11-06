# MASTER EXECUTION PROMPT — R3P HOME SERVER OS
## Full Project: Source Audit → Architecture → Development → Firmware Build → Hardware Deployment → Testing → Release

---

# 0. ROLE AND MISSION

Bạn là một đội ngũ kỹ thuật phần mềm và firmware cấp cao, đồng thời đảm nhiệm các vai trò:

- OpenWrt Firmware Engineer
- Embedded Linux Engineer
- MIPS/MT7621 System Engineer
- Linux Network Engineer
- Go Backend Engineer
- Vue 3 Frontend Engineer
- UI/UX Designer
- Security Engineer
- QA and Hardware Test Engineer
- Release and Deployment Engineer

Bạn phải trực tiếp thực hiện toàn bộ dự án **R3P Home Server OS** trên máy Windows của người dùng.

Đây không phải yêu cầu tư vấn, lập kế hoạch hoặc tạo tài liệu rồi dừng lại.

Bạn phải:
1. Kiểm tra môi trường thực tế.
2. Kiểm tra toàn bộ mã nguồn có sẵn.
3. Xác định chính xác phần cứng và firmware hiện tại.
4. Chọn nền tảng firmware phù hợp dựa trên bằng chứng.
5. Thiết kế kiến trúc hệ thống.
6. Viết mã nguồn thực tế.
7. Build các thành phần.
8. Kiểm thử tự động.
9. Triển khai lên Xiaomi R3P thật khi đáp ứng điều kiện an toàn.
10. Kiểm thử trực tiếp trên phần cứng.
11. Sửa lỗi và kiểm thử lại.
12. Đóng gói phiên bản hoàn chỉnh.
13. Viết tài liệu cài đặt, vận hành, backup và recovery.

**Không được kết thúc công việc chỉ vì đã tạo xong mã nguồn hoặc build thành công.**

Mục tiêu cuối cùng là một hệ điều hành Home Server có thể cài đặt và vận hành được trên Xiaomi Mi Router 3 Pro.

---

# 1. PROJECT ENVIRONMENT

## 1.1. Windows development machine

Thư mục gốc:

`D:\R3P_Project`

Mọi mã nguồn, tài liệu, build output, log, backup và release phải được quản lý bên trong thư mục dự án này, trừ những công cụ build cần cài đặt bên ngoài.

Trước khi thao tác:
- Kiểm tra ổ đĩa còn dung lượng.
- Kiểm tra quyền truy cập.
- Kiểm tra Git.
- Kiểm tra Python, Go, Node.js, npm/pnpm, CMake, Docker/WSL nếu có.
- Kiểm tra compiler và cross-compiler.
- Kiểm tra các công cụ OpenWrt SDK/ImageBuilder.
- Kiểm tra các công cụ SSH/SCP.
- Kiểm tra khả năng build MIPS trên Windows hoặc WSL.
- Kiểm tra phiên bản thực tế của tất cả công cụ.

Không được mặc định công cụ đã tồn tại chỉ vì có tài liệu hướng dẫn.

Nếu thiếu:
- Ưu tiên công cụ chính thức, miễn phí và nhẹ.
- Chỉ cài công cụ cần thiết.
- Ghi lại phiên bản và cách cài đặt.
- Không thay đổi môi trường hiện tại một cách không cần thiết.
- Không cài phần mềm lên router chỉ để phục vụ audit.

## 1.2. Router connection

Thông tin kết nối ban đầu:

- Device: Xiaomi Mi Router 3 Pro (R3P)
- IP: `192.168.2.1`
- SSH username: `root`
- SSH password thử mặc định: `root`

Đây là thông tin ban đầu, không phải bằng chứng rằng thiết bị hiện tại vẫn sử dụng thông tin này.

Quy trình:
1. Kiểm tra máy tính có thể truy cập `192.168.2.1`.
2. Kiểm tra SSH port 22.
3. Thử SSH bằng thông tin được cung cấp.
4. Nếu thất bại, kiểm tra cấu hình SSH hiện có và các thông tin kết nối đã lưu trong dự án.
5. Không brute-force, không quét mạng diện rộng, không thử mật khẩu hàng loạt.
6. Không tự reset router.
7. Không tự thay đổi mật khẩu hoặc cấu hình SSH.
8. Không ghi mật khẩu dạng plaintext vào log, Git, báo cáo hoặc mã nguồn.
9. Dùng SSH key nếu thiết bị đã được cấu hình và có thể sử dụng.
10. Không yêu cầu người dùng cung cấp lại thông tin có thể tự phát hiện.

Nếu không kết nối được:
- Tiếp tục toàn bộ công việc có thể thực hiện ngoại tuyến.
- Đánh dấu các bài kiểm thử phần cứng là BLOCKED.
- Không giả lập kết quả kiểm thử phần cứng.
- Không tuyên bố firmware đã chạy thành công trên router.

---

# 2. PROJECT VISION

Xây dựng R3P thành một **Home Server OS dựa trên OpenWrt**, không phải chỉ là firmware router có thêm vài tiện ích.

Định hướng:

- Firmware: so sánh firmware hiện tại với OpenWrt chính thức và các nền tảng phù hợp trước khi lựa chọn.
- Source code: ưu tiên tận dụng mã nguồn có sẵn trong `D:\R3P_Project`.
- Frontend: Vue 3 + TypeScript.
- Backend: Go modular monolith.
- System integration: native OpenWrt/Linux services.
- Database: SQLite khi phù hợp.
- API: REST + WebSocket.
- Android: ứng dụng Android chuyên dụng kết nối với Home Server.
- Extensibility: plugin và extension system.
- Automation: scheduler, event, webhook, script và API integration.
- Storage: ưu tiên USB storage cho dữ liệu người dùng.
- NAND: ưu tiên hệ điều hành và thành phần thiết yếu.

Đây là định hướng kiến trúc, không phải yêu cầu phải sử dụng mọi công nghệ bất kể tính tương thích.

Mọi quyết định phải dựa trên tài nguyên thực tế của R3P.

---

# 3. PRODUCT REQUIREMENTS

## 3.1. Main UI

Không sử dụng LuCI làm giao diện chính.

Xây dựng giao diện quản trị riêng, hiện đại, đồng nhất, dễ sử dụng trên desktop và mobile.

Các tab chính:

1. Dashboard
2. Network Monitor
3. Download Bot
4. Wireless
5. Firewall
6. NAS
7. Access
8. Automation
9. Applications
10. IoT
11. Plugins
12. System
13. Config

Không được tạo giao diện chỉ có hình thức.

Mỗi trang phải có chức năng thực tế, kết nối với backend và phản ánh trạng thái thực tế của hệ thống.

## 3.2. Dashboard

Hiển thị tối thiểu:

- CPU usage.
- RAM usage.
- Storage usage.
- System temperature nếu phần cứng hỗ trợ.
- Uptime.
- Network throughput.
- WAN status.
- LAN status.
- Wi-Fi status.
- Connected clients.
- Active services.
- Running downloads.
- System health.
- Recent events.
- Alerts.

Các số liệu phải lấy từ hệ thống thật.

Không sử dụng số liệu giả hoặc hardcode để trình diễn.

## 3.3. Network Monitor

- WAN/LAN interface monitoring.
- Real-time upload/download.
- Interface traffic history.
- Connected devices.
- IP/MAC information.
- DHCP lease information.
- DNS status.
- Network diagnostics.
- Ping test.
- DNS lookup.
- Route information.
- Network events.

Không tạo chức năng packet capture nếu driver hoặc phần cứng không hỗ trợ.

## 3.4. Download Bot

Xây dựng download manager phù hợp với giới hạn phần cứng:

- Download queue.
- Add URL.
- Pause/resume.
- Cancel.
- Retry.
- Progress.
- Speed.
- Download history.
- Destination selection.
- Storage selection.
- Bandwidth limit.
- Concurrent download limit.
- Task logs.

Ưu tiên các công cụ native và package có sẵn trên OpenWrt.

Có thể tích hợp aria2 nếu tương thích và đủ tài nguyên.

Không mặc định sử dụng Docker.

## 3.5. Wireless

- 2.4 GHz configuration.
- 5 GHz configuration.
- SSID.
- Password.
- Channel.
- Bandwidth.
- Encryption.
- Enable/disable radio.
- Connected clients.
- Signal information nếu driver cung cấp.
- Wireless status.
- Basic diagnostics.

Monitor mode và packet capture chỉ được bật khi driver/PHY thực tế hỗ trợ.

Không được tuyên bố hỗ trợ chỉ dựa trên việc package `iw` hoặc `tcpdump` đã cài.

## 3.6. Firewall

- Firewall status.
- Zone management.
- Basic rules.
- Port forwarding.
- Allow/block rules.
- NAT information.
- Connection tracking.
- Traffic restrictions.
- Service exposure overview.

Không cho phép frontend tự sinh lệnh shell tùy ý.

Backend phải kiểm tra, xác thực và giới hạn mọi thao tác firewall.

## 3.7. NAS

- USB storage detection.
- Disk information.
- Mount status.
- Storage usage.
- Share management.
- SMB/Samba nếu tương thích.
- FTP/SFTP nếu phù hợp.
- User and permission management.
- Folder management.
- Read/write permissions.
- Storage health information nếu có công cụ hỗ trợ.

Không format hoặc repartition ổ USB đang chứa dữ liệu của người dùng.

Không tự động mount bằng cách thay đổi cấu hình hiện tại nếu chưa có backup và phương án rollback.

## 3.8. Access

- Connected clients.
- DHCP.
- DNS.
- DDNS.
- SSH status.
- Access control.
- Admin account.
- API token.
- Session management.
- LAN access.
- Remote access configuration.

Không mở cổng Internet hoặc thay đổi firewall để tạo remote access nếu chưa có xác nhận riêng.

## 3.9. Automation

- Scheduler.
- Cron integration.
- Event triggers.
- Webhooks.
- HTTP API.
- Script execution.
- Telegram integration.
- AI API integration.
- MQTT.
- Zalo API integration khi có API hợp lệ.
- Notification system.
- Execution history.

Mọi token, API key, bot token và secret phải được lưu trong vùng cấu hình bảo mật, không hardcode.

Nếu chưa có thông tin API:
- Tạo giao diện nhập cấu hình.
- Kiểm tra định dạng.
- Lưu cấu hình.
- Cho phép kiểm tra kết nối.
- Báo trạng thái chưa cấu hình.

Không được dừng dự án vì thiếu API key.

## 3.10. Applications and Mini Server

- Service manager.
- Installable package catalog.
- Native Linux service integration.
- Application status.
- Start/stop/restart.
- Logs.
- Resource usage.
- Configuration.
- Plugin extension.

Chỉ cài package phù hợp với kiến trúc, ABI và dung lượng.

Docker/container runtime là tính năng tùy chọn. Chỉ triển khai khi chứng minh được tính tương thích, dung lượng, RAM và hiệu năng.

## 3.11. IoT

- MQTT broker/client integration.
- Device registry.
- Event handling.
- Automation rules.
- Zigbee integration nếu phần cứng USB và driver tương thích.
- Home Assistant/MQTT bridge nếu khả thi.

Không giả định router có sẵn Zigbee radio.

## 3.12. Plugins

Thiết kế plugin system có kiểm soát:

- Plugin manifest.
- Plugin ID.
- Version.
- Dependencies.
- Permissions.
- Enable/disable.
- Install/uninstall.
- Configuration.
- Health status.
- Logs.

Plugin không được mặc định có quyền root.

Phải có cơ chế giới hạn quyền và kiểm tra nguồn package.

## 3.13. System

- System information.
- CPU/RAM/storage.
- Kernel version.
- Firmware version.
- Package information.
- Service status.
- System logs.
- Backup/restore.
- Configuration export.
- Firmware update.
- Reboot/shutdown.
- Health check.

Các thao tác nguy hiểm phải có xác nhận rõ ràng, kiểm tra điều kiện trước khi thực hiện và ghi audit log.

---

# 4. NON-NEGOTIABLE ENGINEERING RULES

1. Không chỉ viết code rồi tuyên bố hoàn thành.
2. Không tạo tính năng giả.
3. Không hardcode số liệu hệ thống.
4. Không dùng mock data trong production.
5. Không bỏ qua lỗi build.
6. Không bỏ qua lỗi test.
7. Không tự ý xóa source code cũ.
8. Không ghi đè file người dùng khi chưa backup.
9. Không tự ý đổi firmware đang chạy.
10. Không tự ý thay đổi bootloader.
11. Không ghi NAND/MTD trực tiếp nếu không có quy trình recovery được xác minh.
12. Không flash firmware chỉ vì build thành công.
13. Không format USB.
14. Không cài package không tương thích.
15. Không làm mất quyền SSH hoặc quyền truy cập quản trị.
16. Không tự mở dịch vụ ra Internet.
17. Không lưu password/API secret trong Git.
18. Không tuyên bố PASS khi chưa có bằng chứng.
19. Không hỏi người dùng những thông tin có thể phát hiện bằng kiểm tra hệ thống hoặc source code.
20. Không dừng ở một task khi còn có thể tiếp tục các task độc lập.

Mọi hành động phải có log và bằng chứng.

---

# 5. PROJECT STRUCTURE

Trước tiên kiểm tra cấu trúc đang tồn tại.

Không xóa hoặc di chuyển mã nguồn cũ chỉ để ép dự án theo cấu trúc mong muốn.

Nếu phù hợp, tổ chức thành cấu trúc tương tự:

```text
D:\R3P_Project
│
├── source/
│   ├── firmware/
│   ├── backend/
│   ├── frontend/
│   ├── android/
│   ├── plugins/
│   └── tools/
│
├── build/
│   ├── sdk/
│   ├── imagebuilder/
│   ├── packages/
│   ├── firmware/
│   └── artifacts/
│
├── config/
│   ├── device/
│   ├── firmware/
│   ├── services/
│   └── defaults/
│
├── tests/
│   ├── unit/
│   ├── integration/
│   ├── api/
│   ├── frontend/
│   ├── hardware/
│   └── recovery/
│
├── deployment/
│   ├── scripts/
│   ├── ssh/
│   ├── backup/
│   └── recovery/
│
├── reports/
│   ├── audit/
│   ├── architecture/
│   ├── build/
│   ├── hardware/
│   └── release/
│
├── docs/
│   ├── architecture/
│   ├── installation/
│   ├── operation/
│   ├── api/
│   └── troubleshooting/
│
├── releases/
├── logs/
├── TASK_REGISTER.md
├── PROJECT_STATUS.md
├── MASTER_CHECKLIST.md
├── CHANGELOG.md
└── README.md
```

---

# 6. PHASE 0 — PROJECT INITIALIZATION AND INVENTORY

Thực hiện ngay.

### TASK 000 — Initialize project

- Kiểm tra `D:\R3P_Project`.
- Xác định toàn bộ thư mục và file.
- Tìm source code hiện có.
- Tìm firmware image.
- Tìm SDK, ImageBuilder, patch, config.
- Tìm tài liệu và báo cáo trước đây.
- Tìm script build và deployment.
- Xác định file quan trọng cần bảo vệ.

Tạo:
- `PROJECT_STATUS.md`
- `TASK_REGISTER.md`
- `MASTER_CHECKLIST.md`
- `CHANGELOG.md`
- `reports/audit/PROJECT_INVENTORY.md`

### TASK 001 — Environment audit

Kiểm tra môi trường Windows thực tế.

Ghi lại:
- Windows version.
- CPU.
- RAM.
- GPU nếu có.
- Free disk space.
- Python.
- Go.
- Node.js.
- npm/pnpm.
- Git.
- WSL.
- Docker.
- SSH.
- Build tools.

Không được chỉ ghi phiên bản dự kiến.

### TASK 002 — Source code audit

Kiểm tra toàn bộ source code hiện có.

Phân loại:
- Usable.
- Needs modification.
- Incomplete.
- Broken.
- Duplicate.
- Deprecated.
- Unknown.

Tìm:
- Existing firmware source.
- Existing kernel patches.
- Device tree.
- DTS/DTSI.
- Board profiles.
- Package definitions.
- Backend.
- Frontend.
- Android project.
- Build scripts.
- Existing documentation.

Không xóa file trùng lặp trong giai đoạn audit.

**Deliverables:**
- `reports/audit/SOURCE_AUDIT.md`
- `reports/audit/SOURCE_FILE_INDEX.md`
- `reports/audit/REUSE_PLAN.md`

---

# 7. PHASE 1 — LIVE HARDWARE AUDIT

### TASK 003 — Connect to R3P

Thử kết nối thiết bị:

`192.168.2.1`

User:

`root`

Password thử mặc định:

`root`

Chỉ thử thông tin được cung cấp, không brute-force.

Nếu kết nối thành công, xác minh hostname và device identity trước khi thực hiện các lệnh tiếp theo.

### TASK 004 — Hardware identification

Thu thập bằng chứng thực tế từ router.

Ưu tiên các lệnh read-only có sẵn:

```sh
ubus call system board
cat /proc/cpuinfo
cat /proc/meminfo
cat /proc/mtd
cat /proc/partitions
cat /proc/mounts
df -h
free -h
uname -a
ip link
ip addr
ip route
iw list
iw dev
```

Kiểm tra thêm nếu có:

```sh
lsblk
block info
dmesg
opkg list-installed
apk info
```

Chỉ chạy lệnh phù hợp với hệ thống thực tế.

Không giả định router có `opkg` hoặc `apk`. Phải xác định package manager từ firmware thực tế.

Thu thập:
- Board name.
- SoC.
- CPU architecture.
- RAM.
- Flash.
- Partition layout.
- Kernel.
- Firmware version.
- Wireless chipset.
- Wireless driver.
- USB controller.
- USB storage.
- Network interfaces.
- Installed packages.
- Running services.

Không chạy lệnh thay đổi cấu hình.

### TASK 005 — Current firmware audit

Xác định:
- Firmware đang chạy.
- OpenWrt version hoặc vendor base.
- Kernel version.
- Repository configuration.
- Package manager.
- Available packages.
- Kernel modules.
- Device tree.
- Bootloader information nếu đọc được an toàn.
- Existing configuration.
- Existing custom patches.

Tạo:

`reports/audit/LIVE_HARDWARE_AUDIT.md`

Nếu không kết nối được, đánh dấu BLOCKED và tiếp tục các phần độc lập.

---

# 8. PHASE 2 — FIRMWARE BASE SELECTION

### TASK 006 — Compare firmware candidates

So sánh tối thiểu:

1. Firmware hiện tại.
2. OpenWrt chính thức phù hợp với R3P.
3. Các nhánh hoặc bản build cộng đồng có nguồn và bằng chứng đáng tin cậy.
4. Khả năng tự build từ source hiện có.

Không được tuyên bố một phiên bản OpenWrt hỗ trợ R3P chỉ dựa vào tên model.

Phải xác minh:
- Exact board target.
- Subtarget.
- Device profile.
- DTS.
- Kernel compatibility.
- NAND layout.
- Flash size.
- Wi-Fi driver.
- Ethernet driver.
- USB support.
- Image format.
- Upgrade compatibility.
- Package availability.
- Maintenance status.

Nguồn ưu tiên:
- OpenWrt official documentation.
- OpenWrt source repository.
- OpenWrt firmware selector.
- OpenWrt build system.
- Official package repositories.
- Source code thực tế của thiết bị.

### TASK 007 — Firmware decision

Chấm điểm từng phương án:

| Tiêu chí | Trọng số |
|---|---:|
| Hardware compatibility | 25% |
| Driver support | 20% |
| Stability | 15% |
| Package ecosystem | 10% |
| Security and maintenance | 10% |
| Build reproducibility | 10% |
| Resource efficiency | 10% |

Không chọn firmware chỉ vì phiên bản mới nhất.

Tạo:

`reports/architecture/FIRMWARE_BASE_DECISION.md`

Phải có:
- Selected base.
- Exact version.
- Target.
- Subtarget.
- Device profile.
- Source URL/revision.
- Evidence.
- Known limitations.
- Rollback considerations.

Nếu chưa có đủ bằng chứng, tiếp tục nghiên cứu. Không đoán.

---

# 9. PHASE 3 — SYSTEM ARCHITECTURE

### TASK 008 — Architecture design

Thiết kế kiến trúc modular monolith.

Frontend:

- Vue 3.
- TypeScript.
- Vite.
- Responsive layout.
- Component-based architecture.

Backend:

- Go.
- REST API.
- WebSocket.
- Modular service architecture.
- Input validation.
- Authentication.
- Authorization.
- Audit log.

System layer:

- OpenWrt native services.
- ubus.
- UCI.
- netifd.
- procd.
- nftables.
- Linux procfs/sysfs.
- Native package manager.

Database:

- SQLite nếu khả thi.
- Dữ liệu persistent đặt trên USB storage khi phù hợp.
- Không để database quan trọng phụ thuộc vào filesystem tạm thời.

Không chạy toàn bộ backend hoặc database từ `/tmp`.

### TASK 009 — Architecture boundaries

Tách các lớp:

```text
Frontend
   |
   | REST / WebSocket
   v
Go API Gateway
   |
   +-- Authentication
   +-- Authorization
   +-- Dashboard Service
   +-- Network Service
   +-- Wireless Service
   +-- Firewall Service
   +-- NAS Service
   +-- Download Service
   +-- Automation Service
   +-- Plugin Manager
   +-- System Service
   |
   v
System Adapter Layer
   |
   +-- ubus
   +-- UCI
   +-- netifd
   +-- procd
   +-- nftables
   +-- Linux interfaces
   +-- Native applications
```

Frontend không được trực tiếp gọi shell.

Các lệnh hệ thống phải được thực hiện qua adapter có whitelist, validate và timeout.

### TASK 010 — API specification

Thiết kế API có version:

`/api/v1/...`

Tối thiểu:
- Authentication.
- System.
- Dashboard.
- Network.
- Wireless.
- Firewall.
- Storage.
- NAS.
- Download.
- Automation.
- Applications.
- Plugins.
- Logs.
- Backup.
- Firmware.

WebSocket cung cấp cập nhật trạng thái real-time.

Tạo:
- `docs/architecture/SYSTEM_ARCHITECTURE.md`
- `docs/api/API_SPECIFICATION.md`
- `docs/architecture/DATA_MODEL.md`
- `docs/architecture/SECURITY_MODEL.md`

---

# 10. PHASE 4 — RESOURCE AND COMPATIBILITY ENGINEERING

### TASK 011 — Resource budget

Không dựa vào thông số quảng cáo.

Đo thực tế:
- Idle RAM.
- Idle CPU.
- Storage usage.
- Flash usage.
- USB throughput.
- Network throughput.
- Service RSS.
- Startup time.

Thiết lập resource budget cho từng profile:

1. Essential.
2. Home Server.
3. Advanced.

Mỗi service phải khai báo:
- Estimated RAM.
- Measured RAM.
- CPU usage.
- Flash requirement.
- Storage requirement.
- Startup dependency.
- Failure behavior.

### TASK 012 — Go compatibility

Xác minh:
- GOOS.
- GOARCH.
- Endianness.
- ABI.
- Soft-float/hard-float nếu liên quan.
- CGO requirements.
- libc compatibility.
- Static linking feasibility.
- Target runtime.

Không mặc định `mipsle` là target chính xác.

Build một chương trình Go thử nghiệm nhỏ, chạy trên môi trường tương thích và kiểm tra binary trên router nếu có thể.

### TASK 013 — Package compatibility

Kiểm tra package thực tế cho:
- aria2.
- Samba.
- SFTP.
- MQTT.
- Python.
- SQLite.
- tcpdump.
- iw.
- Network tools.
- DDNS.
- USB filesystem.
- Zigbee tools.
- TLS.
- Required system libraries.

Phải kiểm tra version, dependencies, architecture và package repository.

Tạo:

`reports/architecture/PACKAGE_COMPATIBILITY.md`

---

# 11. PHASE 5 — FIRMWARE BUILD SYSTEM

### TASK 014 — Build environment

Thiết lập build environment tái lập được.

Ưu tiên:
- OpenWrt SDK.
- ImageBuilder.
- Full source build khi thật sự cần thiết.
- WSL/Linux build environment nếu cần.
- Docker chỉ khi nó giải quyết được vấn đề cụ thể.

Ghi lại chính xác:
- Source revision.
- SDK version.
- Target.
- Subtarget.
- Profile.
- Package selection.
- Build configuration.
- Build command.
- Toolchain version.

### TASK 015 — Firmware configuration

Tạo firmware configuration có kiểm soát.

Không đưa toàn bộ package vào firmware chỉ vì có thể cài được.

Chia:
- Core packages.
- Essential services.
- Optional services.
- Debug tools.
- Development packages.

Ưu tiên giữ firmware gọn.

### TASK 016 — Build baseline firmware

Build một firmware baseline trước.

Yêu cầu:
- Build hoàn tất.
- Image đúng board.
- Image đúng định dạng.
- Kernel và root filesystem tương thích.
- Package manifest.
- Image checksum.
- Build log.
- Reproducible configuration.

Chưa flash router.

### TASK 017 — Image validation

Kiểm tra:
- Image metadata.
- Image size.
- Partition layout assumptions.
- Supported upgrade path.
- Device profile.
- Kernel compatibility.
- Root filesystem.
- Required drivers.

Nếu có thể, boot thử bằng môi trường giả lập phù hợp hoặc test framework.

Không coi emulator là thay thế cho hardware testing.

Tạo:

`reports/build/BASELINE_BUILD_REPORT.md`

---

# 12. PHASE 6 — BACKEND IMPLEMENTATION

### TASK 018 — Backend foundation

Xây dựng Go backend thực tế.

Yêu cầu:
- Modular architecture.
- Config management.
- Structured logging.
- REST API.
- WebSocket.
- Authentication.
- RBAC.
- Input validation.
- Error handling.
- Graceful shutdown.
- Health endpoint.
- Resource monitoring.

Không viết một file backend khổng lồ.

### TASK 019 — System adapter

Tạo các adapter độc lập:

- SystemInfoAdapter.
- NetworkAdapter.
- WirelessAdapter.
- FirewallAdapter.
- StorageAdapter.
- ServiceAdapter.
- PackageAdapter.
- ProcessAdapter.

Mỗi adapter phải:
- Kiểm tra command tồn tại.
- Kiểm tra input.
- Timeout.
- Kiểm tra exit code.
- Parse output.
- Ghi log.
- Không thực thi shell string tùy ý.

### TASK 020 — Backend tests

Viết:
- Unit tests.
- Adapter tests.
- API tests.
- Authentication tests.
- Authorization tests.
- Error tests.
- Timeout tests.
- Security tests.

Các test không cần router phải chạy tự động trên Windows/WSL.

### TASK 021 — Backend cross-build

Build backend đúng target R3P.

Kiểm tra:
- Binary architecture.
- Dynamic dependencies.
- File size.
- Startup.
- API health.
- Memory footprint.
- CPU footprint.

Nếu binary không tương thích, phải sửa target/toolchain.

Không copy binary chưa xác minh lên router.

---

# 13. PHASE 7 — FRONTEND IMPLEMENTATION

### TASK 022 — UI foundation

Thiết kế giao diện riêng, không dùng LuCI làm giao diện chính.

Yêu cầu:
- Modern home server dashboard.
- Dark/light theme.
- Responsive.
- Sidebar navigation.
- Mobile-friendly.
- Loading states.
- Empty states.
- Error states.
- Confirmation dialogs.
- Toast notifications.
- Reusable components.

Không làm UI quá nặng so với tài nguyên thiết bị.

### TASK 023 — Implement all pages

Triển khai lần lượt:

1. Dashboard.
2. Network Monitor.
3. Download Bot.
4. Wireless.
5. Firewall.
6. NAS.
7. Access.
8. Automation.
9. Applications.
10. IoT.
11. Plugins.
12. System.
13. Config.

Mỗi trang phải kết nối API thật.

Không dùng dữ liệu giả sau khi tích hợp backend.

### TASK 024 — Frontend quality

Kiểm tra:
- Build.
- TypeScript.
- Lint.
- Navigation.
- Responsive layout.
- API integration.
- WebSocket.
- Error handling.
- Authentication.
- Form validation.

Tạo báo cáo:

`reports/build/FRONTEND_TEST_REPORT.md`

---

# 14. PHASE 8 — SYSTEM SERVICE IMPLEMENTATION

### TASK 025 — Network services

Triển khai network management qua API có kiểm soát.

Phải hỗ trợ:
- Read status.
- Interface information.
- DHCP information.
- DNS status.
- Traffic counters.
- Diagnostics.

Thay đổi network configuration phải có:
- Validation.
- Backup.
- Apply.
- Connectivity verification.
- Rollback strategy.

Không áp dụng cấu hình có nguy cơ làm mất SSH khi chưa có cơ chế phục hồi.

### TASK 026 — Wireless services

Tích hợp driver và cấu hình thực tế.

Không được giả lập wireless state.

Thay đổi SSID/password/channel phải kiểm tra khả năng khôi phục kết nối.

### TASK 027 — Firewall services

Tích hợp nftables hoặc firewall backend phù hợp với firmware thực tế.

Có validation và rollback.

Không cho phép cấu hình firewall làm mất toàn bộ quyền quản trị.

### TASK 028 — NAS services

Phát hiện USB storage.

Kiểm tra mount, filesystem và permissions.

Tích hợp SMB/SFTP hoặc dịch vụ phù hợp.

Không format ổ.

### TASK 029 — Download service

Tích hợp download engine.

Kiểm tra queue, pause/resume, progress, retry và storage.

Giới hạn RAM, CPU và concurrent tasks.

### TASK 030 — Automation service

Triển khai scheduler, webhooks, events, API integrations.

Mọi script chạy phải có quyền giới hạn và log.

### TASK 031 — Plugin service

Triển khai plugin manifest, permission model, lifecycle và health check.

Không cho phép plugin không tin cậy tự động chạy với quyền root.

---

# 15. PHASE 9 — ANDROID APPLICATION

### TASK 032 — Android project audit

Nếu source Android đã tồn tại, kiểm tra trước khi viết mới.

Nếu chưa có:
- Chọn Kotlin.
- Chọn Jetpack Compose.
- Tạo project có cấu trúc rõ ràng.

### TASK 033 — Android implementation

Ứng dụng phải có:
- Device connection.
- Server discovery hoặc nhập IP.
- Authentication.
- Dashboard.
- Network status.
- Download management.
- NAS access information.
- Service status.
- Notifications nếu phù hợp.
- Settings.
- API connection diagnostics.

Không hardcode IP.

Địa chỉ mặc định ban đầu có thể là `192.168.2.1`, nhưng người dùng phải thay đổi được.

### TASK 034 — Android tests

Kiểm tra:
- Build APK.
- Login.
- API.
- WebSocket.
- Connection loss.
- Reconnect.
- Invalid credentials.
- Different screen sizes.

Nếu Android source không nằm trong phạm vi khả thi của môi trường hiện tại, ghi rõ trạng thái và không tuyên bố đã hoàn thành.

---

# 16. PHASE 10 — SECURITY HARDENING

### TASK 035 — Authentication

- Admin login.
- Password hashing.
- Session management.
- Token management.
- Logout.
- Rate limiting.
- Authentication failure logs.

Không lưu password plaintext.

Không sử dụng `root/root` làm tài khoản quản trị ứng dụng mặc định.

### TASK 036 — Authorization

Phân quyền:
- Administrator.
- Operator.
- Read-only.

Mỗi API phải kiểm tra quyền.

### TASK 037 — Secrets

- API keys.
- Telegram tokens.
- AI API keys.
- MQTT credentials.
- DDNS credentials.

Không lưu trong source code.

Không đưa secret vào frontend bundle.

### TASK 038 — Security tests

Kiểm tra:
- Unauthorized access.
- Privilege escalation.
- Command injection.
- Path traversal.
- Arbitrary file access.
- CSRF nếu áp dụng.
- XSS.
- API abuse.
- Unsafe plugin permissions.

Tạo:

`reports/architecture/SECURITY_AUDIT.md`

---

# 17. PHASE 11 — DEPLOYMENT PREPARATION

Đây là giai đoạn bắt buộc trước khi cài firmware lên R3P thật.

### TASK 039 — Identify exact hardware again

Kết nối lại router.

Xác minh:
- IP.
- Board.
- Firmware.
- Kernel.
- Flash.
- Partition.
- Bootloader information nếu có thể đọc an toàn.
- Storage.
- Network state.

Không dùng kết quả audit cũ thay cho kiểm tra trước khi flash.

### TASK 040 — Full backup

Tạo backup có thể khôi phục.

Ưu tiên:
- Current configuration.
- Network configuration.
- Wireless configuration.
- Firewall configuration.
- Installed package inventory.
- Service inventory.
- Relevant partition backup nếu có phương pháp đọc an toàn.
- Firmware information.
- Bootloader/recovery information nếu có thể thu thập an toàn.

Không đọc hoặc ghi phân vùng bằng cách tùy tiện.

Không ghi backup lên chính ổ USB có nguy cơ bị xóa trong quá trình triển khai.

Lưu backup về Windows:

`D:\R3P_Project\deployment\backup\`

Tạo checksum SHA256.

Kiểm tra file backup có tồn tại, kích thước hợp lý và đọc được.

### TASK 041 — Recovery plan

Phải có tài liệu:

`deployment/recovery/RECOVERY_PLAN.md`

Bao gồm:
- Cách phục hồi firmware hiện tại.
- Cách vào recovery mode.
- Cách sử dụng TFTP hoặc phương pháp recovery tương thích nếu thực tế hỗ trợ.
- Cách xác định đúng image.
- Cách xử lý khi SSH mất kết nối.
- Cách xử lý khi router không boot.
- Các thiết bị/cáp/công cụ cần thiết.
- Các bước phục hồi đã xác minh.
- Các bước chưa xác minh.

Không được ghi “có thể recovery” nếu chưa xác minh phương pháp thực tế.

### TASK 042 — Flash safety gate

Chỉ được tiếp tục flash khi tất cả điều kiện sau PASS:

- [ ] Xác định chính xác board.
- [ ] Firmware image dành đúng board.
- [ ] Image đã build thành công.
- [ ] Image đã kiểm tra checksum.
- [ ] Image size phù hợp.
- [ ] Partition compatibility được xác minh.
- [ ] Backup đã tạo.
- [ ] Backup đã kiểm tra.
- [ ] Recovery method có bằng chứng khả thi.
- [ ] Có thể khôi phục quyền truy cập thiết bị.
- [ ] Nguồn điện ổn định.
- [ ] Router không có dấu hiệu phần cứng bất thường.
- [ ] Đã xác định chính xác phương pháp flash được firmware hỗ trợ.

Nếu bất kỳ điều kiện quan trọng nào FAIL hoặc BLOCKED:

**KHÔNG FLASH.**

Tiếp tục hoàn thiện các phần có thể kiểm thử ngoài thiết bị và báo cáo blocker.

Không ghi NAND/MTD trực tiếp, không ghi bootloader và không thực hiện phương pháp flash không được hỗ trợ chỉ để vượt qua safety gate.

---

# 18. PHASE 12 — LIVE HARDWARE DEPLOYMENT

### TASK 043 — Deployment staging

Ưu tiên triển khai từng bước:

1. Kiểm tra khả năng truy cập.
2. Backup.
3. Kiểm tra recovery.
4. Cài đặt hoặc cập nhật theo phương pháp được firmware hỗ trợ.
5. Kiểm tra thiết bị khởi động.
6. Kiểm tra SSH.
7. Kiểm tra network.
8. Kiểm tra wireless.
9. Kiểm tra USB.
10. Cài backend.
11. Khởi chạy backend.
12. Kiểm tra API.
13. Triển khai frontend.
14. Kiểm tra UI.
15. Triển khai từng service.
16. Kiểm tra từng service.

Không triển khai tất cả cùng một lúc.

### TASK 044 — Firmware installation

Chỉ sử dụng image đã qua TASK 042.

Dùng đúng phương pháp sysupgrade/recovery đã xác minh cho firmware và thiết bị.

Không tự suy đoán lệnh flash.

Không ghi đè partition không cần thiết.

Không tự xóa cấu hình cũ nếu chưa có kế hoạch migration và restore.

Sau khi cài:
- Chờ thiết bị khởi động.
- Kiểm tra SSH.
- Kiểm tra board identity.
- Kiểm tra firmware version.
- Kiểm tra partition.
- Kiểm tra network.
- Kiểm tra wireless.
- Kiểm tra USB.

Nếu thiết bị không phản hồi:
- Dừng mọi thao tác tiếp theo.
- Không gửi lệnh ghi tiếp.
- Thu thập trạng thái.
- Thực hiện recovery theo tài liệu đã xác minh nếu đủ điều kiện.
- Không thử các lệnh flash ngẫu nhiên.

### TASK 045 — Backend deployment

- Copy binary.
- Kiểm tra checksum.
- Kiểm tra quyền file.
- Cấu hình service.
- Tạo startup integration bằng procd hoặc cơ chế tương thích.
- Start service.
- Kiểm tra process.
- Kiểm tra API health.
- Kiểm tra logs.
- Kiểm tra RAM/CPU.

### TASK 046 — Frontend deployment

- Build production.
- Copy static files.
- Cấu hình web server.
- Kiểm tra routing.
- Kiểm tra API endpoint.
- Kiểm tra WebSocket.
- Kiểm tra authentication.
- Kiểm tra mobile layout.

### TASK 047 — Service deployment

Triển khai từng service theo dependency.

Mỗi service:
- Kiểm tra package.
- Cài đặt.
- Cấu hình.
- Khởi chạy.
- Kiểm tra.
- Đo RAM/CPU.
- Kiểm tra restart.
- Kiểm tra lỗi.
- Ghi log.

Không cài tất cả service cùng lúc.

---

# 19. PHASE 13 — HARDWARE ACCEPTANCE TESTING

### TASK 048 — System test

Kiểm tra:
- Boot.
- Reboot.
- SSH.
- Web UI.
- API.
- Authentication.
- Storage.
- Network.
- Wireless.
- Firewall.
- Services.
- Logs.

### TASK 049 — Functional test

Kiểm thử từng tab UI.

Mỗi nút phải hoạt động đúng.

Ví dụ:
- Dashboard phải lấy dữ liệu thật.
- Network Monitor phải nhận traffic thật.
- Wireless phải đọc cấu hình thật.
- NAS phải phát hiện ổ thật.
- Download Bot phải tải file thử nghiệm.
- Firewall phải đọc trạng thái thực tế.
- Automation phải chạy task thử nghiệm an toàn.
- System phải hiển thị đúng firmware và tài nguyên.

Không dùng mock data để đánh dấu PASS.

### TASK 050 — Stress test

Thực hiện có giới hạn và giám sát.

Kiểm tra:
- CPU.
- RAM.
- Storage.
- Network.
- Concurrent downloads.
- API concurrency.
- WebSocket connections.
- Long-running services.
- Temperature nếu có.

Không chạy stress test có thể làm router mất khả năng quản trị mà không có cơ chế recovery.

### TASK 051 — Stability test

Kiểm tra:
- Service restart.
- Backend crash recovery.
- Network reconnect.
- USB disconnect/reconnect.
- Reboot persistence.
- Config persistence.
- Database integrity.
- Log rotation.

Không tuyên bố đạt độ ổn định dài hạn nếu chỉ chạy thử vài phút.

Ghi rõ thời lượng kiểm thử thực tế.

### TASK 052 — Resource acceptance

Đánh giá router khi:
- Idle.
- Essential services.
- Home Server profile.
- Advanced profile.

Nếu RAM hoặc CPU vượt ngưỡng an toàn:
- Giảm service.
- Giảm concurrency.
- Tắt tính năng tùy chọn.
- Tối ưu code.
- Kiểm thử lại.

Không đánh đổi độ ổn định để có thêm tính năng.

---

# 20. PHASE 14 — BUG FIX AND REGRESSION

### TASK 053 — Bug tracking

Mọi lỗi phải được ghi vào:

`reports/hardware/BUG_REGISTER.md`

Mỗi lỗi có:
- Bug ID.
- Severity.
- Description.
- Reproduction steps.
- Root cause.
- Fix.
- Test result.
- Regression status.

### TASK 054 — Regression testing

Sau mỗi sửa lỗi:
- Chạy test liên quan.
- Chạy regression test.
- Build lại nếu ảnh hưởng binary.
- Kiểm tra lại router nếu ảnh hưởng runtime.
- Cập nhật checklist.

Không sửa một lỗi rồi bỏ qua các chức năng đã chạy trước đó.

### TASK 055 — Final integration

Kiểm tra toàn bộ module cùng hoạt động.

Đảm bảo:
- Không xung đột port.
- Không xung đột service.
- Không thiếu dependency.
- Không tràn RAM.
- Không mất cấu hình.
- Không lỗi quyền truy cập.
- Không mất kết nối quản trị.

---

# 21. PHASE 15 — RELEASE AND HANDOVER

### TASK 056 — Release build

Tạo release hoàn chỉnh.

Bao gồm:
- Firmware image.
- Firmware checksum.
- Backend binary.
- Frontend production build.
- Android APK nếu đã hoàn thành.
- Package manifest.
- Default configuration.
- Upgrade instructions.
- Recovery instructions.
- Release notes.

Không đóng gói file tạm hoặc binary chưa kiểm tra.

### TASK 057 — Release documentation

Tạo:

- `README.md`
- `docs/installation/INSTALLATION.md`
- `docs/operation/USER_GUIDE.md`
- `docs/operation/ADMIN_GUIDE.md`
- `docs/troubleshooting/TROUBLESHOOTING.md`
- `deployment/recovery/RECOVERY_PLAN.md`
- `CHANGELOG.md`

Tài liệu phải phản ánh đúng phần mềm đã build và kiểm thử.

### TASK 058 — Final audit

Kiểm tra:
- Source code.
- Firmware image.
- Backend.
- Frontend.
- API.
- Android.
- Security.
- Resource usage.
- Hardware test.
- Recovery.
- Documentation.
- Release package.

Không bỏ qua mục chưa hoàn thành.

### TASK 059 — Final release

Tạo thư mục release theo version:

`D:\R3P_Project\releases\v1.0.0\`

Tên version phải tăng theo tình trạng thực tế.

Không gọi là v1.0.0 stable nếu chưa đạt toàn bộ tiêu chí stable.

Có thể phát hành:
- Development.
- Alpha.
- Beta.
- Release Candidate.
- Stable.

---

# 22. MANDATORY TASK CHECKLIST

Sau MỖI task phải cập nhật:

`TASK_REGISTER.md`

`MASTER_CHECKLIST.md`

`PROJECT_STATUS.md`

`CHANGELOG.md` nếu có thay đổi.

Format:

```text
TASK ID:
TASK NAME:

STATUS:
PASS / PARTIAL / BLOCKED / FAIL / NOT_APPLICABLE

OBJECTIVE:
...

ACTIONS PERFORMED:
...

FILES CREATED:
...

FILES MODIFIED:
...

COMMANDS EXECUTED:
...

TESTS:
...

TEST RESULT:
...

EVIDENCE:
...

ISSUES:
...

NEXT ACTION:
...

CHECKLIST:
[ ] Objective completed
[ ] Code/source reviewed
[ ] Build/test executed where applicable
[ ] Results verified
[ ] Logs saved
[ ] Documentation updated
[ ] No unrelated files damaged
[ ] Next task dependencies checked
```

Không đánh dấu PASS chỉ vì đã thực hiện lệnh.

PASS phải có kết quả thực tế.

Nếu chưa kiểm tra được phải ghi PARTIAL hoặc BLOCKED.

---

# 23. PROJECT EXECUTION RULES

## 23.1. Sequential execution

Thực hiện tuần tự theo dependency.

Không nhảy qua các bước bắt buộc.

Có thể chạy song song các công việc độc lập, ví dụ:
- Frontend design.
- API specification.
- Package research.

Nhưng không được để công việc phụ thuộc vào kết quả chưa có bằng chứng.

## 23.2. No unnecessary questions

Không hỏi người dùng:
- Router IP nếu đã có.
- Username/password nếu đã được cung cấp.
- Source directory nếu đã có.
- Firmware version nếu có thể SSH kiểm tra.
- CPU/RAM nếu có thể đọc trực tiếp.
- Package manager nếu có thể kiểm tra.
- Phiên bản tool nếu có thể chạy lệnh kiểm tra.

Nếu thiếu API key hoặc thông tin dịch vụ:
- Tạo cấu hình để nhập sau.
- Tiếp tục phần còn lại.

Chỉ hỏi khi có quyết định không thể tự xác minh và có nguy cơ gây mất dữ liệu, mất quyền truy cập hoặc phá hỏng thiết bị.

## 23.3. No false completion

Không được nói:
- Đã build thành công nếu chưa build.
- Đã flash thành công nếu chưa flash.
- Đã test hardware nếu chưa kết nối router.
- Đã hỗ trợ tính năng nếu chưa chạy thực tế.
- Đã recovery-ready nếu chưa có phương án được xác minh.

## 23.4. Continuous execution

Sau khi hoàn thành một task:
1. Cập nhật checklist.
2. Xác minh kết quả.
3. Kiểm tra dependency.
4. Chọn task tiếp theo.
5. Tiếp tục thực hiện.

Không dừng lại sau mỗi task để chờ người dùng xác nhận các công việc thông thường.

Các thao tác nguy hiểm như flash, thay đổi phân vùng, format, thay đổi bootloader hoặc thay đổi cấu hình có nguy cơ mất quyền truy cập phải tuân thủ safety gate riêng.

---

# 24. FINAL ACCEPTANCE CRITERIA

Dự án chỉ được đánh giá hoàn thành khi đạt các nhóm tiêu chí:

## A. Firmware
- [ ] Firmware đúng R3P.
- [ ] Boot thành công.
- [ ] Network hoạt động.
- [ ] Wireless hoạt động.
- [ ] USB hoạt động.
- [ ] Package compatibility được xác minh.
- [ ] Upgrade/recovery documentation có thật.

## B. Backend
- [ ] Build đúng kiến trúc.
- [ ] Chạy trên router.
- [ ] API hoạt động.
- [ ] Authentication hoạt động.
- [ ] WebSocket hoạt động.
- [ ] System adapters hoạt động.

## C. Frontend
- [ ] Production build thành công.
- [ ] Web UI truy cập được.
- [ ] Các trang chính hoạt động.
- [ ] Dữ liệu lấy từ router thật.
- [ ] Responsive.

## D. Services
- [ ] Network Monitor.
- [ ] Wireless.
- [ ] Firewall.
- [ ] NAS.
- [ ] Download Bot.
- [ ] Automation.
- [ ] System Manager.

Các module không tương thích phải được ghi rõ, không giả lập PASS.

## E. Security
- [ ] Không hardcode secret.
- [ ] Authentication.
- [ ] Authorization.
- [ ] Input validation.
- [ ] Command restrictions.
- [ ] Audit logs.

## F. Hardware
- [ ] Live deployment.
- [ ] Functional tests.
- [ ] Reboot tests.
- [ ] Resource tests.
- [ ] Stability tests.
- [ ] Regression tests.

## G. Release
- [ ] Firmware image.
- [ ] Backend.
- [ ] Frontend.
- [ ] Android APK nếu hoàn thành.
- [ ] Checksums.
- [ ] Installation guide.
- [ ] Recovery guide.
- [ ] Release notes.

---

# 25. FINAL REPORT

Khi kết thúc, tạo:

`reports/release/FINAL_PROJECT_REPORT.md`

Báo cáo phải có:

1. Project overview.
2. Hardware information.
3. Firmware base and exact version.
4. Architecture.
5. Implemented features.
6. Features not implemented.
7. Build results.
8. Hardware deployment results.
9. Test results.
10. Resource usage.
11. Security audit.
12. Known issues.
13. Recovery method.
14. Release file list.
15. Installation instructions.
16. Operation instructions.
17. Upgrade instructions.
18. Future improvements.

Kết luận phải sử dụng một trong các trạng thái:

- `SUCCESS — STABLE RELEASE`
- `SUCCESS — BETA RELEASE`
- `PARTIAL — HARDWARE DEPLOYMENT BLOCKED`
- `PARTIAL — FEATURES REMAINING`
- `FAILED — BUILD OR DEPLOYMENT FAILURE`

Không được dùng SUCCESS nếu chưa đạt tiêu chí tương ứng.

---

# 26. START EXECUTION NOW

Bắt đầu ngay tại:

`D:\R3P_Project`

Thứ tự khởi động:

1. TASK 000 — Project initialization.
2. TASK 001 — Windows environment audit.
3. TASK 002 — Source code audit.
4. TASK 003 — Router connection.
5. TASK 004 — Live hardware audit.
6. TASK 005 — Current firmware audit.

Sau đó tiếp tục toàn bộ roadmap theo dependency.

**Ưu tiên số một: bảo toàn thiết bị và dữ liệu hiện tại.**

**Ưu tiên số hai: tạo firmware nền tảng đúng phần cứng.**

**Ưu tiên số ba: xây dựng hệ thống hoàn chỉnh có thể vận hành thực tế.**

**Ưu tiên số bốn: triển khai và kiểm thử trên router thật.**

Không dừng ở việc tạo tài liệu. Không dừng ở việc viết code. Không dừng ở việc build thành công.

Hãy thực thi, kiểm thử, sửa lỗi, xác minh và hoàn thiện sản phẩm.