package ws

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/swapnil404/orca/pkg/types"
)

var errSessionCannotWrite = errors.New("session connection does not support WebSocket writes")

const sessionWriteTimeout = 10 * time.Second

type messageWriter interface {
	WriteMessage(messageType int, data []byte) error
	SetWriteDeadline(time.Time) error
}

type controlWriter interface {
	WriteControl(messageType int, data []byte, deadline time.Time) error
}

// Session represents the lifetime of an agent WebSocket connection.
type Session struct {
	initOnce   sync.Once
	closeOnce  sync.Once
	done       chan struct{}
	connection io.Closer
	writer     messageWriter
	controls   controlWriter
	writeMu    sync.Mutex
}

// NewSession creates an active session.
func NewSession(connection ...io.Closer) *Session {
	session := &Session{}
	if len(connection) > 0 {
		session.connection = connection[0]
		session.writer, _ = connection[0].(messageWriter)
		session.controls, _ = connection[0].(controlWriter)
	}
	session.init()
	return session
}

// SendDesiredState writes a desired-state protobuf message to the agent.
func (s *Session) SendDesiredState(message *types.DesiredStateMessage) error {
	payload, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	if s.writer == nil {
		return errSessionCannotWrite
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.writer.SetWriteDeadline(time.Now().Add(sessionWriteTimeout)); err != nil {
		s.Close()
		return err
	}
	if err := s.writer.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		s.Close()
		return err
	}
	return s.writer.SetWriteDeadline(time.Time{})
}

// Ping verifies that the agent is still reading from the connection.
func (s *Session) Ping() error {
	if s.controls == nil {
		return errSessionCannotWrite
	}
	if err := s.controls.WriteControl(websocket.PingMessage, nil, time.Now().Add(sessionWriteTimeout)); err != nil {
		s.Close()
		return err
	}
	return nil
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
