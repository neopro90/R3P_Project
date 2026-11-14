#!/usr/bin/env python3
"""
Packages R3P OS Update Bundle (r3p-update-bundle.tar.gz) containing:
  - usr/bin/r3p-api (MIPSLE binary)
  - www/* (Vue 3 Production SPA)
  - etc/r3p/version.json (Release metadata)
And optionally deploys & verifies it on the live R3P router (192.168.2.1).
"""
import getpass
import hashlib
import io
import json
import os
import sys
import tarfile
import time
from pathlib import Path

ROOT = Path(r"d:\R3P_Project")
BINARY_PATH = ROOT / "build" / "artifacts" / "r3p-api"
DIST_DIR = ROOT / "frontend" / "dist"
RELEASE_JSON = ROOT / "release.json"
BUNDLE_PATH = ROOT / "build" / "artifacts" / "r3p-update-bundle.tar.gz"


def build_bundle() -> Path:
    if not BINARY_PATH.exists():
        raise FileNotFoundError(f"Missing binary: {BINARY_PATH}")
    if not DIST_DIR.exists():
        raise FileNotFoundError(f"Missing frontend dist: {DIST_DIR}")

    BUNDLE_PATH.parent.mkdir(parents=True, exist_ok=True)
    now = int(time.time())

    with tarfile.open(BUNDLE_PATH, "w:gz") as tar:
        # 1. Add usr/bin/r3p-api with 0755 permissions
        api_bytes = BINARY_PATH.read_bytes()
        bin_sha = hashlib.sha256(api_bytes).hexdigest()
        BINARY_PATH.with_name(BINARY_PATH.name + ".sha256").write_text(f"{bin_sha}  r3p-api\n", encoding="utf-8")
        ti = tarfile.TarInfo(name="usr/bin/r3p-api")
        ti.size = len(api_bytes)
        ti.mode = 0o755
        ti.mtime = now
        tar.addfile(ti, io.BytesIO(api_bytes))

        # 2. Add etc/r3p/version.json
        if RELEASE_JSON.exists():
            ver_bytes = RELEASE_JSON.read_bytes().lstrip(b"\xef\xbb\xbf")
            ti_ver = tarfile.TarInfo(name="etc/r3p/version.json")
            ti_ver.size = len(ver_bytes)
            ti_ver.mode = 0o644
            ti_ver.mtime = now
            tar.addfile(ti_ver, io.BytesIO(ver_bytes))

        # 3. Add www/* from frontend/dist
        for fpath in sorted(DIST_DIR.rglob("*")):
            if fpath.is_file():
                rel = fpath.relative_to(DIST_DIR).as_posix()
                data = fpath.read_bytes()
                ti_web = tarfile.TarInfo(name=f"www/{rel}")
                ti_web.size = len(data)
                ti_web.mode = 0o644
                ti_web.mtime = now
                tar.addfile(ti_web, io.BytesIO(data))

    bundle_sha = hashlib.sha256(BUNDLE_PATH.read_bytes()).hexdigest()
    BUNDLE_PATH.with_name(BUNDLE_PATH.name + ".sha256").write_text(
        f"{bundle_sha}  r3p-update-bundle.tar.gz\n", encoding="utf-8"
    )
    print(f"[OK] Built {BUNDLE_PATH} ({BUNDLE_PATH.stat().st_size:,} bytes, sha256={bundle_sha[:12]}...)")
    return BUNDLE_PATH


def deploy_to_router(host="192.168.1.7", user="root", password=None):
    import paramiko

    if password is None:
        password = os.environ.get("R3P_SSH_PASS")
    if not password:
        password = getpass.getpass(f"SSH password for {user}@{host}: ")

    target_hosts = [host]
    if "--host" in sys.argv:
        idx = sys.argv.index("--host")
        if idx + 1 < len(sys.argv):
            target_hosts = [sys.argv[idx + 1]]
    else:
        target_hosts = ["192.168.1.7", "192.168.2.1"]

    ssh = None
    active_host = None
    for h in target_hosts:
        print(f"[*] Trying connection to {user}@{h} ...")
        try:
            client = paramiko.SSHClient()
            client.load_system_host_keys()
            client.set_missing_host_key_policy(paramiko.RejectPolicy())
            client.connect(h, username=user, password=password, timeout=4)
            ssh = client
            active_host = h
            print(f"[+] Connected successfully to {user}@{active_host}")
            break
        except Exception as e:
            print(f"[-] Could not reach {h}: {e}")

    if not ssh or not active_host:
        raise ConnectionError(f"Failed to connect to any target router host: {target_hosts}")

    bundle_bytes = BUNDLE_PATH.read_bytes()
    expected_sha = hashlib.sha256(bundle_bytes).hexdigest()
    print(f"[*] Streaming {len(bundle_bytes):,} bytes to /tmp/r3p-update-bundle.tar.gz ...")
    stdin, stdout, stderr = ssh.exec_command("cat > /tmp/r3p-update-bundle.tar.gz")
    stdin.write(bundle_bytes)
    stdin.channel.shutdown_write()
    stdout.read()

    cmds = [
        f"echo '{expected_sha}  /tmp/r3p-update-bundle.tar.gz' | sha256sum -c -",
        "mkdir -p /etc/r3p /www",
        "if [ -f /usr/bin/r3p-api ]; then cp /usr/bin/r3p-api /usr/bin/r3p-api.bak; fi",
        "/etc/init.d/r3p-api stop 2>/dev/null || true",
        "killall -9 r3p-api 2>/dev/null || true",
        "rm -rf /www/assets",
        "tar -xzf /tmp/r3p-update-bundle.tar.gz -C /",
        "rm -f /tmp/r3p-update-bundle.tar.gz",
        "chmod 755 /usr/bin/r3p-api",
        "sync",
        "/etc/init.d/r3p-api start",
        "sleep 2",
        "if (command -v curl >/dev/null && curl -kfsS https://127.0.0.1:8443/api/v1/health >/dev/null) || wget -qO- --no-check-certificate https://127.0.0.1:8443/api/v1/health >/dev/null 2>&1; then rm -f /usr/bin/r3p-api.bak; echo HEALTH_OK; else echo 'HEALTH_FAILED_ROLLING_BACK'; /etc/init.d/r3p-api stop 2>/dev/null || true; if [ -f /usr/bin/r3p-api.bak ]; then mv /usr/bin/r3p-api.bak /usr/bin/r3p-api && chmod 755 /usr/bin/r3p-api && /etc/init.d/r3p-api start; fi; exit 1; fi",
        "netstat -tlpn | grep r3p-api",
    ]
    for c in cmds:
        _, out, err = ssh.exec_command(c)
        rc = out.channel.recv_exit_status()
        o = out.read().decode("utf-8", errors="ignore").strip()
        e = err.read().decode("utf-8", errors="ignore").strip()
        if o:
            print(f"  [{c}] -> {o}")
        if e and "uci" not in e.lower():
            print(f"  [{c} stderr] -> {e}")
        if rc != 0:
            ssh.close()
            raise RuntimeError(f"Remote command failed (rc={rc}): {c}")

    ssh.close()
    print(f"[OK] Deployment to {active_host} completed!")


if __name__ == "__main__":
    build_bundle()
    if "--deploy" in sys.argv:
        deploy_to_router()
