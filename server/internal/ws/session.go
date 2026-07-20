package ws

import "sync"

// Session represents the lifetime of an agent WebSocket connection.
// Its transport and send behavior will be added with the WebSocket integration.
type Session struct {
	initOnce  sync.Once
	closeOnce sync.Once
	done      chan struct{}
}

// NewSession creates an active session.
func NewSession() *Session {
	session := &Session{}
	session.init()
	return session
}

// Close marks the underlying connection as closed.
func (s *Session) Close() {
	s.init()
	s.closeOnce.Do(func() {
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
