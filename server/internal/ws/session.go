package ws

import (
	"errors"
	"io"
	"sync"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/swapnil404/orca/pkg/types"
)

var errSessionCannotWrite = errors.New("session connection does not support WebSocket writes")

type messageWriter interface {
	WriteMessage(messageType int, data []byte) error
}

// Session represents the lifetime of an agent WebSocket connection.
type Session struct {
	initOnce   sync.Once
	closeOnce  sync.Once
	done       chan struct{}
	connection io.Closer
	writer     messageWriter
	writeMu    sync.Mutex
}

// NewSession creates an active session.
func NewSession(connection ...io.Closer) *Session {
	session := &Session{}
	if len(connection) > 0 {
		session.connection = connection[0]
		session.writer, _ = connection[0].(messageWriter)
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
	return s.writer.WriteMessage(websocket.BinaryMessage, payload)
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
