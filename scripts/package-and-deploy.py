#!/usr/bin/env python3
"""
Packages R3P OS Update Bundle (r3p-update-bundle.tar.gz) containing:
  - usr/bin/r3p-api (MIPSLE binary)
  - www/* (Vue 3 Production SPA)
  - etc/r3p/version.json (Release metadata)
And optionally deploys & verifies it on the live R3P router (192.168.2.1).
"""
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
        ti = tarfile.TarInfo(name="usr/bin/r3p-api")
        ti.size = len(api_bytes)
        ti.mode = 0o755
        ti.mtime = now
        tar.addfile(ti, io.BytesIO(api_bytes))

        # 2. Add etc/r3p/version.json
        if RELEASE_JSON.exists():
            ver_bytes = RELEASE_JSON.read_bytes()
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

    print(f"[OK] Built {BUNDLE_PATH} ({BUNDLE_PATH.stat().st_size:,} bytes)")
    return BUNDLE_PATH


def deploy_to_router(host="192.168.2.1", user="root", password="root"):
    import paramiko

    print(f"[*] Connecting to {user}@{host} ...")
    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    ssh.connect(host, username=user, password=password, timeout=10)

    bundle_bytes = BUNDLE_PATH.read_bytes()
    print(f"[*] Streaming {len(bundle_bytes):,} bytes to /tmp/r3p-update-bundle.tar.gz ...")
    stdin, stdout, stderr = ssh.exec_command("cat > /tmp/r3p-update-bundle.tar.gz")
    stdin.write(bundle_bytes)
    stdin.channel.shutdown_write()
    stdout.read()

    cmds = [
        "mkdir -p /etc/r3p /www",
        "rm -f /usr/bin/r3p-api.old && mv /usr/bin/r3p-api /usr/bin/r3p-api.old 2>/dev/null || true",
        "tar -xzf /tmp/r3p-update-bundle.tar.gz -C /",
        "rm -f /usr/bin/r3p-api.old /tmp/r3p-update-bundle.tar.gz",
        "chmod 755 /usr/bin/r3p-api",
        "/etc/init.d/r3p-api restart",
        "sleep 2",
        "netstat -tlpn | grep r3p-api",
    ]
    for c in cmds:
        _, out, err = ssh.exec_command(c)
        o = out.read().decode("utf-8", errors="ignore").strip()
        e = err.read().decode("utf-8", errors="ignore").strip()
        if o:
            print(f"  [{c}] -> {o}")
        if e and "uci" not in e.lower():
            print(f"  [{c} stderr] -> {e}")

    ssh.close()
    print("[OK] Deployment to 192.168.2.1 completed!")


if __name__ == "__main__":
    build_bundle()
    if "--deploy" in sys.argv:
        deploy_to_router()
