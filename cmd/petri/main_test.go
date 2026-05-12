package main

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---- loadConfig ----

func TestLoadConfig_Defaults(t *testing.T) {
	cfg := mustLoadConfig(t, env{"PETRI_BACKEND_ADDR": "postgres:5432"})
	require.Equal(t, ":5432", cfg.ListenAddr)
	require.Equal(t, "postgres:5432", cfg.BackendAddr)
}

func TestLoadConfig_OverrideListenAddr(t *testing.T) {
	cfg := mustLoadConfig(t, env{
		"PETRI_LISTEN_ADDR":  "0.0.0.0:6543",
		"PETRI_BACKEND_ADDR": "pg:5432",
	})
	require.Equal(t, "0.0.0.0:6543", cfg.ListenAddr)
}

func TestLoadConfig_BackendRequired(t *testing.T) {
	_, err := loadConfig(env{}.lookup())
	require.ErrorContains(t, err, "PETRI_BACKEND_ADDR")
}

// ---- run ----

func TestRun_FailsOnBusyListenAddr(t *testing.T) {
	busy := bindAndHold(t)
	err := run(env{
		"PETRI_LISTEN_ADDR":  busy.Addr().String(),
		"PETRI_BACKEND_ADDR": "127.0.0.1:1",
	}.lookup(), io.Discard)
	require.ErrorContains(t, err, "listen")
}

func TestRun_ProxiesBytesEndToEnd(t *testing.T) {
	backend := startEchoBackend(t)
	listenAddr := pickFreeAddr(t)

	logs := &bytes.Buffer{}
	go run(env{
		"PETRI_LISTEN_ADDR":  listenAddr,
		"PETRI_BACKEND_ADDR": backend,
	}.lookup(), logs)
	waitForListening(t, logs)

	requireEchoes(t, listenAddr, "hello")
}

// ---- helpers ----

type env map[string]string

func (e env) lookup() func(string) string { return func(k string) string { return e[k] } }

func mustLoadConfig(t *testing.T, e env) config {
	t.Helper()
	cfg, err := loadConfig(e.lookup())
	require.NoError(t, err)
	return cfg
}

// bindAndHold takes a port and keeps it bound for the test's lifetime.
func bindAndHold(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

// pickFreeAddr returns a host:port that's free right now. Race-prone in
// principle, fine in practice for local tests.
func pickFreeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func startEchoBackend(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go acceptAndEcho(ln)
	return ln.Addr().String()
}

func acceptAndEcho(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) { defer c.Close(); io.Copy(c, c) }(c)
	}
}

func waitForListening(t *testing.T, logs *bytes.Buffer) {
	t.Helper()
	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), "petri listening")
	}, 2*time.Second, 10*time.Millisecond)
}

func requireEchoes(t *testing.T, addr, msg string) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte(msg))
	require.NoError(t, err)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	buf := make([]byte, len(msg))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	require.Equal(t, msg, string(buf))
}
