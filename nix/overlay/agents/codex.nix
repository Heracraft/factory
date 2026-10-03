# OpenAI Codex CLI from its GitHub release: the static musl binary, so no
# patching; bubblewrap and ripgrep on PATH for its Linux sandbox and
# search. Pinned in versions.json.
#
# Codex runs every shell command through codex-code-mode-host, a separate
# release asset it looks for next to its own executable. Without it each
# command fails with "failed to spawn code-mode host" (DECISIONS I-426).
{ lib, stdenvNoCC, fetchurl, makeBinaryWrapper, bubblewrap, ripgrep }:
let
  v = (builtins.fromJSON (builtins.readFile ./versions.json)).codex;
  host = fetchurl { inherit (v.code-mode-host) url hash; };
in
stdenvNoCC.mkDerivation {
  pname = "codex";
  inherit (v) version;
  src = fetchurl { inherit (v.x86_64-linux) url hash; };
  sourceRoot = ".";
  dontBuild = true;
  nativeBuildInputs = [ makeBinaryWrapper ];
  installPhase = ''
    runHook preInstall
    install -Dm755 codex-x86_64-unknown-linux-musl $out/bin/codex
    tar xzf ${host} codex-code-mode-host-x86_64-unknown-linux-musl
    install -Dm755 codex-code-mode-host-x86_64-unknown-linux-musl $out/bin/codex-code-mode-host
    wrapProgram $out/bin/codex --prefix PATH : ${lib.makeBinPath [ bubblewrap ripgrep ]}
    runHook postInstall
  '';
  meta = {
    description = "OpenAI Codex CLI";
    homepage = "https://github.com/openai/codex";
    license = lib.licenses.asl20;
    mainProgram = "codex";
    platforms = [ "x86_64-linux" ];
  };
}
