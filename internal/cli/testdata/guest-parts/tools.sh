mkdir -p ~/.repose
cp "$1/tools/wanted.json" ~/.repose/tools-wanted.json.new
mv -f ~/.repose/tools-wanted.json.new ~/.repose/tools-wanted.json
if command -v repose-tools-install >/dev/null 2>&1; then
  repose-tools-install plan
fi
