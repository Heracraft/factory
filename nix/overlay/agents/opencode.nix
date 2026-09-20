# opencode from its GitHub release (a bun-compiled, dynamically linked
# binary), pinned in versions.json.
{ lib, stdenvNoCC, fetchurl, autoPatchelfHook, makeBinaryWrapper, ripgrep }:
let
  v = (builtins.fromJSON (builtins.readFile ./versions.json)).opencode;
in
stdenvNoCC.mkDerivation {
  pname = "opencode";
  inherit (v) version;
  src = fetchurl { inherit (v.x86_64-linux) url hash; };
  sourceRoot = ".";
  dontBuild = true;
  dontStrip = true;
  nativeBuildInputs = [ autoPatchelfHook makeBinaryWrapper ];
  installPhase = ''
    runHook preInstall
    install -Dm755 opencode $out/bin/opencode
    wrapProgram $out/bin/opencode \
      --set OPENCODE_DISABLE_AUTOUPDATE 1 \
      --prefix PATH : ${lib.makeBinPath [ ripgrep ]}
    runHook postInstall
  '';
  meta = {
    description = "opencode, the terminal coding agent";
    homepage = "https://opencode.ai";
    license = lib.licenses.mit;
    mainProgram = "opencode";
    platforms = [ "x86_64-linux" ];
  };
}
