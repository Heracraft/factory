#!/bin/sh
# install.sh: installs the repose CLI (docs/workstreams/07-cli.md §5.11).
# Served at https://repose.herakraft.co/install.sh.
#
#   curl -fsSL https://repose.herakraft.co/install.sh | sh
#   curl -fsSL https://repose.herakraft.co/install.sh | sh -s -- --system
#   curl -fsSL https://repose.herakraft.co/install.sh | sh -s -- --version v1.2.3
set -eu

# The GitHub repository the releases live in. It is still named `factory`
# (DECISIONS I-98); renaming it to `repose` is the owner's pending step, and
# GitHub redirects the old name afterwards, so this keeps working either way.
REPO="heracraft/factory"
BIN_NAME="repose"
INSTALL_DIR="$HOME/.local/bin"
VERSION="latest"

for arg in "$@"; do
  case "$arg" in
    --system) INSTALL_DIR="/usr/local/bin" ;;
    --version=*) VERSION="${arg#--version=}" ;;
    --version) NEED_VERSION_ARG=1 ;;
    *)
      if [ "${NEED_VERSION_ARG:-0}" = "1" ]; then
        VERSION="$arg"
        NEED_VERSION_ARG=0
      fi
      ;;
  esac
done

os_raw=$(uname -s)
arch_raw=$(uname -m)

case "$os_raw" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *)
    echo "repose: unsupported OS $os_raw (supports darwin, linux)" >&2
    exit 1
    ;;
esac

case "$arch_raw" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "repose: unsupported architecture $arch_raw (supports amd64, arm64)" >&2
    exit 1
    ;;
esac

if [ "$VERSION" = "latest" ] && [ -z "${REPOSE_INSTALL_BASE_URL:-}" ]; then
  # The archive name carries the version, so "latest" is resolved to a
  # real tag first, by following releases/latest's redirect.
  VERSION=$(curl -fsSL -o /dev/null -w '%{url_effective}' \
    "https://github.com/$REPO/releases/latest" | sed 's#.*/tag/##')
  if [ -z "$VERSION" ]; then
    echo "repose: could not resolve the latest release" >&2
    exit 1
  fi
fi

if [ -n "${REPOSE_INSTALL_BASE_URL:-}" ]; then
  # Overridable for CI and local testing; production installs never set it.
  release_url="$REPOSE_INSTALL_BASE_URL"
else
  release_url="https://github.com/$REPO/releases/download/$VERSION"
fi

archive="${BIN_NAME}_${VERSION}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $archive ($VERSION)..." >&2
curl -fsSL "$release_url/$archive" -o "$tmp/$archive"
curl -fsSL "$release_url/checksums.txt" -o "$tmp/checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  ( cd "$tmp" && grep " $archive\$" checksums.txt | sha256sum -c - ) \
    || { echo "repose: checksum verification failed" >&2; exit 1; }
else
  # macOS ships shasum, not sha256sum.
  ( cd "$tmp" && grep " $archive\$" checksums.txt | shasum -a 256 -c - ) \
    || { echo "repose: checksum verification failed" >&2; exit 1; }
fi

tar -xzf "$tmp/$archive" -C "$tmp"

if [ "$INSTALL_DIR" = "/usr/local/bin" ] && [ ! -w "$INSTALL_DIR" ]; then
  sudo mkdir -p "$INSTALL_DIR"
  sudo install -m 755 "$tmp/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
else
  mkdir -p "$INSTALL_DIR"
  install -m 755 "$tmp/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
fi

echo "Installed $INSTALL_DIR/$BIN_NAME"

# DECISIONS I-15: Arch Linux ships an unrelated `repose` binary. Put ours
# first on PATH, and say so rather than silently shadowing or losing to it.
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    shell_rc=""
    case "${SHELL:-}" in
      */zsh) shell_rc="$HOME/.zshrc" ;;
      */bash) shell_rc="$HOME/.bashrc" ;;
      *) shell_rc="$HOME/.profile" ;;
    esac
    printf '\nexport PATH="%s:$PATH"\n' "$INSTALL_DIR" >> "$shell_rc"
    echo "Added $INSTALL_DIR to PATH in $shell_rc; restart your shell or run: export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

resolved=$(command -v "$BIN_NAME" 2>/dev/null || true)
if [ -n "$resolved" ] && [ "$resolved" != "$INSTALL_DIR/$BIN_NAME" ]; then
  echo "another repose is on your PATH at $resolved; ours is at $INSTALL_DIR/$BIN_NAME"
fi

exit 0
