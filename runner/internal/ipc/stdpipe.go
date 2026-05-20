package ipc

import (
	"bufio"
	"io"
)

// Pipe bridges stdout/stderr to callback listeners.
type Pipe struct {
	reader   io.ReadCloser
	writer   io.WriteCloser
	buffer   int
	maxBytes int
}

func NewPipe(reader io.ReadCloser, writer io.WriteCloser, buffer, maxBytes int) *Pipe {
	return &Pipe{
		reader:   reader,
		writer:   writer,
		buffer:   buffer,
		maxBytes: maxBytes,
	}
}

func (p *Pipe) Start(fn func(string)) error {
	if p.reader == nil {
		return nil
	}
	defer p.reader.Close()
	scanner := bufio.NewScanner(p.reader)
	if p.buffer <= 0 {
		p.buffer = 1024
	}
	if p.maxBytes <= 0 {
		p.maxBytes = 5 * 1024 * 1024
	}
	scanner.Buffer(make([]byte, p.buffer), p.maxBytes)

	for scanner.Scan() {
		fn(scanner.Text())
	}
	return scanner.Err()
}

func (p *Pipe) Write(b []byte) (int, error) {
	if p.writer == nil {
		return 0, io.ErrClosedPipe
	}
	return p.writer.Write(b)
}

func (p *Pipe) CloseWriter() error {
	if p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
