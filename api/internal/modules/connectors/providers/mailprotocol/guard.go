package mailprotocol

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"strconv"
)

var errResponseLimit = errors.New("mail response resource limit")

// go-imap v1 allocates a literal before reading it and does not expose its
// Reader limit through Client. Bound framing before that allocation; parsing
// commands and response semantics remains entirely in the locked IMAP library.
type imapGuard struct {
	net.Conn
	reader  *bufio.Reader
	pending []byte
	literal int64
	total   int64
}

func newIMAPGuard(conn net.Conn) *imapGuard {
	return &imapGuard{Conn: conn, reader: bufio.NewReaderSize(conn, maxHeaderBytes+1)}
}
func (g *imapGuard) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if g.total > 8*maxMessageBytes {
		return 0, errResponseLimit
	}
	if len(g.pending) > 0 {
		n := copy(p, g.pending)
		g.pending = g.pending[n:]
		g.total += int64(n)
		return n, nil
	}
	if g.literal > 0 {
		if int64(len(p)) > g.literal {
			p = p[:g.literal]
		}
		n, err := g.reader.Read(p)
		g.literal -= int64(n)
		g.total += int64(n)
		return n, err
	}
	line, err := g.reader.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) || len(line) > maxHeaderBytes {
		return 0, errResponseLimit
	}
	if err != nil {
		return 0, err
	}
	trim := bytes.TrimSuffix(line, []byte("\r\n"))
	if len(trim) > 0 && trim[len(trim)-1] == '}' {
		start := bytes.LastIndexByte(trim, '{')
		if start >= 0 {
			number := bytes.TrimSuffix(trim[start+1:len(trim)-1], []byte("+"))
			size, err := strconv.ParseUint(string(number), 10, 32)
			if err != nil || size > maxMessageBytes {
				return 0, errResponseLimit
			}
			g.literal = int64(size)
		}
	}
	// Reject excessive syntactic nesting before the recursive library parser.
	depth := 0
	quoted, escaped := false, false
	for _, b := range trim {
		if escaped {
			escaped = false
			continue
		}
		if quoted && b == '\\' {
			escaped = true
			continue
		}
		if b == '"' {
			quoted = !quoted
		}
		if !quoted {
			if b == '(' {
				depth++
				if depth > 64 {
					return 0, errResponseLimit
				}
			}
			if b == ')' {
				depth--
			}
		}
	}
	g.pending = append(g.pending[:0], line...)
	return g.Read(p)
}

// Keep the interface explicit: no plaintext transport fallback is provided.
var _ io.Reader = (*imapGuard)(nil)
