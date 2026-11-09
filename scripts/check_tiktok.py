import paramiko

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect('192.168.1.7', username='root', password='root')

cmds = [
    "nft list table inet fw4 2>/dev/null | grep -E '443|drop|reject' | head -n 30",
    "cat /proc/sys/net/netfilter/nf_conntrack_count /proc/sys/net/netfilter/nf_conntrack_max"
]
for c in cmds:
    print('=== ' + c)
    _, out, err = client.exec_command(c)
    print(out.read().decode())
