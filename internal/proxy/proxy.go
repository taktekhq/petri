// Package proxy forwards client connections to a single backend Postgres.
//
// Each client connection is walked through: read the startup message →
// notify the OnStartup hook → dial the backend → replay the startup →
// pipe bytes in both directions until either side closes.
//
// File map:
//   - proxy.go   – Proxy type, Serve loop, per-connection orchestration
//   - bridge.go  – the bidirectional byte pipe used after startup
package proxy

import (
	"errors"
	"net"

	"github.com/taktekhq/petri/internal/startup"
)

// Proxy forwards TCP connections to a single backend.
type Proxy struct {
	BackendAddr string

	// OnStartup, if set, is called once per client after the startup message
	// is parsed and before the backend is contacted. Returning an error closes
	// the client connection. Phase 3 will use this hook to fork the database.
	OnStartup func(*startup.Info) error
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

// handleClient walks one connection through: read startup → notify → dial
// backend → bridge.
func (p *Proxy) handleClient(client net.Conn) {
	defer client.Close()

	info, err := startup.Read(client)
	if err != nil {
		return
	}
	if err := p.notifyStartup(info); err != nil {
		return
	}

	backend, err := p.dialBackend(info)
	if err != nil {
		return
	}
	defer backend.Close()

	bridge(client, backend)
}

func (p *Proxy) notifyStartup(info *startup.Info) error {
	if p.OnStartup == nil {
		return nil
	}
	return p.OnStartup(info)
}

// dialBackend opens a TCP connection to the backend and replays the captured
// startup message so the backend sees the same handshake the client sent.
func (p *Proxy) dialBackend(info *startup.Info) (net.Conn, error) {
	backend, err := net.Dial("tcp", p.BackendAddr)
	if err != nil {
		return nil, err
	}
	if _, err := info.WriteTo(backend); err != nil {
		backend.Close()
		return nil, err
	}
	return backend, nil
}
