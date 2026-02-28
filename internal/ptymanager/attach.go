package ptymanager

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

const detachByte = 0x1d // Ctrl+]

// AttachFunc returns a function that attaches stdin/stdout to a PTY session.
// Output is forwarded via the session's single reader goroutine (SetForward),
// avoiding concurrent reads on the PTY fd.
// The user can detach with Ctrl+] while Claude keeps running in the background.
func AttachFunc(mgr *Manager, sessionID string) func() error {
	return func() error {
		ptmx, done, err := mgr.Attach(sessionID)
		if err != nil {
			return err
		}

		// Set terminal to raw mode
		fd := int(os.Stdin.Fd())
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return err
		}
		defer term.Restore(fd, oldState)

		// Sync PTY size with real terminal
		syncSize(fd, ptmx)

		// Forward SIGWINCH to PTY
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)
		defer signal.Stop(sigCh)

		go func() {
			for range sigCh {
				syncSize(fd, ptmx)
			}
		}()

		// Forward PTY output to stdout via the session's reader goroutine.
		mgr.SetForward(sessionID, os.Stdout)
		defer mgr.SetForward(sessionID, nil)

		// Dup stdin so we can close the dup to interrupt blocking reads
		// when the process exits. This prevents a goroutine leak that
		// would compete with bubbletea's input reader after we return.
		dupFd, err := syscall.Dup(fd)
		if err != nil {
			return err
		}
		stdinFile := os.NewFile(uintptr(dupFd), "stdin")

		// stdin → ptmx (with Ctrl+] detection)
		stdinDone := make(chan struct{})
		go func() {
			defer close(stdinDone)
			buf := make([]byte, 1024)
			for {
				n, err := stdinFile.Read(buf)
				if err != nil {
					return
				}
				for i := 0; i < n; i++ {
					if buf[i] == detachByte {
						return
					}
				}
				ptmx.Write(buf[:n])
			}
		}()

		// Wait for detach (stdin goroutine exits) or process exit
		select {
		case <-stdinDone:
			// User pressed Ctrl+] or stdin error — goroutine already exited
			stdinFile.Close()
		case <-done:
			// Process exited — close dup'd fd to unblock the read goroutine
			stdinFile.Close()
			<-stdinDone
		}

		return nil
	}
}

// syncSize reads the current terminal size and applies it to the PTY.
func syncSize(fd int, ptmx *os.File) {
	w, h, err := term.GetSize(fd)
	if err == nil {
		pty.Setsize(ptmx, &pty.Winsize{
			Rows: uint16(h),
			Cols: uint16(w),
		})
	}
}
