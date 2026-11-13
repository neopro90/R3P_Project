import paramiko

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect("192.168.2.1", username="root", password="root", timeout=8)

cmds = [
    # 1. Add firewall rules to allow ports 8443 and 80 from WAN/WWAN
    "uci set firewall.allow_wan_8443=rule",
    "uci set firewall.allow_wan_8443.name='Allow-WAN-HTTPS-8443'",
    "uci set firewall.allow_wan_8443.src='wan'",
    "uci set firewall.allow_wan_8443.proto='tcp'",
    "uci set firewall.allow_wan_8443.dest_port='8443'",
    "uci set firewall.allow_wan_8443.target='ACCEPT'",

    "uci -q delete firewall.allow_wan_8787 || true",

    # Clean bloated zone 1 network list
    "uci set firewall.@zone[1].network='wan wan6 usbwan wwan'",
    "uci commit firewall",
    "/etc/init.d/firewall reload",

    # 2. Fix the leading space in DDNS config
    "sed -i 's/\"token\": \" c528/\"token\": \"c528/g' /etc/r3p/ddns.json",
    "sed -i 's/\"enabled\": false/\"enabled\": true/g' /etc/r3p/ddns.json",
]

for cmd in cmds:
    stdin, stdout, stderr = ssh.exec_command(cmd)
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    if err and "uci" not in err.lower():
        print(f"[{cmd}] error:", err)

# Check results
_, stdout, _ = ssh.exec_command("uci show firewall.allow_wan_8443; echo '---'; cat /etc/r3p/ddns.json")
print(stdout.read().decode())
ssh.close()
