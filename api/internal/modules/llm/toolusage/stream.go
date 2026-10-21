package toolusage

import (
	"bytes"
	"io"
	"sync"
)

// StreamBody observes SSE without changing bytes or buffering the complete response.
type StreamBody struct {
	body      io.ReadCloser
	observer  *Observer
	finish    func(bool)
	once      sync.Once
	pending   []byte
	data      []byte
	truncated bool
	eof       bool
}

func WrapStream(body io.ReadCloser, o *Observer, finish func(bool)) io.ReadCloser {
	return &StreamBody{body: body, observer: o, finish: finish}
}
func (b *StreamBody) Read(dst []byte) (int, error) {
	n, err := b.body.Read(dst)
	if n > 0 {
		b.observe(dst[:n])
	}
	if err != nil {
		b.eof = err == io.EOF
		if b.eof {
			b.flush()
		}
		b.once.Do(func() { b.finish(!b.eof) })
	}
	return n, err
}
func (b *StreamBody) Close() error {
	err := b.body.Close()
	b.once.Do(func() { b.finish(!b.eof) })
	return err
}
func (b *StreamBody) observe(chunk []byte) {
	b.pending = append(b.pending, chunk...)
	for {
		idx := bytes.IndexByte(b.pending, '\n')
		if idx < 0 {
			break
		}
		line := bytes.TrimSuffix(b.pending[:idx], []byte("\r"))
		b.pending = b.pending[idx+1:]
		if len(line) == 0 {
			b.flush()
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			v := bytes.TrimSpace(line[5:])
			if len(b.data)+len(v) <= 1024*1024 && !b.truncated {
				if len(b.data) > 0 {
					b.data = append(b.data, '\n')
				}
				b.data = append(b.data, v...)
			} else {
				b.truncated = true
				b.data = nil
			}
		}
	}
	if len(b.pending) > 1024*1024 {
		b.pending = nil
		b.truncated = true
	}
}
func (b *StreamBody) flush() {
	if !b.truncated && len(b.data) > 0 {
		b.observer.Observe(b.data)
	}
	b.data = nil
	b.truncated = false
}
