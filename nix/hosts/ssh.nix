# sshd for operators: on the WireGuard address only (rendered into
# /run/repose/sshd.conf by repose-host-net), root via certificates from the
# Host CA (public key from host.json), no passwords, no other users, and a
# PAM session hook that hands every login to `hostd audit-login` so it
# lands in audit_log. While bootstrap is on, sshd also listens on the
# provider NIC with the operator's plain key.
{ config, lib, pkgs, ... }:
let
  hostd = config.repose.host.hostdPackage;
  # pam_exec swallows stdout; the hook's own log line goes to the journal
  # under the `hostd-audit` identifier.
  auditLogin = pkgs.writeShellScript "repose-audit-login" ''
    ${hostd}/bin/hostd audit-login 2>&1 | ${pkgs.systemd}/bin/systemd-cat -t hostd-audit
  '';
in
{
  services.openssh = {
    enable = true;
    openFirewall = false;
    settings = {
      PermitRootLogin = "prohibit-password";
      PasswordAuthentication = false;
      KbdInteractiveAuthentication = false;
      AllowUsers = [ "root" ];
      TrustedUserCAKeys = "/run/repose/host_ca.pub";
      ClientAliveInterval = 60;
      ClientAliveCountMax = 3;
      LogLevel = "VERBOSE";
      X11Forwarding = false;
    };
    extraConfig = ''
      Include /run/repose/sshd.conf
    '';
  };

  systemd.services.sshd = {
    after = [ "repose-host-net.service" ];
    wants = [ "repose-host-net.service" ];
  };

  security.pam.services.sshd.rules.session.repose-audit = {
    control = "optional";
    modulePath = "${pkgs.pam}/lib/security/pam_exec.so";
    args = [ "quiet" "seteuid" "${auditLogin}" ];
    order = config.security.pam.services.sshd.rules.session.unix.order + 10;
  };
}
