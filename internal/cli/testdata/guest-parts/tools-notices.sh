f=~/.repose/tools-notices
if [ -s "$f" ]; then
  mv -f "$f" "$f.sent"
  sed 's/^/#warn /' "$f.sent"
  rm -f "$f.sent"
fi
