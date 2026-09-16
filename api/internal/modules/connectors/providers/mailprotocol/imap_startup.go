package mailprotocol

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

// The locked v1 client starts its reader with a stderr logger inside New.
// Read the real greeting with the library parser first, then hold that reader
// behind a gate until its private logger is installed. No provider text can be
// logged, even if the next capability response is malformed.
func newPrivateIMAPClient(conn net.Conn) (*client.Client, error) {
	guard := newIMAPGuard(conn)
	reader := imap.NewReader(bufio.NewReader(guard))
	reader.MaxLiteralSize = maxHeaderBytes
	response, err := imap.ReadResp(reader)
	if err != nil {
		return nil, protocolFailure(err, false)
	}
	greeting, ok := response.(*imap.StatusResp)
	if !ok || greeting.Tag != "*" || greeting.Type != imap.StatusRespOk {
		return nil, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	// Cache an empty capability set only for initialization when none was
	// announced. The real server is queried below before credentials are sent.
	capabilities := []interface{}{}
	if greeting.Code == imap.CodeCapability {
		for _, raw := range greeting.Arguments {
			value, ok := raw.(string)
			if !ok || len(value) > 128 || strings.ContainsAny(value, " \t\r\n(){}[]\"\\") {
				return nil, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
			}
			capabilities = append(capabilities, value)
		}
	}
	var initial bytes.Buffer
	safe := &imap.StatusResp{Tag: "*", Type: imap.StatusRespOk, Code: imap.CodeCapability, Arguments: capabilities, Info: "ready"}
	writer := imap.NewWriter(&initial)
	if err = safe.WriteTo(writer); err != nil {
		return nil, providers.Invalid()
	}
	if err = writer.Flush(); err != nil {
		return nil, providers.Invalid()
	}
	gated := &startupConn{Conn: guard, initial: bytes.NewReader(initial.Bytes()), ready: make(chan struct{})}
	cl, err := client.New(gated)
	if err != nil {
		close(gated.ready)
		return nil, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	cl.ErrorLog = log.New(io.Discard, "", 0)
	close(gated.ready)
	if greeting.Code != imap.CodeCapability {
		if _, err = cl.Capability(); err != nil {
			return nil, protocolFailure(err, false)
		}
	}
	return cl, nil
}

type startupConn struct {
	net.Conn
	initial *bytes.Reader
	ready   chan struct{}
}

func (conn *startupConn) Read(p []byte) (int, error) {
	if conn.initial.Len() > 0 {
		return conn.initial.Read(p)
	}
	<-conn.ready
	return conn.Conn.Read(p)
}
