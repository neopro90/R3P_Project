# Build logs

Complete OpenWrt ImageBuilder output is retained here. All source, logs, intermediate image outputs, and firmware artifacts created for this project remain beneath `D:\R3P_Project`.

| Log | Result |
|---|---|
| `baseline-20261101-105255.log` | Failed initial attempt; inherited Windows PATH caused `find -execdir` to reject a path component. Retained as diagnostic history. |
| `baseline-20261101-105850.log` | PASS — stock R3P factory and sysupgrade images built. |
| `custom-20261101-110216.log` | PASS — custom API/frontend/USB-storage firmware images built. |
| `custom-20261102-141110.log` | PASS — final Dashboard 2.0 API/frontend/USB-storage firmware rebuild; `scripts/verify-custom.sh` passed. |
| `custom-20261102-201649.log` | PASS — intermediate module-pipeline/Telegram aria2/Photo Library build before API UPX optimization; superseded by the final build. |
| `custom-20261102-203926.log` | PASS — final stripped/UPX API module-pipeline firmware with adblock-fast and ksmbd packages; `scripts/verify-custom.sh` passed. |
| `custom-20261102-205656.log` | PASS — responsive interactive SVG module topology firmware; superseded by the final frontend-polish rebuild. |
| `custom-20261102-210028.log` | PASS — responsive SVG module topology firmware; superseded by the visible-connector/status rebuild. |
| `custom-20261102-210842.log` | PASS — corrected SVG connector visibility and unavailable/active status display; frontend embedded and structural verifier passed. |

Final image hashes, sizes, package checks, and root filesystem verification are recorded in `TEST_REPORTS/firmware-build.md`. Dashboard and mini-server integration boundaries are recorded in `TEST_REPORTS/dashboard-2.0.md` and `TEST_REPORTS/mini-server-pipeline.md`.
