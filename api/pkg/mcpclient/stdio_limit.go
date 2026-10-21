package mcpclient

import (
	"errors"
	"io"
)

// lineLimitedReader enforces the stdio transport's newline-delimited JSON
// message limit before the SDK allocates a decoded message.
type lineLimitedReader struct {
	io.ReadCloser
	limit   int64
	used    int64
	onLimit func(error)
}

func (r *lineLimitedReader) Read(buf []byte) (int, error) {
	n, err := r.ReadCloser.Read(buf)
	for i := 0; i < n; i++ {
		if buf[i] == '\n' {
			r.used = 0
			continue
		}
		r.used++
		if r.used > r.limit {
			limitErr := &MCPError{Kind: ErrLimitExceeded, Operation: "read stdio message", Cause: errors.New("MCP stdio message exceeds configured size")}
			if r.onLimit != nil {
				r.onLimit(limitErr)
			}
			return i, limitErr
		}
	}
	return n, err
}
