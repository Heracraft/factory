# A registry.npmjs.org (and Docker Hub API root) stand-in for the
# host-caches VM test: one package
# document whose tarball URL names this server, one tarball, and a log of
# every request so the test can count what reached the "registry". Every
# answer sets a cookie, as Cloudflare does in front of the real registry.
import json
from http.server import BaseHTTPRequestHandler, HTTPServer

BASE = "http://127.0.0.1:9999"
TARBALL = b"\x1f\x8b fake tarball bytes"


class H(BaseHTTPRequestHandler):
    def do_GET(self):
        with open("/tmp/fake-registry.log", "a") as f:
            f.write(f"GET {self.path}\n")
        if self.path == "/v2/":
            # the Docker registry API root, which the mirror asks at start
            body, ctype = b"{}", "application/json"
        elif self.path == "/-/ping":
            body, ctype = b"{}", "application/json"
        elif self.path == "/left-pad":
            body = json.dumps({
                "name": "left-pad",
                "versions": {"1.0.0": {"dist": {"tarball": BASE + "/left-pad/-/left-pad-1.0.0.tgz"}}},
            }).encode()
            ctype = "application/json"
        elif self.path == "/left-pad/-/left-pad-1.0.0.tgz":
            body, ctype = TARBALL, "application/octet-stream"
        else:
            self.send_response(404)
            self.end_headers()
            return
        self.send_response(200)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        # registry.npmjs.org sits behind Cloudflare, which sets a bot
        # management cookie on every answer; nginx caches nothing that
        # sets a cookie unless told to ignore it (DECISIONS I-214).
        self.send_header("Set-Cookie", "__cf_bm=abc123; path=/; domain=.npmjs.org; HttpOnly; Secure; SameSite=None")
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *a):
        pass


HTTPServer(("127.0.0.1", 9999), H).serve_forever()
