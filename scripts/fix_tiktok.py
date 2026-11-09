import paramiko

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect('192.168.1.7', username='root', password='root')

cmds = [
    # Trigger adblock rebuild
    "/usr/bin/r3p-api -rebuild-adblock 2>/dev/null || true",
    "uci set dhcp.@dnsmasq[0].filter_aaaa=1",
    "uci set dhcp.lan.dhcpv6=disabled",
    "uci set dhcp.lan.ra=disabled",
    "uci commit dhcp",
    "/etc/init.d/dnsmasq restart",
    "/etc/init.d/odhcpd restart",
    "nslookup api-h2.tiktokv.com 127.0.0.1",
    "nslookup log.byteoversea.com 127.0.0.1",
    "nslookup mon.byteoversea.com 127.0.0.1",
    "nslookup v16-webapp-prime.tiktokcdn.com 127.0.0.1",
    "grep -rn 'byteoversea' /tmp/dnsmasq.d/ /tmp/r3p-adblock.hosts 2>/dev/null || echo 'Clean!'"
]
for c in cmds:
    print('=== CMD: ' + c)
    _, out, err = client.exec_command(c)
    print(out.read().decode())
