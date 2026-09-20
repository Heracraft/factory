# OpenAI Codex CLI from its GitHub release: the static musl binary, so no
# patching; bubblewrap and ripgrep on PATH for its Linux sandbox and
# search. Pinned in versions.json.
{ lib, stdenvNoCC, fetchurl, makeBinaryWrapper, bubblewrap, ripgrep }:
let
  v = (builtins.fromJSON (builtins.readFile ./versions.json)).codex;
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
