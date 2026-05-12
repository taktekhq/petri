// Package proxy forwards client connections to a single backend Postgres.
//
// Per connection: read startup → call OnStartup hook → dial backend → replay
// startup → bridge bytes → run cleanup.
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
	// is parsed and before the backend is contacted. It may mutate Info to
	// rewrite the session (e.g. database name). The returned cleanup, if
	// non-nil, runs after the client disconnects — used by the forking hook
	// to drop the per-connection database.
	OnStartup func(*startup.Info) (cleanup func(), err error)
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

func (p *Proxy) handleClient(client net.Conn) {
	defer client.Close()

	info, err := startup.Read(client)
	if err != nil {
		return
	}

	if p.OnStartup != nil {
		cleanup, err := p.OnStartup(info)
		if err != nil {
			return
		}
		if cleanup != nil {
			defer cleanup()
		}
	}

	backend, err := net.Dial("tcp", p.BackendAddr)
	if err != nil {
		return
	}
	defer backend.Close()

	if _, err := info.WriteTo(backend); err != nil {
		return
	}

	bridge(client, backend)
}
