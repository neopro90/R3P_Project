import urllib.request, ssl, re

ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

req = urllib.request.Request("http://192.168.2.1/cgi-bin/luci/", headers={'User-Agent': 'Mozilla/5.0'})
try:
    with urllib.request.urlopen(req, timeout=5, context=ctx) as r:
        html = r.read().decode('utf-8', errors='replace')
        title = re.findall(r'<title>(.*?)</title>', html, re.I)
        print("Title:", title)
        for m in re.finditer(r'(OpenWrt [^\<\>\n]+)', html):
            print("Version match:", m.group(1))
        # check if there is an error message or auth token
        print("Body snippet:")
        print(html[:1500])
except Exception as e:
    print("Error:", e)
