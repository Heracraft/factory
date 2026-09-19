// Package guestd is the fake guest daemon of docs/interfaces/README.md: an
// in-process implementation of the vsock protocol that hostd's tests talk to
// over a unix socket pair. It records every call and returns canned results,
// and it can be told to fail any request with any error code.
//
// It is the guest side only. hostd's client side is internal/vsockrpc.
package guestd
