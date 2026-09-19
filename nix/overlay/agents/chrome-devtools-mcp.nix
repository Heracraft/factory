# chrome-devtools-mcp from the npm registry, pinned in versions.json. The
# published tarball is fully bundled (no dependencies), so this is an
# install of one tree plus a node wrapper; Chromium is nixpkgs's, passed as
# --executablePath so nothing is downloaded at run time.
{ lib, stdenvNoCC, fetchurl, nodejs_24, chromium, makeWrapper }:
let
  versions = builtins.fromJSON (builtins.readFile ./versions.json);
  v = versions."chrome-devtools-mcp";
in
stdenvNoCC.mkDerivation {
  pname = "chrome-devtools-mcp";
  inherit (v) version;
  src = fetchurl { inherit (v) url hash; };
  nativeBuildInputs = [ makeWrapper ];
  dontBuild = true;
  dontConfigure = true;
  installPhase = ''
    runHook preInstall
    dest=$out/lib/node_modules/chrome-devtools-mcp
    mkdir -p $dest $out/bin
    cp -r . $dest/
    makeWrapper ${nodejs_24}/bin/node $out/bin/chrome-devtools-mcp \
      --add-flags "$dest/build/src/bin/chrome-devtools-mcp.js" \
      --add-flags "--executablePath ${chromium}/bin/chromium" \
      --set-default CHROME_DEVTOOLS_MCP_NO_USAGE_STATISTICS 1
    runHook postInstall
  '';
  meta = {
    description = "Chrome DevTools MCP server";
    homepage = "https://github.com/ChromeDevTools/chrome-devtools-mcp";
    license = lib.licenses.asl20;
    mainProgram = "chrome-devtools-mcp";
    platforms = lib.platforms.linux;
  };
}
