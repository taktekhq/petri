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

	"github.com/stretchr/testify/require"

	"github.com/taktekhq/petri/internal/startup"
)

// ---- loadConfig ----

func TestLoadConfig_Defaults(t *testing.T) {
	cfg := loadConfig(env{}.lookup())
	require.Equal(t, ":5432", cfg.PassthroughAddr)
	require.Equal(t, ":5433", cfg.ForkAddr)
	require.Equal(t, "5432", cfg.BackendPort)
	require.Equal(t, "postgres", cfg.AdminUser)
	require.Empty(t, cfg.AdminPassword)
}

func TestLoadConfig_ReadsPostgresEnvVars(t *testing.T) {
	cfg := loadConfig(env{
		"PGPORT":            "5434",
		"POSTGRES_USER":     "adminbob",
		"POSTGRES_PASSWORD": "s3cret",
	}.lookup())
	require.Equal(t, "5434", cfg.BackendPort)
	require.Equal(t, "adminbob", cfg.AdminUser)
	require.Equal(t, "s3cret", cfg.AdminPassword)
}

func TestLoadConfig_OverridesPassthroughAndForkAddrs(t *testing.T) {
	cfg := loadConfig(env{
		"PETRI_PASSTHROUGH_ADDR": "0.0.0.0:6543",
		"PETRI_FORK_ADDR":        "0.0.0.0:6544",
	}.lookup())
	require.Equal(t, "0.0.0.0:6543", cfg.PassthroughAddr)
	require.Equal(t, "0.0.0.0:6544", cfg.ForkAddr)
}

// ---- run ----

func TestRun_FailsOnBusyPassthroughAddr(t *testing.T) {
	busy := bindAndHold(t)
	err := run(env{
		"PETRI_PASSTHROUGH_ADDR": busy.Addr().String(),
		"PETRI_FORK_ADDR":        pickFreeAddr(t),
	}.lookup(), io.Discard)
	require.ErrorContains(t, err, "listen passthrough")
}

func TestRun_FailsOnBusyForkAddr(t *testing.T) {
	busy := bindAndHold(t)
	err := run(env{
		"PETRI_PASSTHROUGH_ADDR": pickFreeAddr(t),
		"PETRI_FORK_ADDR":        busy.Addr().String(),
	}.lookup(), io.Discard)
	require.ErrorContains(t, err, "listen fork")
}

func TestRun_StartsBothListenersAndLogsAddresses(t *testing.T) {
	passthroughAddr := pickFreeAddr(t)
	forkAddr := pickFreeAddr(t)
	logs := &bytes.Buffer{}
	go run(env{
		"PETRI_PASSTHROUGH_ADDR": passthroughAddr,
		"PETRI_FORK_ADDR":        forkAddr,
	}.lookup(), logs)

	waitForLog(t, logs, "petri listening")

	for _, addr := range []string{passthroughAddr, forkAddr} {
		conn, err := net.Dial("tcp", addr)
		require.NoError(t, err, "dial %s", addr)
		require.NoError(t, conn.Close())
	}
}

// ---- forkPerConnection ----

func TestForkPerConnection_MutatesDatabaseAndAppName(t *testing.T) {
	f := &fakeForker{}
	hook := forkPerConnection(testCfg(), f, io.Discard)

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

// TestForkPerConnection_AdminDSNUsesEnvCreds pins the contract that proxy
// DB work (Fork / Drop) authenticates as POSTGRES_USER + POSTGRES_PASSWORD —
// NOT the client's user. This keeps forking working when the client is a
// read-only role; the client's own queries still use their own credentials
// via the bridge.
func TestForkPerConnection_AdminDSNUsesEnvCreds(t *testing.T) {
	f := &fakeForker{}
	cfg := config{BackendPort: "5433", AdminUser: "adminbob", AdminPassword: "s3cret"}
	hook := forkPerConnection(cfg, f, io.Discard)

	_, err := hook(&startup.Info{Database: "appdb", User: "readonly-alice"})
	require.NoError(t, err)

	dsn := f.forks[0].adminDSN
	require.Contains(t, dsn, "adminbob", "admin DSN should authenticate as POSTGRES_USER")
	require.Contains(t, dsn, "s3cret", "admin DSN should use POSTGRES_PASSWORD")
	require.NotContains(t, dsn, "readonly-alice", "admin DSN must not use the client's user")
	require.Contains(t, dsn, "127.0.0.1:5433", "admin DSN should target the backend on PGPORT")
}

func TestForkPerConnection_CleanupDropsTheForkWithSameDSN(t *testing.T) {
	f := &fakeForker{}
	hook := forkPerConnection(testCfg(), f, io.Discard)

	cleanup, err := hook(&startup.Info{Database: "appdb", User: "alice"})
	require.NoError(t, err)
	require.Empty(t, f.drops, "Drop should not run until cleanup is called")

	cleanup()
	require.Len(t, f.drops, 1)
	require.Equal(t, f.forks[0].forkName, f.drops[0].forkName)
	require.Equal(t, f.forks[0].adminDSN, f.drops[0].adminDSN, "Drop should reuse the Fork's DSN")
}

func TestForkPerConnection_PropagatesForkError(t *testing.T) {
	f := &fakeForker{forkErr: errors.New("boom")}
	hook := forkPerConnection(testCfg(), f, io.Discard)

	cleanup, err := hook(&startup.Info{Database: "appdb", User: "alice"})
	require.ErrorContains(t, err, "boom")
	require.Nil(t, cleanup, "no cleanup should be returned when fork fails")
}

func TestForkPerConnection_GeneratesUniqueNames(t *testing.T) {
	f := &fakeForker{}
	hook := forkPerConnection(testCfg(), f, io.Discard)

	_, err := hook(&startup.Info{Database: "appdb", User: "alice"})
	require.NoError(t, err)
	_, err = hook(&startup.Info{Database: "appdb", User: "alice"})
	require.NoError(t, err)
	require.NotEqual(t, f.forks[0].forkName, f.forks[1].forkName)
}

// ---- helpers ----

// fakeForker records calls for assertions. The captured adminDSN per call
// lets tests verify that admin auth is built from the client's user.
type fakeForker struct {
	forks   []fakeForkCall
	drops   []fakeDropCall
	forkErr error
	dropErr error
}

type fakeForkCall struct {
	adminDSN string
	template string
	forkName string
}

type fakeDropCall struct {
	adminDSN string
	forkName string
}

func (f *fakeForker) Fork(_ context.Context, adminDSN, template, forkName string) error {
	f.forks = append(f.forks, fakeForkCall{adminDSN: adminDSN, template: template, forkName: forkName})
	return f.forkErr
}

func (f *fakeForker) Drop(_ context.Context, adminDSN, forkName string) error {
	f.drops = append(f.drops, fakeDropCall{adminDSN: adminDSN, forkName: forkName})
	return f.dropErr
}

func testCfg() config {
	return config{BackendPort: "5432", AdminUser: "postgres", AdminPassword: "pw"}
}

type env map[string]string

func (e env) lookup() func(string) string { return func(k string) string { return e[k] } }

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

func waitForLog(t *testing.T, logs *bytes.Buffer, substr string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), substr)
	}, 2*time.Second, 10*time.Millisecond, "waiting for log line containing %q", substr)
}
