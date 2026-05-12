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

// envLookup returns a getenv-style function backed by a map.
func envLookup(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

func TestLoadConfigDefaultsListenAddr(t *testing.T) {
	cfg, err := loadConfig(envLookup(map[string]string{
		"PETRI_BACKEND_ADDR": "postgres:5432",
	}))
	require.NoError(t, err)
	require.Equal(t, ":5432", cfg.ListenAddr)
	require.Equal(t, "postgres:5432", cfg.BackendAddr)
}

func TestLoadConfigOverridesListenAddr(t *testing.T) {
	cfg, err := loadConfig(envLookup(map[string]string{
		"PETRI_LISTEN_ADDR":  "0.0.0.0:6543",
		"PETRI_BACKEND_ADDR": "pg.local:5432",
	}))
	require.NoError(t, err)
	require.Equal(t, "0.0.0.0:6543", cfg.ListenAddr)
}

func TestLoadConfigRequiresBackend(t *testing.T) {
	_, err := loadConfig(envLookup(nil))
	require.Error(t, err)
	require.Contains(t, err.Error(), "PETRI_BACKEND_ADDR")
}

// TestRunListenError surfaces listener errors without requiring a backend at
// all — picking a port that's already bound forces Listen to fail.
func TestRunListenError(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occupied.Close()

	env := map[string]string{
		"PETRI_LISTEN_ADDR":  occupied.Addr().String(),
		"PETRI_BACKEND_ADDR": "127.0.0.1:1",
	}
	err = run(envLookup(env), io.Discard)
	require.Error(t, err)
	require.Contains(t, err.Error(), "listen")
}

// TestRunStartsProxy boots the full main entrypoint against a stub TCP backend
// and confirms a connection through the listen addr is forwarded. We don't
// need a real Postgres here — proxy_test already covers wire-level behavior.
// This test guards the wiring inside run().
func TestRunStartsProxy(t *testing.T) {
	backend := startEchoBackend(t)

	listenLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	listenAddr := listenLn.Addr().String()
	require.NoError(t, listenLn.Close()) // free the port for run() to bind

	env := map[string]string{
		"PETRI_LISTEN_ADDR":  listenAddr,
		"PETRI_BACKEND_ADDR": backend,
	}

	var logs bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- run(envLookup(env), &logs) }()

	// Wait for the startup log line so we know the listener is bound.
	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), "petri listening")
	}, 2*time.Second, 10*time.Millisecond)

	conn, err := net.Dial("tcp", listenAddr)
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("hello"))
	require.NoError(t, err)

	buf := make([]byte, 5)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	require.Equal(t, "hello", string(buf))
}

// startEchoBackend launches a tiny TCP echo server on a random port. The
// proxy's wire transparency makes it ideal for testing wiring without a real
// Postgres dependency.
func startEchoBackend(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(c)
		}
	}()
	return ln.Addr().String()
}
