import urllib.request
import ssl
import json
import sys

ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

def test():
    # 1. Login
    req = urllib.request.Request(
        'https://192.168.2.1:8443/api/v1/auth/session', 
        data=json.dumps({'username':'root','password':'root'}).encode(), 
        headers={'Content-Type':'application/json'}
    )
    res = urllib.request.urlopen(req, context=ctx, timeout=5)
    cookie = res.headers.get('Set-Cookie').split(';')[0]
    print('[OK] Auth cookie obtained:', cookie[:25] + '...', flush=True)

    def get(url):
        r = urllib.request.Request(url, headers={'Cookie': cookie})
        resp = urllib.request.urlopen(r, context=ctx, timeout=5)
        return resp.status, json.loads(resp.read().decode())

    # 2. Explorer
    st, d = get('https://192.168.2.1:8443/api/v1/explorer/list?path=/mnt/sda1')
    entries = d.get('data', {}).get('entries', [])
    print(f'[OK] Explorer /mnt/sda1 -> Status {st}, entries: {len(entries)}', flush=True)

    # 3. Wireless Saved
    st, d = get('https://192.168.2.1:8443/api/v1/wireless/saved')
    nets = d.get('data', {}).get('networks', [])
    print(f'[OK] Wireless Saved -> Status {st}, saved networks: {len(nets)}', flush=True)

    # 4. AI Sessions & Blueprint
    st, d = get('https://192.168.2.1:8443/api/v1/ai/sessions')
    sess = d.get('data', {}).get('sessions', [])
    print(f'[OK] AI Sessions -> Status {st}, active sessions: {len(sess)}', flush=True)

    st, d = get('https://192.168.2.1:8443/api/v1/ai/blueprint')
    title = d.get('data', {}).get('title', 'N/A')
    print(f'[OK] AI Blueprint -> Status {st}, blueprint: {title}', flush=True)

    # 5. AdBlock Streams
    st, d = get('https://192.168.2.1:8443/api/v1/adblock/streams')
    tot = d.get('data', {}).get('total', 0)
    print(f'[OK] AdBlock Streams -> Status {st}, tracked streams: {tot}', flush=True)

    # 6. ESP32 Download Localhost API
    req_esp = urllib.request.Request('http://192.168.2.1:8787/api/local/status')
    resp_esp = urllib.request.urlopen(req_esp, timeout=10)
    d_esp = json.loads(resp_esp.read().decode())
    host_addr = d_esp.get('host', {}).get('address')
    print(f'[OK] ESP32-Download :8787 -> Host Address: {host_addr}', flush=True)

if __name__ == '__main__':
    test()
