import paramiko
import sys
import json

ROUTER_IP = "192.168.2.1"
ROUTER_USER = "root"
ROUTER_PASS = "root"

def main():
    print(f"Connecting to {ROUTER_IP} as {ROUTER_USER}...")
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    try:
        client.connect(ROUTER_IP, port=22, username=ROUTER_USER, password=ROUTER_PASS, timeout=10)
    except Exception as e:
        print(f"Connection failed: {e}")
        sys.exit(1)

    print("Connected successfully!\n" + "="*50)

    commands = [
        ("Release / OS Info", "cat /etc/openwrt_release || cat /etc/os-release"),
        ("Kernel Version", "uname -a"),
        ("Board Info", "ubus call system board 2>/dev/null || cat /tmp/sysinfo/board_name 2>/dev/null || echo 'N/A'"),
        ("CPU Info", "cat /proc/cpuinfo"),
        ("Memory Info", "free -m || cat /proc/meminfo"),
        ("NAND Chip Detection (dmesg)", "dmesg | grep -i -E 'Manufacturer ID|nand|chip ID|esmt|micron' | head -30"),
        ("MTD Partitions", "cat /proc/mtd"),
        ("Mount & Disk Space", "df -h"),
        ("Current Network Interfaces", "ip addr show || ifconfig"),
    ]

    results = {}
    for title, cmd in commands:
        print(f"\n--- {title} [{cmd}] ---")
        stdin, stdout, stderr = client.exec_command(cmd, timeout=10)
        out = stdout.read().decode('utf-8', errors='replace').strip()
        err = stderr.read().decode('utf-8', errors='replace').strip()
        if out:
            print(out)
        if err:
            print(f"[stderr]: {err}")
        results[title] = out or err

    client.close()
    print("\n" + "="*50 + "\nInspection complete.")

if __name__ == '__main__':
    main()
