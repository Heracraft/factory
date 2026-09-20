# Runs inside a guest@ transient unit in the host-services VM test, as user
# hostd under the I-49 sandbox, and checks what the hypervisor can and
# cannot reach. Each line printed is asserted by the test script.
import fcntl
import os
import socket
import struct
import sys

print("uid", os.getuid(), "gid", os.getgid(), "groups", sorted(os.getgroups()))

# /dev/net/tun is in DeviceAllow and the tap is owned by hostd: TUNSETIFF
# on the existing tap-h2 (IFF_TAP | IFF_NO_PI | IFF_VNET_HDR) must work.
fd = os.open("/dev/net/tun", os.O_RDWR)
fcntl.ioctl(fd, 0x400454CA, struct.pack("16sH", b"tap-h2", 0x0002 | 0x1000 | 0x4000))
print("tap ok")

# The guest directory is bound read-write: CH creates its sockets there.
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.bind("/var/lib/repose/guests/h2/vsock.sock")
print("unix socket ok")

# RestrictAddressFamilies: no host network sockets.
try:
    socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    print("AF_INET allowed")
    sys.exit(1)
except OSError as e:
    print("AF_INET refused:", e.errno)

# TemporaryFileSystem + BindPaths: no other guest's directory is visible.
visible = os.listdir("/var/lib/repose/guests")
print("guests visible:", visible)
if visible != ["h2"]:
    sys.exit(1)

# DevicePolicy=closed: a node hostd may open by permission bits (group kvm)
# but that is not in DeviceAllow is refused by the cgroup.
try:
    os.open("/dev/vhost-vsock", os.O_RDWR)
    print("vhost-vsock allowed")
    sys.exit(1)
except OSError as e:
    print("vhost-vsock refused by DevicePolicy:", e.errno)

print("kvm", "present" if os.path.exists("/dev/kvm") else "absent")
