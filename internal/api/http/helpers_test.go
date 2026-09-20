package httpapi_test

import "golang.org/x/crypto/ssh"

func sshMarshal(pub ssh.PublicKey) []byte { return ssh.MarshalAuthorizedKey(pub) }
