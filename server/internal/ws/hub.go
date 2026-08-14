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
	previous := h.sessions[hostID]
	if previous == session {
		h.mu.Unlock()
		return
	}
	h.sessions[hostID] = session
	h.mu.Unlock()
	if previous != nil {
		previous.Close()
	}
}

func (h *Hub) withCurrentSession(hostID string, session *Session, fn func()) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.sessions[hostID] != session {
		return false
	}
	fn()
	return true
}

// Get returns the active session for hostID, if one is registered.
func (h *Hub) Get(hostID string) (*Session, bool) {
	h.mu.RLock()
	session, ok := h.sessions[hostID]
	h.mu.RUnlock()
	return session, ok
}

// IsConnected reports whether hostID has a current agent session.
func (h *Hub) IsConnected(hostID string) bool {
	_, connected := h.Get(hostID)
	return connected
}

// UnregisterSession removes session only if it is still active for hostID.
func (h *Hub) UnregisterSession(hostID string, session *Session) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.sessions[hostID] == session {
		delete(h.sessions, hostID)
		return true
	}
	return false
}
