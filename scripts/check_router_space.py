import paramiko

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect('192.168.2.1', username='root', password='root')

py_cmd = r"""python3 -c "
import os, glob
for path in sorted(glob.glob('/proc/*/fd/*')):
    try:
        target = os.readlink(path)
        if '(deleted)' in target:
            size = os.stat(path).st_size
            if size > 1024:
                print(f'{size/1024/1024:8.2f} MB: {path} -> {target}')
    except Exception:
        pass
"
"""

_, stdout, _ = ssh.exec_command(py_cmd)
print("Deleted open files:")
print(stdout.read().decode())

_, stdout, _ = ssh.exec_command("du -k -d 1 / 2>/dev/null | sort -n")
print("du of /:")
print(stdout.read().decode())
