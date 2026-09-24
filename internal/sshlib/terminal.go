package sshlib

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// InteractiveSession runs a full interactive PTY session over the connected SSH client.
// It switches the local terminal to raw mode, attaches stdin/stdout/stderr, and tracks window resizes.
func (c *Client) InteractiveSession(ctx context.Context, initialCommand string) error {
	session, err := c.NewSession()
	if err != nil {
		return fmt.Errorf("sshlib: open session: %w", err)
	}
	defer session.Close()

	// Get local terminal file descriptor
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		fd = int(os.Stdout.Fd())
	}

	termType := os.Getenv("TERM")
	if termType == "" {
		termType = "xterm-256color"
	}

	w, h := 80, 24
	if term.IsTerminal(fd) {
		if tw, th, err := term.GetSize(fd); err == nil && tw > 0 && th > 0 {
			w, h = tw, th
		}
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // enable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	if err := session.RequestPty(termType, h, w, modes); err != nil {
		return fmt.Errorf("sshlib: request pty failed: %w", err)
	}

	// Set raw mode on standard input
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("sshlib: make terminal raw: %w", err)
		}
		defer func() {
			_ = term.Restore(int(os.Stdin.Fd()), oldState)
		}()
	}

	// Forward stdio
	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	// Watch window resizing
	winchCh := make(chan os.Signal, 1)
	notifyWinch(winchCh)
	defer signal.Stop(winchCh)

	go func() {
		for range winchCh {
			if tw, th, err := term.GetSize(fd); err == nil && tw > 0 && th > 0 {
				_ = session.WindowChange(th, tw)
			}
		}
	}()

	// Start shell or initial command
	if initialCommand != "" {
		if err := session.Start(initialCommand); err != nil {
			return fmt.Errorf("sshlib: start command failed: %w", err)
		}
	} else {
		if err := session.Shell(); err != nil {
			return fmt.Errorf("sshlib: request shell failed: %w", err)
		}
	}

	// Wait for session to finish or context cancellation
	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
	}()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return ctx.Err()
	case err := <-done:
		if err != nil && err != io.EOF {
			// If remote exited normally with exit code (e.g. user typed 'exit'),
			// session.Wait() might return ExitError, which is normal exit.
			if _, ok := err.(*ssh.ExitError); ok {
				return nil
			}
			return err
		}
		return nil
	}
}

// ConnectInteractive wraps Dial and InteractiveSession in a single call.
func ConnectInteractive(opts *Options, initialCommand string) error {
	ctx := context.Background()
	client, _, err := Dial(ctx, opts)
	if err != nil {
		return err
	}
	defer client.Close()

	// Keep alive ticker
	stopKeepAlive := make(chan struct{})
	defer close(stopKeepAlive)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopKeepAlive:
				return
			case <-ticker.C:
				_, _, _ = client.SendRequest("keepalive@openssh.com", true, nil)
			}
		}
	}()

	return client.InteractiveSession(ctx, initialCommand)
}
