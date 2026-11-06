import paramiko

s = paramiko.SSHClient()
s.set_missing_host_key_policy(paramiko.AutoAddPolicy())
s.connect("192.168.2.1", username="root", password="root")
cmd = """
printf "interface=test\\nwpa=2\\nwpa_passphrase=admin\\nwpa_key_mgmt=WPA-PSK\\n" > /tmp/test.conf
/usr/sbin/hostapd /tmp/test.conf 2>&1
rm -f /tmp/test.conf
"""
_, o, _ = s.exec_command(cmd)
print(o.read().decode())
s.close()
