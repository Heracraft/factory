#!/usr/bin/env python3
"""latency-proxy.py: add a laptop's round-trip time to one process's
connections, without touching the machine's network (startup-bench.sh).

  latency-proxy.py stdio RTT_MS HOST PORT     ssh ProxyCommand: stdin/stdout <-> HOST:PORT
  latency-proxy.py http RTT_MS LISTEN_PORT    HTTP CONNECT proxy for HTTPS_PROXY

Each chunk is forwarded RTT/2 after it arrives, in both directions, in
order, and a new connection costs one RTT before its first byte (the TCP
handshake), so TLS and SSH handshakes pay their real number of round
trips. Bandwidth is not limited.
"""
import asyncio
import os
import sys


async def pipe(reader, write, drain, delay):
    """Forward reader to write, each chunk delay seconds after it arrived."""
    q = asyncio.Queue()
    loop = asyncio.get_running_loop()

    async def sender():
        while True:
            due, data = await q.get()
            wait = due - loop.time()
            if wait > 0:
                await asyncio.sleep(wait)
            if data is None:
                return
            write(data)
            await drain()

    t = asyncio.create_task(sender())
    while True:
        data = await reader.read(65536)
        await q.put((loop.time() + delay, data or None))
        if not data:
            break
    await t


async def splice(r1, w1, r2, w2, delay):
    async def one(r, w):
        try:
            await pipe(r, w.write, w.drain, delay)
        finally:
            try:
                w.write_eof()
            except Exception:
                pass

    await asyncio.gather(one(r1, w2), one(r2, w1), return_exceptions=True)
    for w in (w1, w2):
        w.close()


async def stdio(rtt, host, port):
    loop = asyncio.get_running_loop()
    await asyncio.sleep(rtt)  # TCP handshake
    r, w = await asyncio.open_connection(host, port)
    sin = asyncio.StreamReader()
    await loop.connect_read_pipe(lambda: asyncio.StreamReaderProtocol(sin), sys.stdin)
    out_fd = sys.stdout.fileno()

    def write_out(data):
        os.write(out_fd, data)

    async def nodrain():
        pass

    async def up():
        await pipe(sin, w.write, w.drain, rtt / 2)
        try:
            w.write_eof()
        except Exception:
            pass

    async def down():
        await pipe(r, write_out, nodrain, rtt / 2)

    await asyncio.gather(up(), down(), return_exceptions=True)


async def http(rtt, listen):
    async def handle(cr, cw):
        line = await cr.readline()
        while (await cr.readline()) not in (b"\r\n", b"\n", b""):
            pass
        parts = line.split()
        if len(parts) < 2 or parts[0] != b"CONNECT":
            cw.write(b"HTTP/1.1 405 CONNECT only\r\n\r\n")
            cw.close()
            return
        host, _, port = parts[1].decode().rpartition(":")
        await asyncio.sleep(rtt)  # TCP handshake to the far side
        try:
            ur, uw = await asyncio.open_connection(host, int(port))
        except OSError:
            cw.write(b"HTTP/1.1 502 Bad Gateway\r\n\r\n")
            cw.close()
            return
        cw.write(b"HTTP/1.1 200 Connection established\r\n\r\n")
        await cw.drain()
        await splice(cr, cw, ur, uw, rtt / 2)

    srv = await asyncio.start_server(handle, "127.0.0.1", listen)
    async with srv:
        await srv.serve_forever()


def main():
    mode, rtt = sys.argv[1], int(sys.argv[2]) / 1000.0
    if mode == "stdio":
        asyncio.run(stdio(rtt, sys.argv[3], int(sys.argv[4])))
    elif mode == "http":
        asyncio.run(http(rtt, int(sys.argv[3])))
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
