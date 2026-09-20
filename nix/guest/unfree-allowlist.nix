# Unfree packages a guest (and a fragment) may use, by pname
# (docs/workstreams/12-nix-config-pipeline.md "The fragment contract").
# Read by nix/guest/base/agents.nix, nix/guest/microvm.nix and nix/flake.nix
# so the three pkgs instantiations agree.
[ "claude-code" "codex" "gemini-cli" "vscode" "cursor" "terraform" "ngrok" "google-chrome" ]
