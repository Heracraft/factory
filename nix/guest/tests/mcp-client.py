"""A minimal MCP stdio client for the guest-desktop test.

usage: mcp-client.py <command> [args...] -- <tool> <json-args> [<tool> <json-args> ...]

Starts the MCP server, does the initialize handshake, calls each tool in
turn and prints each result's text content to stdout, one block per call.
The pseudo-tool `!sh` with {"cmd": ...} runs a shell command between two
calls, with the same server still running. Exits 1 when a call returns
isError or the server stops answering.
"""

import json
import subprocess
import sys


def main():
    argv = sys.argv[1:]
    if "--" not in argv:
        print(__doc__, file=sys.stderr)
        sys.exit(64)
    split = argv.index("--")
    cmd, calls = argv[:split], argv[split + 1:]
    if not cmd or len(calls) % 2:
        print(__doc__, file=sys.stderr)
        sys.exit(64)
    proc = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
    next_id = [0]

    def request(method, params):
        next_id[0] += 1
        msg = {"jsonrpc": "2.0", "id": next_id[0], "method": method, "params": params}
        proc.stdin.write(json.dumps(msg) + "\n")
        proc.stdin.flush()
        while True:
            line = proc.stdout.readline()
            if not line:
                raise SystemExit(f"server exited during {method}")
            try:
                resp = json.loads(line)
            except ValueError:
                continue
            if resp.get("id") == next_id[0]:
                if "error" in resp:
                    raise SystemExit(f"{method}: {resp['error']}")
                return resp["result"]

    def notify(method):
        proc.stdin.write(json.dumps({"jsonrpc": "2.0", "method": method}) + "\n")
        proc.stdin.flush()

    request("initialize", {
        "protocolVersion": "2025-06-18",
        "capabilities": {},
        "clientInfo": {"name": "repose-test", "version": "1"},
    })
    notify("notifications/initialized")
    failed = False
    for i in range(0, len(calls), 2):
        name, args = calls[i], json.loads(calls[i + 1])
        if name == "!sh":
            subprocess.run(args["cmd"], shell=True, check=True)
            continue
        result = request("tools/call", {"name": name, "arguments": args})
        text = "\n".join(c.get("text", "") for c in result.get("content", []) if c.get("type") == "text")
        print(f"=== {name}\n{text}", flush=True)
        if result.get("isError"):
            failed = True
    proc.stdin.close()
    try:
        proc.wait(timeout=20)
    except subprocess.TimeoutExpired:
        proc.kill()
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
