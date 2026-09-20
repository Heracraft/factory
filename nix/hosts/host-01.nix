# host-01: the first production host (DECISIONS I-14), a Standard_D16s_v5
# driven by `hostdev` on the edge until the api exists (I-17, I-39). This
# file holds only what makes host-01 differ from the generic `host`
# configuration, and none of it is secret: the edge's static public
# address, hostdev's CA certificate, an operator public key, and the Blob
# endpoint and identity the host snapshots with.
#
# When the api replaces hostdev: drop apiAddr and apiCA (the defaults are the
# api behind a public certificate) and, once workstream 06 has the host on
# WireGuard, drop bootstrap.
{ ... }:
{
  repose.host = {
    # The edge's static public IP (infra output edge_public_ip); hostdev
    # listens on 443 there. The server certificate carries this IP.
    apiAddr = "20.102.98.254:443";
    apiCA = ''
      -----BEGIN CERTIFICATE-----
      MIIBaTCCAQ+gAwIBAgIBATAKBggqhkjOPQQDAjAcMRowGAYDVQQDExFyZXBvc2Ug
      aG9zdGRldiBDQTAeFw0yNjA5MTkyMzA5MDZaFw0zNjA5MTcwMDA5MDZaMBwxGjAY
      BgNVBAMTEXJlcG9zZSBob3N0ZGV2IENBMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcD
      QgAE0prMJRbuOtjnfZHM2Zxw0by4Uj01sIGEx8rgw7z/xjhR6ciWt3pYzE7J7Y9E
      c5Tb/ixVddA7oWyUKY2YGlLDOqNCMEAwDgYDVR0PAQH/BAQDAgKEMA8GA1UdEwEB
      /wQFMAMBAf8wHQYDVR0OBBYEFKJZ7cUC1DxWiHXNKDDTtE5gd78bMAoGCCqGSM49
      BAMCA0gAMEUCIQC0VaXK/h2vIIES8eNZUbFzy04eOQ2L79Jg5uMaU3LnTwIgLRMr
      k5Zib21Mn0oWrs/CcQkz7lLen7bFu/AYEI+/R7U=
      -----END CERTIFICATE-----
    '';

    # No WireGuard until workstream 06: the installer, the join-token
    # delivery and operators reach this host over the VNet through the
    # edge, which is sshd on the provider NIC with a plain key.
    bootstrap.enable = true;
    bootstrap.authorizedKeys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEnlaWLqhKDXwKRp74FWyHFmrK5eZXh7U3VL3a3J/EO0 operator"
    ];

    # infra outputs `snapshots.blob_endpoint` and `snapshots.identity_client_id`
    # (the user-assigned identity id-repose-host, Storage Blob Data
    # Contributor on the container).
    snapshots = {
      blobUrl = "https://reposesnapshots3912.blob.core.windows.net";
      container = "repose-snapshots";
      identityClientId = "072397e1-f6f0-4d0f-a3f0-f8d35be6d2f3";
    };
  };
}
