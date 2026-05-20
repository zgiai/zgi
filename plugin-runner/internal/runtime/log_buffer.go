package runtime

import "sync"

// logBuffer keeps the newest N log lines in memory.
type logBuffer struct {
	mu       sync.RWMutex
	capacity int
	items    []LogLine
}

func newLogBuffer(capacity int) *logBuffer {
	if capacity <= 0 {
		capacity = 200
	}
	return &logBuffer{
		capacity: capacity,
		items:    make([]LogLine, 0, capacity),
	}
}

func (b *logBuffer) append(line LogLine) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.items) == b.capacity {
		copy(b.items[0:], b.items[1:])
		b.items[len(b.items)-1] = line
		return
	}

	b.items = append(b.items, line)
}

func (b *logBuffer) snapshot() []LogLine {
	b.mu.RLock()
	defer b.mu.RUnlock()

	cp := make([]LogLine, len(b.items))
	copy(cp, b.items)
	return cp
}
