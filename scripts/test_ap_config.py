import paramiko
import time

s = paramiko.SSHClient()
s.set_missing_host_key_policy(paramiko.AutoAddPolicy())
s.connect("192.168.2.1", username="root", password="root")

cmds = [
    "uci set wireless.default_radio0.ssid='r3pminisever'",
    "uci set wireless.default_radio0.encryption='psk2'",
    "uci set wireless.default_radio0.key='admin1234'",
    "uci set wireless.default_radio0.disabled='0'",
    "uci set wireless.radio0.disabled='0'",
    "uci commit wireless",
    "wifi reload",
]

for c in cmds:
    s.exec_command(c)

time.sleep(3)

_, o, _ = s.exec_command("uci show wireless.default_radio0; iwinfo phy0 info; iwinfo phy0-ap0 info 2>/dev/null || iwinfo wlan0 info 2>/dev/null")
print(o.read().decode())
s.close()
