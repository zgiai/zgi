package ipc

import (
	"bufio"
	"fmt"
	"io"
	"sync"

	"plugin_runner/internal/protocol"
)

// StdIO manages bidirectional communication with a plugin process.
type StdIO struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	bufferSize    int
	maxBufferSize int

	mu     sync.Mutex
	closed bool
}

// NewStdIO creates a new bidirectional stdio handler.
func NewStdIO(stdin io.WriteCloser, stdout, stderr io.ReadCloser, bufferSize, maxBufferSize int) *StdIO {
	if bufferSize <= 0 {
		bufferSize = 4096
	}
	if maxBufferSize <= 0 {
		maxBufferSize = 5 * 1024 * 1024
	}
	return &StdIO{
		stdin:         stdin,
		stdout:        stdout,
		stderr:        stderr,
		bufferSize:    bufferSize,
		maxBufferSize: maxBufferSize,
	}
}

// Write sends data to the plugin's stdin.
func (s *StdIO) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return io.ErrClosedPipe
	}
	if s.stdin == nil {
		return io.ErrClosedPipe
	}

	_, err := s.stdin.Write(data)
	return err
}

// WriteMessage encodes and sends a protocol message.
func (s *StdIO) WriteMessage(msg *protocol.Message) error {
	data, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}
	return s.Write(data)
}

// MessageHandler handles incoming messages from the plugin.
type MessageHandler func(msg *protocol.Message) error

// LogHandler handles non-protocol log lines.
type LogHandler func(stream, line string)

// StartReading begins reading from stdout/stderr.
// Messages from stdout are parsed as protocol messages.
// Non-JSON lines and stderr are treated as logs.
func (s *StdIO) StartReading(msgHandler MessageHandler, logHandler LogHandler) {
	var wg sync.WaitGroup

	// Read stdout - try to parse as protocol messages
	if s.stdout != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.readStdout(msgHandler, logHandler)
		}()
	}

	// Read stderr - always treat as logs
	if s.stderr != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.readStderr(logHandler)
		}()
	}

	wg.Wait()
}

func (s *StdIO) readStdout(msgHandler MessageHandler, logHandler LogHandler) {
	defer s.stdout.Close()

	scanner := bufio.NewScanner(s.stdout)
	scanner.Buffer(make([]byte, s.bufferSize), s.maxBufferSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Try to parse as protocol message (JSON starting with {)
		if line[0] == '{' {
			msg, err := protocol.Decode(line)
			if err == nil && msg.Type != "" {
				if msgHandler != nil {
					if err := msgHandler(msg); err != nil {
						if logHandler != nil {
							logHandler("stdout", fmt.Sprintf("handle message error: %v", err))
						}
					}
				}
				continue
			}
		}

		// Not a protocol message, treat as log
		if logHandler != nil {
			logHandler("stdout", string(line))
		}
	}

	if err := scanner.Err(); err != nil {
		if logHandler != nil {
			logHandler("stdout", fmt.Sprintf("scanner error: %v", err))
		}
	}
}

func (s *StdIO) readStderr(logHandler LogHandler) {
	defer s.stderr.Close()

	scanner := bufio.NewScanner(s.stderr)
	scanner.Buffer(make([]byte, s.bufferSize), s.maxBufferSize)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if logHandler != nil {
			logHandler("stderr", line)
		}
	}

	if err := scanner.Err(); err != nil {
		if logHandler != nil {
			logHandler("stderr", fmt.Sprintf("scanner error: %v", err))
		}
	}
}

// Close closes all stdio handles.
func (s *StdIO) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	var errs []error
	if s.stdin != nil {
		if err := s.stdin.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close stdin: %w", err))
		}
	}
	if s.stdout != nil {
		if err := s.stdout.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close stdout: %w", err))
		}
	}
	if s.stderr != nil {
		if err := s.stderr.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close stderr: %w", err))
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}
