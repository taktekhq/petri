// Package startup parses a Postgres client's opening handshake.
//
// A client may first send an SSLRequest or GSSEncRequest; petri answers 'N'
// (no encryption) so the rest of the conversation is plaintext we can inspect.
// The next message is the StartupMessage, which carries user/database/etc.
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

// Read reads the client's startup phase, answering 'N' to SSL/GSS requests
// and returning the parsed StartupMessage.
func Read(rw io.ReadWriter) (*Info, error) {
	be := pgproto3.NewBackend(rw, rw)
	for {
		msg, err := be.ReceiveStartupMessage()
		if err != nil {
			return nil, fmt.Errorf("read startup: %w", err)
		}
		switch m := msg.(type) {
		case *pgproto3.SSLRequest, *pgproto3.GSSEncRequest:
			if _, err := rw.Write([]byte{'N'}); err != nil {
				return nil, fmt.Errorf("reject encryption: %w", err)
			}
		case *pgproto3.StartupMessage:
			return &Info{
				Database:        m.Parameters["database"],
				User:            m.Parameters["user"],
				ApplicationName: m.Parameters["application_name"],
				raw:             m,
			}, nil
		case *pgproto3.CancelRequest:
			return nil, fmt.Errorf("cancel requests are not supported yet")
		default:
			return nil, fmt.Errorf("unexpected startup message %T", msg)
		}
	}
}

// WriteTo replays the captured StartupMessage to the backend. Mutations to
// Database/User/ApplicationName since Read are reflected on the wire — the
// forking hook uses this to redirect a client onto its fresh fork.
func (i *Info) WriteTo(w io.Writer) (int64, error) {
	syncParam(i.raw.Parameters, "user", i.User)
	syncParam(i.raw.Parameters, "database", i.Database)
	syncParam(i.raw.Parameters, "application_name", i.ApplicationName)

	buf, err := i.raw.Encode(nil)
	if err != nil {
		return 0, fmt.Errorf("encode startup: %w", err)
	}
	n, err := w.Write(buf)
	return int64(n), err
}

// syncParam writes value back into params, or deletes the key when value is
// empty — so clients that never sent a param don't see one on the wire.
func syncParam(params map[string]string, key, value string) {
	if value == "" {
		delete(params, key)
		return
	}
	params[key] = value
}
