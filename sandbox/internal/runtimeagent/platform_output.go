package runtimeagent

import (
	"bytes"
	"errors"
	"sync"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

// Avoid accidentally retaining a Job grant in stdout/stderr, including when
// writes split it across chunks. This is a logging boundary, not a guarantee
// against an authorized script deliberately encoding its own transient grant.
type platformOutput struct {
	mu      sync.Mutex
	secret  []byte
	pending map[agentexecution.OutputStream][]byte
	sink    func(agentexecution.OutputStream, []byte) error
}

func newPlatformOutput(token string, sink func(agentexecution.OutputStream, []byte) error) *platformOutput {
	return &platformOutput{secret: []byte(token), pending: make(map[agentexecution.OutputStream][]byte), sink: sink}
}

func (o *platformOutput) Write(stream agentexecution.OutputStream, data []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.secret) == 0 {
		return o.sink(stream, data)
	}
	value := append(o.pending[stream], data...)
	value = bytes.ReplaceAll(value, o.secret, []byte("[redacted-job-grant]"))
	count := max(0, len(value)-len(o.secret)+1)
	o.pending[stream] = bytes.Clone(value[count:])
	if count == 0 {
		return nil
	}
	return o.sink(stream, value[:count])
}

func (o *platformOutput) Flush() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	var result error
	for _, stream := range []agentexecution.OutputStream{agentexecution.OutputStreamStdout, agentexecution.OutputStreamStderr} {
		if data := o.pending[stream]; len(data) > 0 {
			result = errors.Join(result, o.sink(stream, data))
		}
	}
	clear(o.pending)
	clear(o.secret)
	o.secret = nil
	return result
}
