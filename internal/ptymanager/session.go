package ptymanager

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// RingBuffer is a fixed-size circular buffer for capturing PTY output.
type RingBuffer struct {
	buf  []byte
	size int
	pos  int
	full bool
	mu   sync.Mutex
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		buf:  make([]byte, size),
		size: size,
	}
}

// Write appends data to the ring buffer, overwriting oldest data when full.
func (r *RingBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := len(p)
	if n >= r.size {
		copy(r.buf, p[n-r.size:])
		r.pos = 0
		r.full = true
		return n, nil
	}

	space := r.size - r.pos
	if n <= space {
		copy(r.buf[r.pos:], p)
	} else {
		copy(r.buf[r.pos:], p[:space])
		copy(r.buf, p[space:])
	}

	r.pos = (r.pos + n) % r.size
	if !r.full && r.pos < n {
		r.full = true
	}
	return n, nil
}

// Read returns the current contents of the buffer in write order.
func (r *RingBuffer) Read() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.full {
		out := make([]byte, r.pos)
		copy(out, r.buf[:r.pos])
		return out
	}

	out := make([]byte, r.size)
	n := copy(out, r.buf[r.pos:])
	copy(out[n:], r.buf[:r.pos])
	return out
}

// ManagedSession represents a single Claude session running in a PTY.
type ManagedSession struct {
	SessionID   string
	ProjectPath string
	cmd         *exec.Cmd
	ptmx        *os.File
	mu          sync.Mutex
	buf         *RingBuffer
	done        chan struct{}
	exitErr     error

	// forwardW receives a copy of PTY output when set (during attach).
	// Only the single background reader goroutine writes to it,
	// avoiding concurrent reads on the PTY fd.
	forwardW  io.Writer
	forwardMu sync.Mutex
}

// NewManagedSession creates a new managed session (not yet started).
func NewManagedSession(sessionID, projectPath string, bufSize int) *ManagedSession {
	return &ManagedSession{
		SessionID:   sessionID,
		ProjectPath: projectPath,
		buf:         NewRingBuffer(bufSize),
		done:        make(chan struct{}),
	}
}

// Start launches claude --resume in a PTY.
func (s *ManagedSession) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cmd = exec.Command("claude", "--resume", s.SessionID)
	s.cmd.Dir = s.ProjectPath
	s.cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	var err error
	s.ptmx, err = pty.StartWithSize(s.cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		return fmt.Errorf("pty start: %w", err)
	}

	s.startReader()
	return nil
}

// StartNew launches a fresh claude session (no --resume).
func (s *ManagedSession) StartNew() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cmd = exec.Command("claude")
	s.cmd.Dir = s.ProjectPath
	s.cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	var err error
	s.ptmx, err = pty.StartWithSize(s.cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		return fmt.Errorf("pty start: %w", err)
	}

	s.startReader()
	return nil
}

// startReader launches the single background goroutine that reads from PTY.
// All PTY output goes to the ring buffer; when forwardW is set, it also
// goes there (for real-time display during attach).
func (s *ManagedSession) startReader() {
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := s.ptmx.Read(buf)
			if n > 0 {
				s.buf.Write(buf[:n])

				s.forwardMu.Lock()
				if s.forwardW != nil {
					s.forwardW.Write(buf[:n])
				}
				s.forwardMu.Unlock()
			}
			if err != nil {
				break
			}
		}
		s.mu.Lock()
		s.exitErr = s.cmd.Wait()
		s.mu.Unlock()
		close(s.done)
	}()
}

// SetForward sets (or clears) the writer that receives a copy of PTY output.
// Pass nil to stop forwarding.
func (s *ManagedSession) SetForward(w io.Writer) {
	s.forwardMu.Lock()
	defer s.forwardMu.Unlock()
	s.forwardW = w
}

// CaptureOutput returns the current ring buffer contents as a string
// with ANSI escape sequences stripped.
func (s *ManagedSession) CaptureOutput() string {
	return stripANSI(string(s.buf.Read()))
}

// IsRunning returns true if the process is still alive.
func (s *ManagedSession) IsRunning() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// PTY returns the PTY master file descriptor (for writing stdin to it).
func (s *ManagedSession) PTY() *os.File {
	return s.ptmx
}

// Done returns a channel that is closed when the process exits.
func (s *ManagedSession) Done() <-chan struct{} {
	return s.done
}

// Stop sends SIGTERM to the process and closes the PTY.
func (s *ManagedSession) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Signal(os.Interrupt)
	}
	if s.ptmx != nil {
		s.ptmx.Close()
	}
}
