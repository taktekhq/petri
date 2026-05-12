// Package startup negotiates the opening of a Postgres connection.
//
// A Postgres client may first send an SSLRequest or GSSEncRequest. The server
// answers 'S' (yes), 'N' (no), or an error. Today we always answer 'N': petri
// speaks plaintext to clients so it can inspect the StartupMessage that
// follows. Tomorrow we may speak TLS, but plaintext is the simplest start.
//
// Once the (possibly second) message is a StartupMessage, we capture the
// session parameters — database, user, application_name — and hand them to
// the proxy, which decides where to forward.
//
// File map:
//   - startup.go      – Info struct, Read, Info.WriteTo
//   - startup_test.go – unit tests against net.Pipe + pgproto3 frontends
package startup

import (
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgproto3"
)

// Info is the parsed startup parameters from a client.
type Info struct {
	Database        string
	User            string
	ApplicationName string

	raw *pgproto3.StartupMessage
}

// Read negotiates the startup phase with a Postgres client. SSL and GSS
// requests are answered with 'N'; the resulting StartupMessage is parsed and
// returned.
func Read(rw io.ReadWriter) (*Info, error) {
	be := pgproto3.NewBackend(rw, rw)
	for {
		msg, err := be.ReceiveStartupMessage()
		if err != nil {
			return nil, fmt.Errorf("read startup: %w", err)
		}
		switch m := msg.(type) {
		case *pgproto3.SSLRequest, *pgproto3.GSSEncRequest:
			if err := rejectEncryption(rw); err != nil {
				return nil, err
			}
		case *pgproto3.StartupMessage:
			return fromMessage(m), nil
		case *pgproto3.CancelRequest:
			return nil, fmt.Errorf("cancel requests are not supported yet")
		default:
			return nil, fmt.Errorf("unexpected startup message %T", msg)
		}
	}
}

// WriteTo replays the captured StartupMessage so the backend sees the same
// handshake the client sent us.
func (i *Info) WriteTo(w io.Writer) (int64, error) {
	buf, err := i.raw.Encode(nil)
	if err != nil {
		return 0, fmt.Errorf("encode startup: %w", err)
	}
	n, err := w.Write(buf)
	return int64(n), err
}

func rejectEncryption(w io.Writer) error {
	if _, err := w.Write([]byte{'N'}); err != nil {
		return fmt.Errorf("reject encryption: %w", err)
	}
	return nil
}

func fromMessage(m *pgproto3.StartupMessage) *Info {
	return &Info{
		Database:        m.Parameters["database"],
		User:            m.Parameters["user"],
		ApplicationName: m.Parameters["application_name"],
		raw:             m,
	}
}
