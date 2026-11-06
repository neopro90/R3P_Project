import paramiko
import sys

passwords = ['root', '', 'admin', 'password', '12345678', 'admin123', 'xiaomi', '123456', 'root123']
for p in passwords:
    c = paramiko.SSHClient()
    c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    try:
        c.connect('192.168.2.1', port=22, username='root', password=p, timeout=3, allow_agent=False, look_for_keys=False)
        print(f"SUCCESS with password: '{p}'")
        c.close()
        sys.exit(0)
    except paramiko.AuthenticationException:
        print(f"Failed with password: '{p}'")
    except Exception as e:
        print(f"Error with '{p}': {e}")
print("All tested passwords failed.")
