package ws

import (
	"io"
	"sync"
)

// Session represents the lifetime of an agent WebSocket connection.
// Its transport and send behavior will be added with the WebSocket integration.
type Session struct {
	initOnce   sync.Once
	closeOnce  sync.Once
	done       chan struct{}
	connection io.Closer
}

// NewSession creates an active session.
func NewSession(connection ...io.Closer) *Session {
	session := &Session{}
	if len(connection) > 0 {
		session.connection = connection[0]
	}
	session.init()
	return session
}

// Close marks the underlying connection as closed.
func (s *Session) Close() {
	s.init()
	s.closeOnce.Do(func() {
		if s.connection != nil {
			_ = s.connection.Close()
		}
		close(s.done)
	})
}

// Done is closed when the underlying connection closes.
func (s *Session) Done() <-chan struct{} {
	s.init()
	return s.done
}

func (s *Session) init() {
	s.initOnce.Do(func() {
		s.done = make(chan struct{})
	})
}
