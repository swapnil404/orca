package ws

import (
	"sync"
	"testing"
)

func TestHubReplacementSurvivesOldDisconnect(t *testing.T) {
	hub := NewHub()
	old, current, other := NewSession(), NewSession(), NewSession()
	defer current.Close()
	defer other.Close()
	hub.Register("alpha", old)
	hub.Register("beta", other)
	hub.Register("alpha", current)
	select {
	case <-old.Done():
	default:
		t.Fatal("replaced connection was not closed")
	}
	if hub.UnregisterSession("alpha", old) {
		t.Fatal("old disconnect removed replacement")
	}
	got, ok := hub.Get("alpha")
	if !ok || got != current {
		t.Fatal("replacement missing")
	}
	called := false
	if hub.withCurrentSession("alpha", old, func() { called = true }) || called {
		t.Fatal("stale session applied report")
	}
	if !hub.withCurrentSession("alpha", current, func() { called = true }) || !called {
		t.Fatal("current report rejected")
	}
	if !hub.UnregisterSession("alpha", current) || hub.IsConnected("alpha") {
		t.Fatal("current disconnect failed")
	}
	if got, ok := hub.Get("beta"); !ok || got != other {
		t.Fatal("disconnect crossed host boundary")
	}
}

func TestHubIdempotentRegistrationAndNilSession(t *testing.T) {
	var hub Hub
	session := NewSession()
	defer session.Close()
	hub.Register("alpha", session)
	hub.Register("alpha", session)
	hub.Register("alpha", nil)
	select {
	case <-session.Done():
		t.Fatal("registration closed current session")
	default:
	}
	if got, ok := hub.Get("alpha"); !ok || got != session {
		t.Fatal("registration lost session")
	}
}

func TestHubConcurrentReplacements(t *testing.T) {
	hub := NewHub()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				session := NewSession()
				hub.Register("alpha", session)
				hub.Get("alpha")
				hub.IsConnected("alpha")
				hub.withCurrentSession("alpha", session, func() {})
				hub.UnregisterSession("alpha", session)
				session.Close()
			}
		}()
	}
	wg.Wait()
	final := NewSession()
	defer final.Close()
	hub.Register("alpha", final)
	if got, ok := hub.Get("alpha"); !ok || got != final {
		t.Fatal("hub unusable after concurrent changes")
	}
}
