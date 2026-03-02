package ptymanager

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// Manager manages multiple PTY sessions.
type Manager struct {
	sessions map[string]*ManagedSession
	mu       sync.RWMutex
	bufSize  int
}

// NewManager creates a new PTY session manager.
func NewManager(bufSize int) *Manager {
	return &Manager{
		sessions: make(map[string]*ManagedSession),
		bufSize:  bufSize,
	}
}

// Launch starts a new Claude session in a PTY. If already running, returns nil.
func (m *Manager) Launch(sessionID, projectPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[sessionID]; ok && s.IsRunning() {
		return nil // already running
	}

	s := NewManagedSession(sessionID, projectPath, m.bufSize)
	if err := s.Start(); err != nil {
		return fmt.Errorf("launch session %s: %w", sessionID, err)
	}
	m.sessions[sessionID] = s
	return nil
}

// LaunchNew starts a fresh Claude session (no --resume).
func (m *Manager) LaunchNew(sessionID, projectPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := NewManagedSession(sessionID, projectPath, m.bufSize)
	if err := s.StartNew(); err != nil {
		return fmt.Errorf("launch new session: %w", err)
	}
	m.sessions[sessionID] = s
	return nil
}

// Attach returns the PTY file descriptor for the given session.
func (m *Manager) Attach(sessionID string) (*os.File, <-chan struct{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, nil, fmt.Errorf("session %s not found", sessionID)
	}
	if !s.IsRunning() {
		return nil, nil, fmt.Errorf("session %s is not running", sessionID)
	}
	return s.PTY(), s.Done(), nil
}

// Capture returns the current output of a session from its ring buffer.
func (m *Manager) Capture(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return "Session not running"
	}
	return s.CaptureOutput()
}

// IsRunning checks if a specific session is running.
func (m *Manager) IsRunning(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return false
	}
	return s.IsRunning()
}

// DetectRunning returns a map of session IDs that are currently running.
func (m *Manager) DetectRunning() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	running := make(map[string]bool)
	for id, s := range m.sessions {
		if s.IsRunning() {
			running[id] = true
		}
	}
	return running
}

// StopAll stops all running sessions.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.sessions {
		if s.IsRunning() {
			s.Stop()
		}
	}
}

// SetForward sets (or clears) the writer that receives a copy of PTY output
// for the given session. Used during attach to forward output to stdout.
func (m *Manager) SetForward(sessionID string, w io.Writer) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if s, ok := m.sessions[sessionID]; ok {
		s.SetForward(w)
	}
}

// ResizeEmulator updates the VT emulator dimensions for a session.
func (m *Manager) ResizeEmulator(sessionID string, w, h int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if s, ok := m.sessions[sessionID]; ok {
		s.ResizeEmulator(w, h)
	}
}

// RekeySession changes the key under which a session is tracked.
// Used to replace synthetic "new-*" IDs with real session UUIDs from disk.
func (m *Manager) RekeySession(oldID, newID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[oldID]
	if !ok {
		return false
	}
	s.SessionID = newID
	m.sessions[newID] = s
	delete(m.sessions, oldID)
	return true
}

// RunningNewSessions returns running sessions with synthetic "new-" prefix IDs
// mapped to their project paths. Used to match them to real disk sessions.
func (m *Manager) RunningNewSessions() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string)
	for id, s := range m.sessions {
		if len(id) > 4 && id[:4] == "new-" && s.IsRunning() {
			result[id] = s.ProjectPath
		}
	}
	return result
}

// Remove removes a finished session from the manager.
func (m *Manager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[sessionID]; ok {
		if s.IsRunning() {
			s.Stop()
		}
		delete(m.sessions, sessionID)
	}
}
