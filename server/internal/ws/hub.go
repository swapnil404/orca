package ws

import "sync"

// Hub tracks the active agent session for each host.
type Hub struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewHub creates an empty session hub.
func NewHub() *Hub {
	return &Hub{sessions: make(map[string]*Session)}
}

// Register associates session with hostID, replacing any existing session.
func (h *Hub) Register(hostID string, session *Session) {
	if session == nil {
		return
	}

	h.mu.Lock()
	if h.sessions == nil {
		h.sessions = make(map[string]*Session)
	}
	if h.sessions[hostID] == session {
		h.mu.Unlock()
		return
	}
	h.sessions[hostID] = session
	h.mu.Unlock()

	go func() {
		<-session.Done()
		h.unregisterSession(hostID, session)
	}()
}

// Unregister removes the active session for hostID.
func (h *Hub) Unregister(hostID string) {
	h.mu.Lock()
	delete(h.sessions, hostID)
	h.mu.Unlock()
}

// Get returns the active session for hostID, if one is registered.
func (h *Hub) Get(hostID string) (*Session, bool) {
	h.mu.RLock()
	session, ok := h.sessions[hostID]
	h.mu.RUnlock()
	return session, ok
}

func (h *Hub) unregisterSession(hostID string, session *Session) {
	h.mu.Lock()
	if h.sessions[hostID] == session {
		delete(h.sessions, hostID)
	}
	h.mu.Unlock()
}
