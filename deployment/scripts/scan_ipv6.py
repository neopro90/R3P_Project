import socket

ports = [22, 53, 80, 443, 8080, 8443]
for port in ports:
    s = socket.socket(socket.AF_INET6, socket.SOCK_STREAM)
    s.settimeout(1.5)
    res = s.connect_ex(('fd9d:7191:fe39::1', port))
    status = "OPEN" if res == 0 else f"Closed ({res})"
    print(f"Port {port}: {status}")
    s.close()
