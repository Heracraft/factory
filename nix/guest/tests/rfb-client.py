#!/usr/bin/env python3
"""A small RFB 3.8 client for the guest-desktop VM test (I-292).

It does what the viewer page does, without a browser: VncAuth with the
password (DES on the server's challenge, with VNC's reversed key bits),
ClientInit, SetEncodings (raw, ExtendedDesktopSize, DesktopSize), one full
FramebufferUpdate, and with --resize a SetDesktopSize request whose answer
it checks. It prints one JSON object and exits 0, or raises.

Standard library only: python3 in the guest has no DES, so a plain DES
encryptor is here (FIPS 46-3 tables; encryption only; the test vector
133457799BBCDFF1 / 0123456789ABCDEF -> 85E813540F0AB405 is checked at
import).
"""
import argparse
import json
import socket
import struct
import sys

# --- DES (encrypt only) -------------------------------------------------

IP = [58, 50, 42, 34, 26, 18, 10, 2, 60, 52, 44, 36, 28, 20, 12, 4,
      62, 54, 46, 38, 30, 22, 14, 6, 64, 56, 48, 40, 32, 24, 16, 8,
      57, 49, 41, 33, 25, 17, 9, 1, 59, 51, 43, 35, 27, 19, 11, 3,
      61, 53, 45, 37, 29, 21, 13, 5, 63, 55, 47, 39, 31, 23, 15, 7]
FP = [40, 8, 48, 16, 56, 24, 64, 32, 39, 7, 47, 15, 55, 23, 63, 31,
      38, 6, 46, 14, 54, 22, 62, 30, 37, 5, 45, 13, 53, 21, 61, 29,
      36, 4, 44, 12, 52, 20, 60, 28, 35, 3, 43, 11, 51, 19, 59, 27,
      34, 2, 42, 10, 50, 18, 58, 26, 33, 1, 41, 9, 49, 17, 57, 25]
E = [32, 1, 2, 3, 4, 5, 4, 5, 6, 7, 8, 9, 8, 9, 10, 11, 12, 13,
     12, 13, 14, 15, 16, 17, 16, 17, 18, 19, 20, 21, 20, 21, 22, 23, 24, 25,
     24, 25, 26, 27, 28, 29, 28, 29, 30, 31, 32, 1]
P = [16, 7, 20, 21, 29, 12, 28, 17, 1, 15, 23, 26, 5, 18, 31, 10,
     2, 8, 24, 14, 32, 27, 3, 9, 19, 13, 30, 6, 22, 11, 4, 25]
PC1 = [57, 49, 41, 33, 25, 17, 9, 1, 58, 50, 42, 34, 26, 18, 10, 2,
       59, 51, 43, 35, 27, 19, 11, 3, 60, 52, 44, 36, 63, 55, 47, 39,
       31, 23, 15, 7, 62, 54, 46, 38, 30, 22, 14, 6, 61, 53, 45, 37,
       29, 21, 13, 5, 28, 20, 12, 4]
PC2 = [14, 17, 11, 24, 1, 5, 3, 28, 15, 6, 21, 10, 23, 19, 12, 4,
       26, 8, 16, 7, 27, 20, 13, 2, 41, 52, 31, 37, 47, 55, 30, 40,
       51, 45, 33, 48, 44, 49, 39, 56, 34, 53, 46, 42, 50, 36, 29, 32]
SHIFTS = [1, 1, 2, 2, 2, 2, 2, 2, 1, 2, 2, 2, 2, 2, 2, 1]
SBOX = [
    [14, 4, 13, 1, 2, 15, 11, 8, 3, 10, 6, 12, 5, 9, 0, 7,
     0, 15, 7, 4, 14, 2, 13, 1, 10, 6, 12, 11, 9, 5, 3, 8,
     4, 1, 14, 8, 13, 6, 2, 11, 15, 12, 9, 7, 3, 10, 5, 0,
     15, 12, 8, 2, 4, 9, 1, 7, 5, 11, 3, 14, 10, 0, 6, 13],
    [15, 1, 8, 14, 6, 11, 3, 4, 9, 7, 2, 13, 12, 0, 5, 10,
     3, 13, 4, 7, 15, 2, 8, 14, 12, 0, 1, 10, 6, 9, 11, 5,
     0, 14, 7, 11, 10, 4, 13, 1, 5, 8, 12, 6, 9, 3, 2, 15,
     13, 8, 10, 1, 3, 15, 4, 2, 11, 6, 7, 12, 0, 5, 14, 9],
    [10, 0, 9, 14, 6, 3, 15, 5, 1, 13, 12, 7, 11, 4, 2, 8,
     13, 7, 0, 9, 3, 4, 6, 10, 2, 8, 5, 14, 12, 11, 15, 1,
     13, 6, 4, 9, 8, 15, 3, 0, 11, 1, 2, 12, 5, 10, 14, 7,
     1, 10, 13, 0, 6, 9, 8, 7, 4, 15, 14, 3, 11, 5, 2, 12],
    [7, 13, 14, 3, 0, 6, 9, 10, 1, 2, 8, 5, 11, 12, 4, 15,
     13, 8, 11, 5, 6, 15, 0, 3, 4, 7, 2, 12, 1, 10, 14, 9,
     10, 6, 9, 0, 12, 11, 7, 13, 15, 1, 3, 14, 5, 2, 8, 4,
     3, 15, 0, 6, 10, 1, 13, 8, 9, 4, 5, 11, 12, 7, 2, 14],
    [2, 12, 4, 1, 7, 10, 11, 6, 8, 5, 3, 15, 13, 0, 14, 9,
     14, 11, 2, 12, 4, 7, 13, 1, 5, 0, 15, 10, 3, 9, 8, 6,
     4, 2, 1, 11, 10, 13, 7, 8, 15, 9, 12, 5, 6, 3, 0, 14,
     11, 8, 12, 7, 1, 14, 2, 13, 6, 15, 0, 9, 10, 4, 5, 3],
    [12, 1, 10, 15, 9, 2, 6, 8, 0, 13, 3, 4, 14, 7, 5, 11,
     10, 15, 4, 2, 7, 12, 9, 5, 6, 1, 13, 14, 0, 11, 3, 8,
     9, 14, 15, 5, 2, 8, 12, 3, 7, 0, 4, 10, 1, 13, 11, 6,
     4, 3, 2, 12, 9, 5, 15, 10, 11, 14, 1, 7, 6, 0, 8, 13],
    [4, 11, 2, 14, 15, 0, 8, 13, 3, 12, 9, 7, 5, 10, 6, 1,
     13, 0, 11, 7, 4, 9, 1, 10, 14, 3, 5, 12, 2, 15, 8, 6,
     1, 4, 11, 13, 12, 3, 7, 14, 10, 15, 6, 8, 0, 5, 9, 2,
     6, 11, 13, 8, 1, 4, 10, 7, 9, 5, 0, 15, 14, 2, 3, 12],
    [13, 2, 8, 4, 6, 15, 11, 1, 10, 9, 3, 14, 5, 0, 12, 7,
     1, 15, 13, 8, 10, 3, 7, 4, 12, 5, 6, 11, 0, 14, 9, 2,
     7, 11, 4, 1, 9, 12, 14, 2, 0, 6, 10, 13, 15, 3, 5, 8,
     2, 1, 14, 7, 4, 10, 8, 13, 15, 12, 9, 0, 3, 5, 6, 11],
]


def bits(data, n):
    return [(int.from_bytes(data, "big") >> (n - 1 - i)) & 1 for i in range(n)]


def unbits(b):
    out = 0
    for x in b:
        out = (out << 1) | x
    return out.to_bytes(len(b) // 8, "big")


def permute(b, table):
    return [b[i - 1] for i in table]


def rotl(b, n):
    return b[n:] + b[:n]


def subkeys(key):
    k = permute(bits(key, 64), PC1)
    c, d = k[:28], k[28:]
    out = []
    for s in SHIFTS:
        c, d = rotl(c, s), rotl(d, s)
        out.append(permute(c + d, PC2))
    return out


def des_encrypt_block(key, block):
    b = permute(bits(block, 64), IP)
    l, r = b[:32], b[32:]
    for k in subkeys(key):
        x = [a ^ c for a, c in zip(permute(r, E), k)]
        s = []
        for i in range(8):
            chunk = x[i * 6:(i + 1) * 6]
            row = (chunk[0] << 1) | chunk[5]
            col = (chunk[1] << 3) | (chunk[2] << 2) | (chunk[3] << 1) | chunk[4]
            v = SBOX[i][row * 16 + col]
            s += [(v >> 3) & 1, (v >> 2) & 1, (v >> 1) & 1, v & 1]
        f = permute(s, P)
        l, r = r, [a ^ c for a, c in zip(l, f)]
    return unbits(permute(r + l, FP))


assert des_encrypt_block(bytes.fromhex("133457799BBCDFF1"),
                         bytes.fromhex("0123456789ABCDEF")) == bytes.fromhex("85E813540F0AB405")


def vnc_auth_response(password, challenge):
    # VNC uses the password's first 8 bytes, each with its bits reversed,
    # as the DES key, and encrypts the 16-byte challenge in two blocks.
    pw = password.encode()[:8].ljust(8, b"\0")
    key = bytes(int(f"{b:08b}"[::-1], 2) for b in pw)
    return des_encrypt_block(key, challenge[:8]) + des_encrypt_block(key, challenge[8:])


# --- RFB ------------------------------------------------------------------

ENC_RAW = 0
ENC_DESKTOP_SIZE = -223
ENC_EXTENDED_DESKTOP_SIZE = -308


def recv_exact(sock, n):
    buf = b""
    while len(buf) < n:
        chunk = sock.recv(n - len(buf))
        if not chunk:
            raise EOFError(f"server closed with {len(buf)} of {n} bytes")
        buf += chunk
    return buf


class Client:
    def __init__(self, host, port, password):
        self.sock = socket.create_connection((host, port), timeout=60)
        self.result = {}
        self.screens = []
        self._handshake(password)

    def _handshake(self, password):
        s = self.sock
        version = recv_exact(s, 12)
        if not version.startswith(b"RFB 003."):
            raise RuntimeError(f"not RFB: {version!r}")
        self.result["server_version"] = version.decode().strip()
        s.sendall(b"RFB 003.008\n")
        n = recv_exact(s, 1)[0]
        if n == 0:
            ln = struct.unpack(">I", recv_exact(s, 4))[0]
            raise RuntimeError("server refused: " + recv_exact(s, ln).decode(errors="replace"))
        types = list(recv_exact(s, n))
        self.result["security_types"] = types
        if 2 not in types:
            raise RuntimeError(f"no VncAuth offered: {types}")
        s.sendall(b"\x02")
        challenge = recv_exact(s, 16)
        s.sendall(vnc_auth_response(password, challenge))
        status = struct.unpack(">I", recv_exact(s, 4))[0]
        if status != 0:
            ln = struct.unpack(">I", recv_exact(s, 4))[0]
            reason = recv_exact(s, ln).decode(errors="replace")
            raise RuntimeError(f"auth failed ({status}): {reason}")
        self.result["auth"] = "ok"
        s.sendall(b"\x01")  # ClientInit: shared
        w, h = struct.unpack(">HH", recv_exact(s, 4))
        pf = recv_exact(s, 16)
        self.bpp = pf[0] // 8
        self.depth = pf[1]
        ln = struct.unpack(">I", recv_exact(s, 4))[0]
        name = recv_exact(s, ln).decode(errors="replace")
        self.width, self.height = w, h
        self.result.update({"width": w, "height": h, "bpp": pf[0], "depth": pf[1], "name": name})
        self.set_encodings([ENC_RAW, ENC_EXTENDED_DESKTOP_SIZE, ENC_DESKTOP_SIZE])

    def set_encodings(self, encs):
        msg = struct.pack(">BxH", 2, len(encs)) + b"".join(struct.pack(">i", e) for e in encs)
        self.sock.sendall(msg)

    def request_update(self, incremental):
        self.sock.sendall(struct.pack(">BBHHHH", 3, 1 if incremental else 0, 0, 0, self.width, self.height))

    def read_message(self):
        """Read one server message. Returns ("update", rects) for a
        FramebufferUpdate, or (kind, None) for the others it skips."""
        s = self.sock
        t = recv_exact(s, 1)[0]
        if t == 0:
            recv_exact(s, 1)
            n = struct.unpack(">H", recv_exact(s, 2))[0]
            rects = []
            for _ in range(n):
                x, y, w, h, enc = struct.unpack(">HHHHi", recv_exact(s, 12))
                r = {"x": x, "y": y, "w": w, "h": h, "enc": enc}
                if enc == ENC_RAW:
                    r["bytes"] = len(recv_exact(s, w * h * self.bpp))
                elif enc == ENC_EXTENDED_DESKTOP_SIZE:
                    count = recv_exact(s, 4)[0]
                    screens = []
                    for _ in range(count):
                        sid, sx, sy, sw, sh, flags = struct.unpack(">IHHHHI", recv_exact(s, 16))
                        screens.append({"id": sid, "x": sx, "y": sy, "w": sw, "h": sh, "flags": flags})
                    r["screens"] = screens
                    # x is the reason (0 server, 1 this client, 2 another
                    # client), y the result for a request of ours.
                    if x != 1 or y == 0:
                        self.width, self.height = w, h
                        self.screens = screens
                elif enc == ENC_DESKTOP_SIZE:
                    self.width, self.height = w, h
                else:
                    raise RuntimeError(f"unexpected encoding {enc}")
                rects.append(r)
            return "update", rects
        if t == 1:  # SetColourMapEntries
            recv_exact(s, 1)
            first, n = struct.unpack(">HH", recv_exact(s, 4))
            recv_exact(s, n * 6)
            return "colourmap", None
        if t == 2:  # Bell
            return "bell", None
        if t == 3:  # ServerCutText
            recv_exact(s, 3)
            ln = struct.unpack(">i", recv_exact(s, 4))[0]
            recv_exact(s, abs(ln))
            return "cuttext", None
        raise RuntimeError(f"unexpected server message type {t}")

    def full_update(self):
        """One non-incremental update: the whole framebuffer, raw. The
        server answers a non-incremental request at once with the screen
        layout alone (an ExtendedDesktopSize rectangle) and sends the
        pixels for the next request, as it does for the viewer page, so
        an incremental request follows every update without pixels."""
        self.request_update(False)
        raw = 0
        rects = []
        for _ in range(50):
            kind, rs = self.read_message()
            if kind != "update":
                continue
            rects += rs
            raw += sum(r.get("bytes", 0) for r in rs)
            if raw >= self.width * self.height * self.bpp:
                break
            self.request_update(True)
        return rects, raw

    def set_desktop_size(self, w, h):
        if not self.screens:
            raise RuntimeError("no ExtendedDesktopSize from the server yet")
        first = self.screens[0]
        msg = struct.pack(">BxHHBx", 251, w, h, 1)
        msg += struct.pack(">IHHHHI", first["id"], 0, 0, w, h, first["flags"])
        self.sock.sendall(msg)
        # The answer is an ExtendedDesktopSize rectangle with reason 1;
        # updates for other reasons may come in between.
        self.request_update(True)
        for _ in range(50):
            kind, rects = self.read_message()
            if kind != "update":
                continue
            for r in rects:
                if r["enc"] == ENC_EXTENDED_DESKTOP_SIZE and r["x"] == 1:
                    return {"result": r["y"], "w": r["w"], "h": r["h"], "screens": r["screens"]}
        raise RuntimeError("no answer to SetDesktopSize")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--host", default="127.0.0.1")
    ap.add_argument("--port", type=int, default=5900)
    ap.add_argument("--password", required=True)
    ap.add_argument("--resize", help="WxH to ask the server for with SetDesktopSize")
    a = ap.parse_args()

    c = Client(a.host, a.port, a.password)
    rects, raw = c.full_update()
    c.result["first_update"] = {"rects": len(rects), "raw_bytes": raw,
                                "encodings": sorted({r["enc"] for r in rects})}
    c.result["screens"] = c.screens
    if a.resize:
        w, h = (int(v) for v in a.resize.lower().split("x"))
        c.result["resize"] = c.set_desktop_size(w, h)
        if c.result["resize"]["result"] == 0:
            c.width, c.height = w, h
            rects, raw = c.full_update()
            c.result["resize"]["update_raw_bytes"] = raw
    c.result["final"] = {"width": c.width, "height": c.height}
    print(json.dumps(c.result))


if __name__ == "__main__":
    main()
