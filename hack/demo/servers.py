"""Two fake endpoints for trying swapsafe out.

  18080 = "ingress-nginx": ssl-redirect on, every plain HTTP request 301s.
  18081 = "Gateway":       the redirect filter was forgotten, serves 200.

Run:  python3 docs/playground/servers.py
"""
import http.server, threading, time


class Baseline(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(301)
        self.send_header("Location", "https://" + self.headers.get("Host", "") + self.path)
        self.send_header("Server", "nginx/1.25.3")
        self.send_header("Content-Length", "0")
        self.end_headers()

    def log_message(self, *a):
        pass


class Candidate(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = b"the real page"
        self.send_response(200)
        self.send_header("Content-Type", "text/html")
        self.send_header("Server", "envoy")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *a):
        pass


for port, handler in ((18080, Baseline), (18081, Candidate)):
    srv = http.server.HTTPServer(("127.0.0.1", port), handler)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    print(f"listening on 127.0.0.1:{port}  ({handler.__name__})")

print("ctrl-c to stop")
try:
    while True:
        time.sleep(1)
except KeyboardInterrupt:
    pass
