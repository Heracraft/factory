t=$1
f="$t/git/config"
for k in "$t"/git/check-*.key; do
  [ -e "$k" ] || continue
  b=${k%.key}
  key=$(cat "$k"); kind=$(cat "$b.kind"); val=$(cat "$b.val")
  ok=
  case $kind in
    path)
      p=$val
      case $p in "~/"*) p="$HOME/${p#"~/"}" ;; esac
      [ -e "$p" ] && ok=1 ;;
    cmd)
      c=${val%%[[:space:]]*}
      case $c in *=*) ok=1 ;; *) command -v "$c" >/dev/null 2>&1 && ok=1 ;; esac ;;
  esac
  if [ -z "$ok" ]; then
    git config --file "$f" --unset-all "$key" || true
    echo "#dropped git $key"
  fi
done
mkdir -p ~/.config/git
cp "$f" ~/.config/git/repose-carried.new
mv -f ~/.config/git/repose-carried.new ~/.config/git/repose-carried
if [ -f "$t/git/ignore" ]; then
  cp "$t/git/ignore" ~/.config/git/ignore.new
  mv -f ~/.config/git/ignore.new ~/.config/git/ignore
fi
g="$HOME/.gitconfig"
if [ -L "$g" ]; then
  echo '#warn ~/.gitconfig is a link, so your laptop git config was not included; add "[include] path = ~/.config/git/repose-carried" to it.'
elif ! git config --file "$g" --get-all include.path 2>/dev/null | grep -qxF '~/.config/git/repose-carried'; then
  { printf '[include]\n\tpath = ~/.config/git/repose-carried\n'; cat "$g" 2>/dev/null || true; } > "$g.repose-new"
  mv -f "$g.repose-new" "$g"
  for k in user.name user.email; do
    c=$(git config --file ~/.config/git/repose-carried --get "$k" || true)
    o=$(git config --file "$g" --get "$k" || true)
    if [ -n "$o" ] && [ "$o" = "$c" ]; then git config --file "$g" --unset "$k" || true; fi
  done
fi
