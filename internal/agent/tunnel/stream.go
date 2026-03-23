package tunnel

import (
	"net"
	"sync"
)

// pendingStream buffers data arriving before the target dial completes.
// Once the dial succeeds and Activate() is called, buffered data is flushed
// and subsequent writes go directly to the connection.
type pendingStream struct {
	mu       sync.Mutex
	conn     net.Conn   // nil until Activate()
	buf      [][]byte   // buffered data before activation
	closed   bool
}

// Write buffers data if not yet activated, or writes directly to conn.
func (s *pendingStream) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return &streamClosedErr{}
	}

	if s.conn != nil {
		_, err := s.conn.Write(data)
		return err
	}

	// Buffer a copy of the data
	cp := make([]byte, len(data))
	copy(cp, data)
	s.buf = append(s.buf, cp)
	return nil
}

// Activate sets the real connection and flushes buffered data.
func (s *pendingStream) Activate(conn net.Conn) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		conn.Close()
		return &streamClosedErr{}
	}

	s.conn = conn

	// Flush buffered data
	for _, data := range s.buf {
		if _, err := conn.Write(data); err != nil {
			return err
		}
	}
	s.buf = nil
	return nil
}

// Close closes the underlying connection if activated.
func (s *pendingStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.conn != nil {
		s.conn.Close()
	}
}

// Conn returns the underlying connection, or nil if not yet activated.
func (s *pendingStream) Conn() net.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn
}

type streamClosedErr struct{}

func (e *streamClosedErr) Error() string { return "stream closed" }
