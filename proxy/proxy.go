// Package proxy forwards client connections to a single backend Postgres.
//
// Today it is transparent: every byte passes through unchanged. Later steps
// will inspect the Postgres wire protocol on the way through and rewrite the
// startup message so each client lands on its own forked database.
//
// File map:
//   - proxy.go   – the Proxy type and its accept loop
//   - bridge.go  – the bidirectional byte pipe used per connection
package proxy

import (
	"errors"
	"net"
)

// Proxy forwards TCP connections to a single backend.
type Proxy struct {
	BackendAddr string
}

// Serve accepts connections on ln until ln is closed.
func (p *Proxy) Serve(ln net.Listener) error {
	for {
		client, err := ln.Accept()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		go p.handleClient(client)
	}
}

// handleClient bridges one client to a fresh backend connection.
func (p *Proxy) handleClient(client net.Conn) {
	defer client.Close()

	backend, err := net.Dial("tcp", p.BackendAddr)
	if err != nil {
		return
	}
	defer backend.Close()

	bridge(client, backend)
}
