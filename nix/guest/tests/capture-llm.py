# A stand-in model API for guest-agent-guide: every request's method, path
# and body is written to OUT_DIR/NNN.txt, and every answer is a 400, so an
# agent sends its first request (system prompt included) and gives up.
# Usage: python3 capture-llm.py PORT OUT_DIR
import http.server
import itertools
import os
import sys

out = sys.argv[2]
os.makedirs(out, exist_ok=True)
counter = itertools.count()
ERROR = (b'{"error":{"type":"invalid_request_error","message":"capture",'
         b'"code":400,"status":"INVALID_ARGUMENT"}}')


class Handler(http.server.BaseHTTPRequestHandler):
    def _handle(self):
        n = int(self.headers.get("content-length") or 0)
        body = self.rfile.read(n) if n else b""
        path = os.path.join(out, "%03d.txt" % next(counter))
        with open(path, "wb") as f:
            f.write((self.command + " " + self.path + "\n").encode() + body)
        self.send_response(400)
        self.send_header("content-type", "application/json")
        self.end_headers()
        self.wfile.write(ERROR)

    do_POST = do_GET = _handle

    def log_message(self, *args):
        pass


addr = ("127.0.0.1", int(sys.argv[1]))
http.server.ThreadingHTTPServer(addr, Handler).serve_forever()
