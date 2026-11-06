import os
import sys
import time
import glob
import socket
import threading
import urllib.request
from http.server import HTTPServer, BaseHTTPRequestHandler
import paramiko

ROUTER_IP = "192.168.2.1"
ROUTER_USER = "root"
ROUTER_PASS = "root"
CACHE_DIR = r"D:\R3P_Project\Supplementary Files\repo_cache"
ESP_SRC_DIR = r"D:\Source_Tools\Esp32-Download"
FRONTEND_DIST = r"D:\R3P_Project\frontend\dist"
BACKEND_BIN = r"D:\R3P_Project\build\artifacts\r3p-api"

os.makedirs(CACHE_DIR, exist_ok=True)


class OpenWrtCachingProxy(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        pass

    def do_GET(self):
        rel = self.path.lstrip("/")
        local_path = os.path.join(CACHE_DIR, rel.replace("/", os.sep))
        if not os.path.exists(local_path):
            os.makedirs(os.path.dirname(local_path), exist_ok=True)
            url = "https://downloads.openwrt.org/" + rel
            try:
                req = urllib.request.Request(url, headers={"User-Agent": "OpenWrt-APK-Proxy/1.0"})
                with urllib.request.urlopen(req, timeout=30) as resp:
                    data = resp.read()
                with open(local_path, "wb") as f:
                    f.write(data)
            except Exception as e:
                self.send_error(502, str(e))
                return
        with open(local_path, "rb") as f:
            data = f.read()
        self.send_response(200)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def start_reverse_tunnel(transport, remote_port, local_port):
    transport.request_port_forward("127.0.0.1", remote_port)

    def accept_loop():
        while transport.is_active():
            chan = transport.accept(timeout=1.0)
            if chan is None:
                continue
            threading.Thread(target=bridge, args=(chan, local_port), daemon=True).start()

    def bridge(chan, port):
        try:
            sock = socket.create_connection(("127.0.0.1", port), timeout=10)
        except Exception:
            chan.close()
            return

        def fwd(src, dst):
            try:
                while True:
                    b = src.recv(32768)
                    if not b:
                        break
                    dst.sendall(b)
            except Exception:
                pass
            finally:
                try:
                    dst.close()
                except Exception:
                    pass

        t1 = threading.Thread(target=fwd, args=(chan, sock), daemon=True)
        t2 = threading.Thread(target=fwd, args=(sock, chan), daemon=True)
        t1.start()
        t2.start()
        t1.join()
        t2.join()

    threading.Thread(target=accept_loop, daemon=True).start()


def sftp_mkdir_p(sftp, remote_dir):
    parts = remote_dir.strip("/").split("/")
    cur = ""
    for p in parts:
        cur += "/" + p
        try:
            sftp.stat(cur)
        except IOError:
            try:
                sftp.mkdir(cur)
            except IOError:
                pass


def upload_dir(sftp, local_dir, remote_dir):
    sftp_mkdir_p(sftp, remote_dir)
    for root, dirs, files in os.walk(local_dir):
        rel = os.path.relpath(root, local_dir).replace("\\", "/")
        rdir = remote_dir if rel == "." else f"{remote_dir}/{rel}"
        sftp_mkdir_p(sftp, rdir)
        for fname in files:
            lfile = os.path.join(root, fname)
            rfile = f"{rdir}/{fname}"
            sftp.put(lfile, rfile)


def run_ssh(client, cmd, timeout=180):
    print(f"[SSH] {cmd}")
    _, out, err = client.exec_command(cmd, timeout=timeout)
    out_s = out.read().decode("utf-8", errors="replace")
    err_s = err.read().decode("utf-8", errors="replace")
    rc = out.channel.recv_exit_status()
    if out_s.strip():
        print(out_s.strip())
    if err_s.strip():
        print("STDERR:", err_s.strip())
    return rc, out_s, err_s


def main():
    print(f"Waiting for router {ROUTER_IP}:22...")
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    for attempt in range(60):
        try:
            client.connect(ROUTER_IP, 22, ROUTER_USER, ROUTER_PASS, timeout=3)
            print("Connected to R3P Router via SSH!")
            break
        except Exception as e:
            if attempt % 5 == 0:
                print(f"  Attempt {attempt+1}: {e}")
            time.sleep(2)
    else:
        print("ERROR: Could not connect to 192.168.2.1:22")
        sys.exit(1)

    # 1. Ensure 500GB USB 3.0 HDD (/dev/sda1) is mounted & tuned
    run_ssh(client, """
mkdir -p /mnt/sda1
if [ -b /dev/sda1 ] && ! grep -q '/mnt/sda1' /proc/mounts; then
    mount -t ext4 -o noatime,nodiratime,commit=30,barrier=0 /dev/sda1 /mnt/sda1 || mount /dev/sda1 /mnt/sda1
fi
ln -sfn /mnt/sda1 /mnt/usb
mkdir -p /mnt/sda1/downloads /mnt/sda1/share /mnt/sda1/esp32-downloads /mnt/sda1/esp32-download-host
if [ -d /sys/block/sda/queue ]; then
    echo 4096 > /sys/block/sda/queue/read_ahead_kb 2>/dev/null || true
    echo 1024 > /sys/block/sda/queue/max_sectors_kb 2>/dev/null || true
    echo 256 > /sys/block/sda/queue/nr_requests 2>/dev/null || true
    echo 0 > /sys/block/sda/queue/add_random 2>/dev/null || true
    echo 2 > /sys/block/sda/queue/rq_affinity 2>/dev/null || true
fi
df -h /mnt/sda1 /overlay
""")

    # 2. Install yt-dlp and iwinfo (NO ffmpeg/kmod-sound-core to avoid MT7621 soundcore panic)
    httpd = HTTPServer(("127.0.0.1", 18088), OpenWrtCachingProxy)
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    start_reverse_tunnel(client.get_transport(), 18080, 18088)

    run_ssh(client, """
rm -f /etc/modules.d/*sound* /etc/modules.d/*input* /lib/modules/*/soundcore.ko* 2>/dev/null || true
cp -n /etc/apk/repositories.d/distfeeds.list /etc/apk/repositories.d/distfeeds.list.bak 2>/dev/null || true
sed 's|https://downloads.openwrt.org|http://127.0.0.1:18080|g' /etc/apk/repositories.d/distfeeds.list.bak > /etc/apk/repositories.d/distfeeds.list
apk update
apk add yt-dlp iwinfo
cp /etc/apk/repositories.d/distfeeds.list.bak /etc/apk/repositories.d/distfeeds.list 2>/dev/null || true
which yt-dlp iwinfo
yt-dlp --version 2>/dev/null || true
""", timeout=300)

    # 3. Upload ESP32-Download static web assets, config, APK, and firmware to /mnt/sda1/esp32-download-host
    sftp = client.open_sftp()
    print("Uploading ESP32-Download public web UI & data to /mnt/sda1/esp32-download-host...")
    upload_dir(sftp, os.path.join(ESP_SRC_DIR, "local-host", "public"), "/mnt/sda1/esp32-download-host/public")
    sftp_mkdir_p(sftp, "/mnt/sda1/esp32-download-host/data")
    for fname in ["runtime-queue.json", "scheduled-downloads.json"]:
        src_f = os.path.join(ESP_SRC_DIR, "local-host", "data", fname)
        if os.path.exists(src_f):
            sftp.put(src_f, f"/mnt/sda1/esp32-download-host/data/{fname}")

    apk_file = os.path.join(ESP_SRC_DIR, "esp-download.apk")
    if os.path.exists(apk_file):
        print("Uploading esp-download.apk...")
        sftp.put(apk_file, "/mnt/sda1/esp32-download-host/esp-download.apk")

    fw_root = os.path.join(ESP_SRC_DIR, "firmware-cyd", "releases")
    if os.path.exists(fw_root):
        for model in os.listdir(fw_root):
            fbin = os.path.join(fw_root, model, f"{model}-firmware.bin")
            if os.path.exists(fbin):
                rdir = f"/mnt/sda1/esp32-download-host/firmware/{model}"
                sftp_mkdir_p(sftp, rdir)
                sftp.put(fbin, f"{rdir}/{model}-firmware.bin")

    # 4. Upload new Frontend (/www) and Backend (/usr/bin/r3p-api)
    print("Uploading new Frontend (/www)...")
    run_ssh(client, "rm -rf /www/assets/*")
    upload_dir(sftp, FRONTEND_DIST, "/www")

    print("Uploading new Backend (/usr/bin/r3p-api)...")
    run_ssh(client, "/etc/init.d/r3p-api stop 2>/dev/null || killall r3p-api 2>/dev/null || true")
    sftp.put(BACKEND_BIN, "/usr/bin/r3p-api")
    run_ssh(client, "chmod +x /usr/bin/r3p-api && /etc/init.d/r3p-api start")
    sftp.close()

    time.sleep(3)
    run_ssh(client, "ps | grep r3p-api; netstat -tlpn 2>/dev/null | grep r3p-api || true")
    client.close()
    print("ALL DEPLOYMENTS COMPLETE!")


if __name__ == "__main__":
    main()
