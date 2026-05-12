package proxy

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBridge_CopiesClientToBackend exercises one direction.
func TestBridge_CopiesClientToBackend(t *testing.T) {
	client, backend := pipePair(t)
	go bridge(client.inner, backend.inner)

	send(t, client.outer, "ping")
	require.Equal(t, "ping", recv(t, backend.outer, 4))
}

// TestBridge_CopiesBackendToClient exercises the other direction.
func TestBridge_CopiesBackendToClient(t *testing.T) {
	client, backend := pipePair(t)
	go bridge(client.inner, backend.inner)

	send(t, backend.outer, "pong")
	require.Equal(t, "pong", recv(t, client.outer, 4))
}

// TestBridge_ReturnsWhenClientCloses ensures we don't leak goroutines.
func TestBridge_ReturnsWhenClientCloses(t *testing.T) {
	client, backend := pipePair(t)

	done := make(chan struct{})
	go func() { bridge(client.inner, backend.inner); close(done) }()

	require.NoError(t, client.outer.Close())
	requireClosed(t, done, "bridge did not return after client close")
}

// TestBridge_ReturnsWhenBackendCloses is the symmetric case.
func TestBridge_ReturnsWhenBackendCloses(t *testing.T) {
	client, backend := pipePair(t)

	done := make(chan struct{})
	go func() { bridge(client.inner, backend.inner); close(done) }()

	require.NoError(t, backend.outer.Close())
	requireClosed(t, done, "bridge did not return after backend close")
}

// ---- helpers ----

// pipeEnds models one side of an in-memory connection: outer is the side the
// test pretends to be; inner is what gets handed to bridge.
type pipeEnds struct{ outer, inner net.Conn }

// pipePair returns two in-memory connections wired up so anything written on
// outer comes out of inner, and vice versa.
func pipePair(t *testing.T) (pipeEnds, pipeEnds) {
	t.Helper()
	clientOuter, clientInner := net.Pipe()
	backendInner, backendOuter := net.Pipe()
	t.Cleanup(func() {
		clientOuter.Close()
		clientInner.Close()
		backendOuter.Close()
		backendInner.Close()
	})
	return pipeEnds{clientOuter, clientInner}, pipeEnds{backendOuter, backendInner}
}

func send(t *testing.T, c net.Conn, msg string) {
	t.Helper()
	_, err := c.Write([]byte(msg))
	require.NoError(t, err)
}

func recv(t *testing.T, c net.Conn, n int) string {
	t.Helper()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(time.Second)))
	buf := make([]byte, n)
	_, err := io.ReadFull(c, buf)
	require.NoError(t, err)
	return string(buf)
}

func requireClosed(t *testing.T, done <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal(msg)
	}
}
