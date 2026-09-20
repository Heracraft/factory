// Package gateway is the SSH gateway on the edge (docs/workstreams/
// 06-gateway-edge.md, docs/interfaces/ssh-gateway.md). It accepts a user's
// connection, verifies the certificate the api's User CA issued, resolves
// the login name to a guest through the api's internal routes, dials the
// guest's sshd with a gateway-issued five-minute certificate (DECISIONS
// I-1) and relays channels and requests between the two SSH connections.
//
// Nothing here terminates a session into a shell on the edge: after
// authentication the gateway is a relay, and the guest's sshd is a second,
// independent check of the principal.
package gateway
