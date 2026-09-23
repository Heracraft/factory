z='Asia/Tokyo'
f=/etc/repose/env
if [ -f "$f" ] && ! grep -qxF "TZ=$z" "$f"; then
  { grep -v '^TZ=' "$f" || true; printf 'TZ=%s\n' "$z"; } > "$1/env.new"
  sudo -n sh -c 'cat > /etc/repose/env.repose-new && chmod 0644 /etc/repose/env.repose-new && mv -f /etc/repose/env.repose-new /etc/repose/env' < "$1/env.new"
  echo "#tz $z"
fi
if tmux list-sessions >/dev/null 2>&1; then
  tmux set-environment -g TZ "$z"
  tmux list-sessions -F '#{session_name}' | while IFS= read -r s; do tmux set-environment -t "=$s" TZ "$z"; done
fi
