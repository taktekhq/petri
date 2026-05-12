package startup_test

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/stretchr/testify/require"

	"github.com/taktekhq/petri/internal/startup"
)

const protocolV3 = 196608 // 3.0 in the wire format

// TestRead_CapturesParameters is the happy path: a single StartupMessage.
func TestRead_CapturesParameters(t *testing.T) {
	server, client := pipe(t)

	go sendStartup(client, map[string]string{
		"user":             "alice",
		"database":         "mydb",
		"application_name": "test-app",
	})

	info := mustRead(t, server)
	require.Equal(t, "alice", info.User)
	require.Equal(t, "mydb", info.Database)
	require.Equal(t, "test-app", info.ApplicationName)
}

// TestRead_RejectsSSLRequest verifies we answer 'N' to SSL and then read the
// real StartupMessage that follows.
func TestRead_RejectsSSLRequest(t *testing.T) {
	server, client := pipe(t)

	go func() {
		fe := newFrontend(client)
		send(t, fe, &pgproto3.SSLRequest{})
		require.Equal(t, "N", readByte(t, client))
		sendStartupVia(fe, map[string]string{"user": "alice", "database": "mydb"})
	}()

	info := mustRead(t, server)
	require.Equal(t, "alice", info.User)
}

// TestRead_RejectsGSSEncRequest is the same contract for GSS.
func TestRead_RejectsGSSEncRequest(t *testing.T) {
	server, client := pipe(t)

	go func() {
		fe := newFrontend(client)
		send(t, fe, &pgproto3.GSSEncRequest{})
		require.Equal(t, "N", readByte(t, client))
		sendStartupVia(fe, map[string]string{"user": "alice", "database": "mydb"})
	}()

	info := mustRead(t, server)
	require.Equal(t, "alice", info.User)
}

// TestRead_RejectsCancelRequest pins that out-of-scope messages are reported,
// not silently dropped.
func TestRead_RejectsCancelRequest(t *testing.T) {
	server, client := pipe(t)

	go func() {
		defer client.Close()
		sendCancelRequest(t, client)
	}()

	_, err := startup.Read(server)
	require.ErrorContains(t, err, "cancel")
}

// TestWriteTo_RoundTripsViaPgproto3 confirms WriteTo emits bytes that decode
// back to the same StartupMessage. This is what guarantees the backend sees
// what the client sent.
func TestWriteTo_RoundTripsViaPgproto3(t *testing.T) {
	server, client := pipe(t)
	go sendStartup(client, map[string]string{
		"user":             "alice",
		"database":         "mydb",
		"application_name": "round-trip",
	})

	info := mustRead(t, server)

	var buf bytes.Buffer
	_, err := info.WriteTo(&buf)
	require.NoError(t, err)

	require.Equal(t, "round-trip", decodeStartup(t, &buf).Parameters["application_name"])
}

// TestWriteTo_AppliesMutations is the Phase-3 contract: the OnStartup hook
// mutates Info, and WriteTo emits the mutated values to the backend.
func TestWriteTo_AppliesMutations(t *testing.T) {
	server, client := pipe(t)
	go sendStartup(client, map[string]string{
		"user":             "alice",
		"database":         "appdb",
		"application_name": "original",
	})

	info := mustRead(t, server)
	info.Database = "fork_xyz"
	info.ApplicationName = "fork_xyz"

	var buf bytes.Buffer
	_, err := info.WriteTo(&buf)
	require.NoError(t, err)

	sm := decodeStartup(t, &buf)
	require.Equal(t, "alice", sm.Parameters["user"])
	require.Equal(t, "fork_xyz", sm.Parameters["database"])
	require.Equal(t, "fork_xyz", sm.Parameters["application_name"])
}

// TestWriteTo_RemovesParamWhenClearedToEmpty pins the contract that clearing
// a field to "" removes it from the wire, rather than emitting an empty value.
func TestWriteTo_RemovesParamWhenClearedToEmpty(t *testing.T) {
	server, client := pipe(t)
	go sendStartup(client, map[string]string{
		"user":             "alice",
		"database":         "appdb",
		"application_name": "original",
	})

	info := mustRead(t, server)
	info.ApplicationName = ""

	var buf bytes.Buffer
	_, err := info.WriteTo(&buf)
	require.NoError(t, err)

	_, present := decodeStartup(t, &buf).Parameters["application_name"]
	require.False(t, present, "application_name should be removed, not blanked")
}

// ---- helpers ----

func pipe(t *testing.T) (server, client net.Conn) {
	t.Helper()
	server, client = net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })
	return server, client
}

func newFrontend(c net.Conn) *pgproto3.Frontend {
	return pgproto3.NewFrontend(c, c)
}

func send(t *testing.T, fe *pgproto3.Frontend, msg pgproto3.FrontendMessage) {
	t.Helper()
	fe.Send(msg)
	require.NoError(t, fe.Flush())
}

func sendStartup(c net.Conn, params map[string]string) {
	sendStartupVia(newFrontend(c), params)
}

func sendStartupVia(fe *pgproto3.Frontend, params map[string]string) {
	fe.Send(&pgproto3.StartupMessage{
		ProtocolVersion: protocolV3,
		Parameters:      params,
	})
	_ = fe.Flush()
}

// sendCancelRequest writes the 16-byte CancelRequest packet by hand. pgproto3
// does not expose a typed constructor we want to depend on.
func sendCancelRequest(t *testing.T, c net.Conn) {
	t.Helper()
	pkt := []byte{
		0, 0, 0, 16, // length
		0x04, 0xd2, 0x16, 0x2e, // CancelRequest code
		0, 0, 0, 1, // pid
		0, 0, 0, 2, // secret
	}
	_, err := c.Write(pkt)
	require.NoError(t, err)
}

func readByte(t *testing.T, c net.Conn) string {
	t.Helper()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(time.Second)))
	buf := make([]byte, 1)
	_, err := io.ReadFull(c, buf)
	require.NoError(t, err)
	return string(buf)
}

func mustRead(t *testing.T, server net.Conn) *startup.Info {
	t.Helper()
	require.NoError(t, server.SetReadDeadline(time.Now().Add(2*time.Second)))
	info, err := startup.Read(server)
	require.NoError(t, err)
	return info
}

// decodeStartup parses bytes produced by WriteTo back into a StartupMessage,
// using a pgproto3 Backend (the role that reads startup packets).
func decodeStartup(t *testing.T, r io.Reader) *pgproto3.StartupMessage {
	t.Helper()
	be := pgproto3.NewBackend(r, io.Discard)
	msg, err := be.ReceiveStartupMessage()
	require.NoError(t, err)
	sm, ok := msg.(*pgproto3.StartupMessage)
	require.True(t, ok, "expected StartupMessage, got %T", msg)
	return sm
}
