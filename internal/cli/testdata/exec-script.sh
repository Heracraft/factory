repose_n=
[ -f ~/.repose/checkout ] && IFS= read -r repose_n < ~/.repose/checkout || true
case "$repose_n" in ""|.*|*/*) repose_n= ;; esac
if [ -n "$repose_n" ] && [ -d "$HOME/$repose_n" ]; then repose_co="$HOME/$repose_n"
elif [ -d "$HOME/todo-app" ]; then repose_co="$HOME/todo-app"
else repose_co="$HOME"; fi
cd "$repose_co"
[ -r /etc/profile.d/repose.sh ] && . /etc/profile.d/repose.sh
if [ -r /etc/repose/devshell.sh ]; then . /etc/repose/devshell.sh; REPOSE_DEVSHELL_QUIET=1 _repose_devshell 'sh'; unset -f _repose_devshell _repose_devshell_done
elif command -v direnv >/dev/null 2>&1; then eval "$(direnv export bash 2>/dev/null)"; fi
exec 'sh' '-c' 'echo "flake=$REPOSE_FLAKE_PROBE project=$REPOSE_PROJECT pwd=$PWD"; flake-tool'
