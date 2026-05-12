// Package proxy is a minimal TCP proxy. It accepts client connections on a
// listener and forwards every byte to a single backend address, in both
// directions, until either side closes.
//
// This is the foundation that later phases of petri build on: protocol parsing,
// per-connection database forking, and so on. For now it is intentionally
// transparent — anything that works against the backend works through the proxy.
package proxy

import (
	"errors"
	"io"
	"net"
	"sync"
)

// Proxy forwards TCP connections to a single backend.
type Proxy struct {
	BackendAddr string
}

// Serve accepts connections on ln and handles each in its own goroutine.
// Returns nil when ln is closed; returns the underlying error otherwise.
func (p *Proxy) Serve(ln net.Listener) error {
	for {
		client, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go p.handle(client)
	}
}

func (p *Proxy) handle(client net.Conn) {
	defer client.Close()

	backend, err := net.Dial("tcp", p.BackendAddr)
	if err != nil {
		return
	}
	defer backend.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go pipe(&wg, backend, client)
	go pipe(&wg, client, backend)
	wg.Wait()
}

// pipe copies src→dst and then half-closes dst so the paired pipe in the other
// direction unblocks and returns. Without the close, a half-closed TCP
// connection would leave one io.Copy blocked forever.
func pipe(wg *sync.WaitGroup, dst, src net.Conn) {
	defer wg.Done()
	io.Copy(dst, src)
	dst.Close()
}
