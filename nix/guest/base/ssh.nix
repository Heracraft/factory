# sshd trusting the platform User CA, with the project id as the only
# accepted principal (docs/interfaces/ssh-gateway.md "Guest sshd").
#
# Key material is a secret delivered by hostd (DECISIONS I-10, I-35): guestd
# writes the reserved names to /run/repose/, and /etc/ssh/ holds symlinks to
# them so sshd_config can use the paths ssh-gateway.md shows. sshd must start
# before the material arrives (Ready is sent once sshd listens, and the
# secrets follow Ready), so a throwaway key is generated when none exists;
# guestd's reload after WriteSecrets makes sshd re-exec on the real key and
# certificate.
{ config, lib, pkgs, ... }:
let
  runDir = "/run/repose";
  hostKey = "${runDir}/ssh_host_ed25519_key";
  hostCert = "${runDir}/ssh_host_ed25519_key-cert.pub";
  userCA = "${runDir}/user_ca.pub";
in
{
  # NixOS includes systemd's ssh_config.d drop-in (the systemd-ssh-proxy
  # for AF_VSOCK/AF_UNIX hosts) from the nix store in every ssh client
  # invocation. Over the shared virtio-fs store the file is not owned by
  # root as far as the guest can tell, and ssh refuses the whole config with
  # "Bad owner or permissions", which broke `git fetch origin` in the first
  # real guest (DECISIONS I-109). The proxy is for reaching VMs from a host
  # over vsock, which a guest never does.
  programs.ssh.systemd-ssh-proxy.enable = false;

  services.openssh = {
    enable = true;
    startWhenNeeded = false;
    # We manage the host key ourselves; no NixOS-generated keys.
    hostKeys = [ ];
    settings = {
      PasswordAuthentication = false;
      KbdInteractiveAuthentication = false;
      PermitRootLogin = "no";
      AllowUsers = [ "dev" ];
      ClientAliveInterval = 30;
      ClientAliveCountMax = 4;
      X11Forwarding = false;
      AllowAgentForwarding = true;
      AllowTcpForwarding = true;
      GatewayPorts = "no";
      StreamLocalBindUnlink = true;
      PubkeyAuthentication = true;
      AuthorizedPrincipalsFile = "/etc/ssh/principals/%u";
      TrustedUserCAKeys = "/etc/ssh/user_ca.pub";
      AcceptEnv = [ "TZ" "LANG" "COLORTERM" ];
      # Session env for tmux and agents; SSH gives login shells anyway.
      PrintMotd = false;
    };
    # Only the CA path: no per-user authorized_keys can widen access.
    authorizedKeysInHomedir = false;
    authorizedKeysFiles = lib.mkForce [ "none" ];
    extraConfig = ''
      HostKey /etc/ssh/ssh_host_ed25519_key
      HostCertificate /etc/ssh/ssh_host_ed25519_key-cert.pub
    '';
  };

  # The reserved paths live on tmpfs; /etc/ssh points at them.
  environment.etc."ssh/ssh_host_ed25519_key".source = hostKey;
  environment.etc."ssh/ssh_host_ed25519_key-cert.pub".source = hostCert;
  environment.etc."ssh/user_ca.pub".source = userCA;

  # systemd's ssh generator would add sshd listeners on AF_VSOCK and a local
  # AF_UNIX socket; the only way in is the tap interface through the gateway.
  boot.kernelParams = [ "systemd.ssh_auto=no" ];

  systemd.tmpfiles.rules = [
    "d /etc/ssh/principals 0755 root root -"
    "d ${runDir} 0755 root root -"
  ];

  systemd.services.sshd = {
    # Reload re-execs sshd so a delivered host key and certificate are read
    # (guestd calls `systemctl reload sshd` after WriteSecrets and
    # SetPrincipals). NixOS's unit has no ExecReload of its own.
    serviceConfig.ExecReload = "${pkgs.coreutils}/bin/kill -HUP $MAINPID";
    preStart = lib.mkBefore ''
      # Throwaway key until hostd delivers the real one; an empty CA file
      # trusts nobody, so a guest that never got its CA refuses everyone.
      if [ ! -s ${hostKey} ]; then
        ${pkgs.openssh}/bin/ssh-keygen -q -t ed25519 -N "" -C "repose-bootstrap" -f ${hostKey}
        chmod 0600 ${hostKey}
        rm -f ${hostKey}.pub
      fi
      if [ ! -e ${userCA} ]; then
        : > ${userCA}
        chmod 0644 ${userCA}
      fi
      if [ ! -e /etc/ssh/principals/dev ]; then
        : > /etc/ssh/principals/dev
        chmod 0644 /etc/ssh/principals/dev
      fi
    '';
  };
}
