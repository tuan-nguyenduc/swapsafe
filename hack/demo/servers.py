"""Two fake endpoints for trying swapsafe out, no cluster needed.

  18080 stands in for ingress-nginx
  18081 stands in for the Gateway it was translated to

Three hostnames, three outcomes — which is the whole point of the tool:

  ok.test      both answer the same          the translation worked
  live.test    body changes every request    nothing to compare, and swapsafe says so
  admin.test   nginx 200, the Gateway 404    no HTTPRoute was written for it
  shop.test    nginx 301s, the Gateway 200s  the ssl-redirect was lost

Run:  python3 hack/demo/servers.py
"""
import http.server
import itertools
import threading
import time

BODY = b"the real page\n"
HITS = itertools.count(1)


class Handler(http.server.BaseHTTPRequestHandler):
    nginx = False

    def do_GET(self):
        host = self.headers.get("Host", "").split(":")[0]

        # No HTTPRoute was written for admin.test, so the Gateway has nothing
        # to match and answers 404, exactly as a real one does.
        if not self.nginx and host == "admin.test":
            self.send_response(404)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return

        # ingress-nginx has ssl-redirect on for shop.test; the Gateway lost it.
        if self.nginx and host == "shop.test":
            self.send_response(301)
            self.send_header("Location", "https://" + host + self.path)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return

        # live.test answers with a counter, the way a real application puts a
        # request id, a timestamp or a pod name in every response.
        if host == "live.test":
            body = f'{{"hits": {next(HITS)}}}\n'.encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return

        self.send_response(200)
        self.send_header("Content-Type", "text/html")
        self.send_header("Server", "nginx/1.25.3" if self.nginx else "envoy")
        self.send_header("Content-Length", str(len(BODY)))
        self.end_headers()
        self.wfile.write(BODY)

    def log_message(self, *args):
        pass


class Nginx(Handler):
    nginx = True


class Gateway(Handler):
    nginx = False


for port, handler in ((18080, Nginx), (18081, Gateway)):
    server = http.server.HTTPServer(("127.0.0.1", port), handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    print(f"listening on 127.0.0.1:{port}  ({handler.__name__})")

print("ctrl-c to stop")
try:
    while True:
        time.sleep(1)
except KeyboardInterrupt:
    pass
