import getpass
import json
import os
import paramiko

host = os.environ.get("R3P_HOST", "192.168.1.7")
user = os.environ.get("R3P_SSH_USER", "root")
password = os.environ.get("R3P_SSH_PASS") or getpass.getpass(f"SSH password for {user}@{host}: ")

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.RejectPolicy())
ssh.load_system_host_keys()
ssh.connect(host, username=user, password=password, timeout=8)

# Normalize whitespace in /etc/r3p/ddns.json safely via Python stdin
_, out, err = ssh.exec_command("cat /etc/r3p/ddns.json")
raw_cfg = out.read().decode("utf-8", errors="replace").strip()
if raw_cfg:
    cfg = json.loads(raw_cfg)
    if isinstance(cfg.get("token"), str):
        cfg["token"] = cfg["token"].strip()
    if isinstance(cfg.get("domain"), str):
        cfg["domain"] = cfg["domain"].strip()
    sftp = ssh.open_sftp()
    with sftp.file("/etc/r3p/ddns.json", "w") as f:
        json.dump(cfg, f, indent=2)
    sftp.close()
    print("Updated /etc/r3p/ddns.json safely")
ssh.close()
