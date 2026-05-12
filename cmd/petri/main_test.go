package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/stretchr/testify/require"

	"github.com/taktekhq/petri/internal/startup"
)

const protocolV3 = 196608

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

func TestRun_StartsListenerAndLogsAddress(t *testing.T) {
	listenAddr := pickFreeAddr(t)
	logs := &bytes.Buffer{}
	go run(env{
		"PETRI_LISTEN_ADDR":  listenAddr,
		"PETRI_BACKEND_ADDR": "127.0.0.1:1",
	}.lookup(), logs)

	waitForLog(t, logs, "petri listening")

	conn, err := net.Dial("tcp", listenAddr)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

// TestRun_LogsClientConnectionsByAppName exercises the OnStartup hook wired
// up in main: a client sends a StartupMessage with application_name=… and
// the log line records it. No real Postgres needed.
func TestRun_LogsClientConnectionsByAppName(t *testing.T) {
	backend := startStubBackend(t)
	listenAddr := pickFreeAddr(t)
	logs := &bytes.Buffer{}
	go run(env{
		"PETRI_LISTEN_ADDR":  listenAddr,
		"PETRI_BACKEND_ADDR": backend,
	}.lookup(), logs)
	waitForLog(t, logs, "petri listening")

	sendStartupAndDisconnect(t, listenAddr, "my-test-app")

	waitForLog(t, logs, `app="my-test-app"`)
}

// ---- forkPerConnection ----

func TestForkPerConnection_MutatesDatabaseAndAppName(t *testing.T) {
	f := &fakeForker{}
	hook := forkPerConnection(f, io.Discard)

	info := &startup.Info{Database: "appdb", User: "alice", ApplicationName: "old"}
	cleanup, err := hook(info)
	require.NoError(t, err)
	require.NotNil(t, cleanup)

	require.Len(t, f.forks, 1)
	require.Equal(t, "appdb", f.forks[0].template)
	require.True(t, strings.HasPrefix(f.forks[0].forkName, "petri_"), "fork name should be petri_<hex>")
	require.Equal(t, f.forks[0].forkName, info.Database, "database should be rewritten to the fork")
	require.Equal(t, info.Database, info.ApplicationName, "application_name should equal the new database")
	require.Equal(t, "alice", info.User, "user must not change")
}

func TestForkPerConnection_CleanupDropsTheFork(t *testing.T) {
	f := &fakeForker{}
	hook := forkPerConnection(f, io.Discard)

	cleanup, err := hook(&startup.Info{Database: "appdb"})
	require.NoError(t, err)
	require.Empty(t, f.drops, "Drop should not run until cleanup is called")

	cleanup()
	require.Equal(t, []string{f.forks[0].forkName}, f.drops)
}

func TestForkPerConnection_PropagatesForkError(t *testing.T) {
	f := &fakeForker{forkErr: errors.New("boom")}
	hook := forkPerConnection(f, io.Discard)

	cleanup, err := hook(&startup.Info{Database: "appdb"})
	require.ErrorContains(t, err, "boom")
	require.Nil(t, cleanup, "no cleanup should be returned when fork fails")
}

func TestForkPerConnection_GeneratesUniqueNames(t *testing.T) {
	f := &fakeForker{}
	hook := forkPerConnection(f, io.Discard)

	_, err := hook(&startup.Info{Database: "appdb"})
	require.NoError(t, err)
	_, err = hook(&startup.Info{Database: "appdb"})
	require.NoError(t, err)
	require.NotEqual(t, f.forks[0].forkName, f.forks[1].forkName)
}

// ---- buildOnStartup ----

func TestBuildOnStartup_NoAdminDSN_LogsOnly(t *testing.T) {
	logs := &bytes.Buffer{}
	hook := buildOnStartup(config{}, logs)

	cleanup, err := hook(&startup.Info{Database: "appdb", User: "alice", ApplicationName: "x"})
	require.NoError(t, err)
	require.Nil(t, cleanup, "logging hook has no cleanup")
	require.Contains(t, logs.String(), `client connected`)
	require.Contains(t, logs.String(), `app="x"`)
}

// ---- helpers ----

// fakeForker records calls for assertions. It lives next to the test that
// owns it so the production interface stays minimal.
type fakeForker struct {
	forks   []fakeForkCall
	drops   []string
	forkErr error
	dropErr error
}

type fakeForkCall struct {
	template string
	forkName string
}

func (f *fakeForker) Fork(_ context.Context, template, forkName string) error {
	f.forks = append(f.forks, fakeForkCall{template: template, forkName: forkName})
	return f.forkErr
}

func (f *fakeForker) Drop(_ context.Context, forkName string) error {
	f.drops = append(f.drops, forkName)
	return f.dropErr
}



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

// startStubBackend accepts connections and silently drops every byte —
// enough to keep the proxy's per-connection goroutine alive without speaking
// the real Postgres protocol.
func startStubBackend(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go acceptAndDiscard(ln)
	return ln.Addr().String()
}

func acceptAndDiscard(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) { defer c.Close(); io.Copy(io.Discard, c) }(c)
	}
}

// sendStartupAndDisconnect sends a single StartupMessage with the given
// application_name, then closes — just enough to trigger the OnStartup hook.
func sendStartupAndDisconnect(t *testing.T, addr, appName string) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	fe := pgproto3.NewFrontend(conn, conn)
	fe.Send(&pgproto3.StartupMessage{
		ProtocolVersion: protocolV3,
		Parameters: map[string]string{
			"user":             "alice",
			"database":         "anything",
			"application_name": appName,
		},
	})
	require.NoError(t, fe.Flush())
}

func waitForLog(t *testing.T, logs *bytes.Buffer, substr string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), substr)
	}, 2*time.Second, 10*time.Millisecond, "waiting for log line containing %q", substr)
}
