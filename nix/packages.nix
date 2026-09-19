# The Go binaries that ship inside a guest, built from the repository root.
#
# The generated protobuf code is not in git (`.gitignore` has `/internal/gen/`),
# so it is produced here by `buf` with local plugins. That keeps the build
# offline and reproducible: buf compiles the .proto files itself, so no
# network and no remote plugin is involved.
#
# Workstream: docs/workstreams/04-guestd.md. Used by
# nix/guest/base/guestd.nix (02) and nix/guest/tests/guestd.nix.
{ pkgs, lib, version ? "dev" }:

let
  root = ../.;

  # Only what the Go build reads, so an edit to docs or apps/ does not rebuild
  # the guest.
  src = lib.cleanSourceWith {
    name = "repose-go-src";
    src = root;
    filter = path: type:
      let rel = lib.removePrefix (toString root + "/") (toString path);
      in lib.any (p: rel == p || lib.hasPrefix (p + "/") rel)
        [ "go.mod" "go.sum" "cmd" "internal" "proto" "buf.yaml" ];
  };

  bufTemplate = pkgs.writeText "buf.gen.nix.yaml" (builtins.toJSON {
    version = "v2";
    plugins = [
      {
        local = "protoc-gen-go";
        out = ".";
        opt = "module=github.com/heracraft/repose/internal/gen";
      }
      {
        local = "protoc-gen-go-grpc";
        out = ".";
        opt = "module=github.com/heracraft/repose/internal/gen";
      }
    ];
  });

  generated = pkgs.runCommand "repose-proto-go"
    {
      nativeBuildInputs = [ pkgs.buf pkgs.protoc-gen-go pkgs.protoc-gen-go-grpc ];
    } ''
    # buf writes a cache under $HOME, which the sandbox does not provide.
    export HOME="$NIX_BUILD_TOP/home"
    mkdir -p "$HOME" "$out"
    cd "$out"
    buf generate ${src} --template ${bufTemplate}
  '';

  repose = pkgs.buildGoModule {
    pname = "repose-guest";
    inherit version src;

    # `nix build ./nix#guestd` prints the expected value when a dependency
    # changes and this no longer matches.
    vendorHash = "sha256-7515DkvrcF5X8ryMFqx4bVLZtmYJSLQVBprGxbAquBs=";

    postPatch = ''
      mkdir -p internal/gen
      cp -r ${generated}/* internal/gen/
      chmod -R u+w internal/gen
    '';

    subPackages = [ "cmd/guestd" "cmd/repose-hook" ];

    ldflags = [ "-s" "-w" "-X main.version=${version}" ];

    # The unit tests need a writable /proc fixture tree and fork commands; they
    # run in CI, not in the sandbox.
    doCheck = false;

    meta = {
      description = "repose in-guest daemon and agent hook helper";
      mainProgram = "guestd";
    };
  };
in
{
  inherit repose generated;

  guestd = repose;
  repose-hook = repose;
}
