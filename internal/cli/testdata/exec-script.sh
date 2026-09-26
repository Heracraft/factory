cd ~/todo-app 2>/dev/null || { echo "repose: ~/todo-app does not exist on the machine yet; running in ~" >&2; cd ~; }
[ -r /etc/profile.d/repose.sh ] && . /etc/profile.d/repose.sh
if [ -r /etc/repose/devshell.sh ]; then . /etc/repose/devshell.sh; _repose_devshell 'sh'; unset -f _repose_devshell
elif command -v direnv >/dev/null 2>&1; then eval "$(direnv export bash 2>/dev/null)"; fi
exec 'sh' '-c' 'echo "flake=$REPOSE_FLAKE_PROBE project=$REPOSE_PROJECT pwd=$PWD"; flake-tool'
