# The `--api-addr`, `--api-server-name` and `--api-ca` flags, shared by the
# two units that dial the api: hostd.service and repose-register.service.
# They must agree; a host that registers against one api and streams to
# another is a host nobody can find.
#
# The CA comes from `repose.host.apiCA` (a PEM known at build time, which is
# how a host names the `hostdev` stand-in's CA, DECISIONS I-17 and I-40) or
# from `repose.host.apiCAFile` (a path on the host, for a CA that only
# exists at run time, DECISIONS I-75). Empty means the system roots, which
# is what the real api behind a public certificate needs.
{ lib, pkgs, cfg }:
let
  caFile =
    if cfg.apiCA != "" then "${pkgs.writeText "repose-api-ca.pem" cfg.apiCA}"
    else cfg.apiCAFile;
in
"--api-addr ${lib.escapeShellArg cfg.apiAddr}"
+ lib.optionalString (cfg.apiServerName != "") " --api-server-name ${lib.escapeShellArg cfg.apiServerName}"
+ lib.optionalString (caFile != "") " --api-ca ${lib.escapeShellArg caFile}"
