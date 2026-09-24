// Package sshlib provides a pure-Go SSH client supporting password,
// key and ssh-agent authentication, optional jump host (bastion)
// chaining, and OpenSSH known_hosts verification.
package sshlib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// CommonPasswords is the default fallback password list. It stays
// compatible with the original Python tool (server_manager.py).
var CommonPasswords = []string{
	"Uniontech@2023",
	"Uniontech@2024",
	"uos123..",
}

// Credential is a password candidate with a human readable label used
// for diagnostics (e.g. "stored password", "common password #2").
type Credential struct {
	Secret string
	Label  string
}

// AuthOptions describes how to authenticate to a host.
type AuthOptions struct {
	User           string
	Passwords      []Credential
	PrivateKeyPath string
	KeyPassphrase  string
	TryAgent       bool
}

// Options describes a single SSH hop. ProxyJump, when set, is dialed
// first and used as the transport for this hop.
type Options struct {
	Host            string
	Port            int
	Auth            AuthOptions
	ProxyJump       *Options
	Timeout         time.Duration
	InsecureHostKey bool        // skip host key verification entirely
	AutoAddHostKey  bool        // add unknown host keys to known_hosts
	KnownHostsFile  string      // defaults to ~/.ssh/known_hosts
	Dialer          DialContext // optional custom dialer (testing)
}

// DialContext is the signature used for opening the TCP connection to a
// host. It exists so tests can substitute an in-memory transport.
type DialContext func(ctx context.Context, network, addr string) (net.Conn, error)

// AuthResult reports which authentication method and credential were
// accepted by the server.
type AuthResult struct {
	Method string `json:"method"` // key | agent | password | keyboard-interactive | none
	Source string `json:"source"` // human readable credential label
}

// TestResult is a summary of a connection test.
type TestResult struct {
	Success       bool   `json:"success"`
	LatencyMs     int64  `json:"latencyMs"`
	AuthMethod    string `json:"authMethod"`
	AuthSource    string `json:"authSource"`
	ServerVersion string `json:"serverVersion"`
	Message       string `json:"message"`
}

// Client wraps an ssh.Client and keeps any intermediate hop clients
// alive until Close is called.
type Client struct {
	*ssh.Client
	hops []io.Closer
}

// Close closes the target connection and every hop used to reach it.
func (c *Client) Close() error {
	var errs []error
	if c.Client != nil {
		if err := c.Client.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, h := range c.hops {
		if err := h.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// CredentialByLabel returns a map of credential label to secret for this
// hop and its ProxyJump chain. It is used to look up the exact secret
// that won authentication.
func (o *Options) CredentialByLabel() map[string]string {
	m := make(map[string]string)
	for _, c := range o.Auth.Passwords {
		m[c.Label] = c.Secret
	}
	if o.ProxyJump != nil {
		for _, c := range o.ProxyJump.Auth.Passwords {
			m[c.Label] = c.Secret
		}
	}
	return m
}

// Dial establishes an SSH connection, dialing the ProxyJump chain first
// when configured. It returns the connected client and the auth result.
func Dial(ctx context.Context, opts *Options) (*Client, *AuthResult, error) {
	if opts == nil {
		return nil, nil, errors.New("sshlib: options are required")
	}
	if opts.Host == "" {
		return nil, nil, errors.New("sshlib: host is required")
	}
	if opts.Port == 0 {
		opts.Port = 22
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	addr := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))

	if opts.ProxyJump != nil {
		bastion, _, err := Dial(ctx, opts.ProxyJump)
		if err != nil {
			return nil, nil, fmt.Errorf("sshlib: connect via bastion %s: %w", jumpAddr(opts.ProxyJump), err)
		}
		conn, err := bastion.Dial("tcp", addr)
		if err != nil {
			bastion.Close()
			return nil, nil, fmt.Errorf("sshlib: bastion %s cannot reach %s: %w", jumpAddr(opts.ProxyJump), addr, err)
		}
		client, auth, err := handshake(ctx, conn, addr, opts)
		if err != nil {
			bastion.Close()
			return nil, nil, err
		}
		client.hops = append(client.hops, bastion)
		return client, auth, nil
	}

	dial := opts.Dialer
	if dial == nil {
		dial = (&net.Dialer{Timeout: opts.Timeout}).DialContext
	}
	conn, err := dial(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("sshlib: connect %s: %w", addr, err)
	}
	return handshake(ctx, conn, addr, opts)
}

func handshake(ctx context.Context, conn net.Conn, addr string, opts *Options) (*Client, *AuthResult, error) {
	tracker := newAuthTracker()
	config := &ssh.ClientConfig{
		User:            opts.Auth.User,
		Auth:            buildAuthMethods(opts, tracker),
		HostKeyCallback: hostKeyCallback(opts),
		Timeout:         opts.Timeout,
		HostKeyAlgorithms: []string{
			ssh.KeyAlgoED25519,
			ssh.KeyAlgoECDSA256,
			ssh.KeyAlgoECDSA384,
			ssh.KeyAlgoECDSA521,
			ssh.KeyAlgoRSASHA512,
			ssh.KeyAlgoRSASHA256,
			ssh.KeyAlgoRSA,
		},
	}

	type result struct {
		client *ssh.Client
		err    error
	}
	done := make(chan result, 1)
	go func() {
		sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
		if err != nil {
			done <- result{err: err}
			return
		}
		done <- result{client: ssh.NewClient(sshConn, chans, reqs)}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			return nil, nil, fmt.Errorf("sshlib: ssh handshake with %s failed: %w", addr, tracker.describeFailure(r.err))
		}
		return &Client{Client: r.client}, tracker.result(), nil
	case <-ctx.Done():
		conn.Close()
		return nil, nil, fmt.Errorf("sshlib: ssh handshake with %s: %w", addr, ctx.Err())
	}
}

// TestConnection dials, opens a session to prove the connection works,
// then closes it, reporting latency and the accepted auth method.
func TestConnection(ctx context.Context, opts *Options) (*TestResult, error) {
	start := time.Now()
	client, auth, err := Dial(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	// Prove the session works, not just that auth succeeded.
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("sshlib: open session: %w", err)
	}
	if err := session.Run("true"); err != nil {
		session.Close()
		return nil, fmt.Errorf("sshlib: session check failed: %w", err)
	}
	session.Close()

	latency := time.Since(start)
	res := &TestResult{
		Success:       true,
		LatencyMs:     latency.Milliseconds(),
		ServerVersion: string(client.ServerVersion()),
		Message:       "connection ok",
	}
	if auth != nil {
		res.AuthMethod = auth.Method
		res.AuthSource = auth.Source
	}
	return res, nil
}

// RunCommand connects and executes command on the remote host,
// returning combined stdout+stderr.
func RunCommand(ctx context.Context, opts *Options, command string) (string, error) {
	client, _, err := Dial(ctx, opts)
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("sshlib: open session: %w", err)
	}
	defer session.Close()

	var out, errOut bytes.Buffer
	session.Stdout = &out
	session.Stderr = &errOut

	type result struct {
		err error
	}
	done := make(chan result, 1)
	go func() {
		done <- result{err: session.Run(command)}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			msg := strings.TrimSpace(out.String() + errOut.String())
			if msg != "" {
				return msg, fmt.Errorf("sshlib: command failed: %w: %s", r.err, msg)
			}
			return "", fmt.Errorf("sshlib: command failed: %w", r.err)
		}
		return out.String(), nil
	case <-ctx.Done():
		session.Close()
		return "", fmt.Errorf("sshlib: command: %w", ctx.Err())
	}
}

func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// buildAuthMethods assembles the auth methods in preference order:
// key file, ssh-agent, then the password candidates (both the
// "password" and "keyboard-interactive" methods, since many servers
// only enable one of them).
func buildAuthMethods(opts *Options, tracker *authTracker) []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	var keyPaths []string
	if opts.Auth.PrivateKeyPath != "" {
		keyPaths = append(keyPaths, opts.Auth.PrivateKeyPath)
	}

	// In auto mode, if no key is explicitly given or in addition, try standard ~/.ssh keys
	if opts.Auth.TryAgent {
		if home, err := os.UserHomeDir(); err == nil {
			candidates := []string{
				filepath.Join(home, ".ssh", "id_rsa_uos"),
				filepath.Join(home, ".ssh", "id_ed25519"),
				filepath.Join(home, ".ssh", "id_rsa"),
				filepath.Join(home, ".ssh", "id_ecdsa"),
			}
			for _, c := range candidates {
				if _, err := os.Stat(c); err == nil {
					keyPaths = append(keyPaths, c)
				}
			}
		}
	}

	// De-duplicate key paths
	seenKeys := make(map[string]bool)
	for _, p := range keyPaths {
		expanded := ExpandPath(p)
		if seenKeys[expanded] {
			continue
		}
		seenKeys[expanded] = true

		signer, err := loadSigner(expanded, opts.Auth.KeyPassphrase)
		if err == nil {
			label := "key " + p
			methods = append(methods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
				tracker.record("key", label)
				return []ssh.Signer{signer}, nil
			}))
		}
	}

	if opts.Auth.TryAgent {
		if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
			if conn, err := net.Dial("unix", sock); err == nil {
				ag := agent.NewClient(conn)
				methods = append(methods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
					signers, err := ag.Signers()
					if err != nil {
						return nil, err
					}
					tracker.record("agent", "ssh-agent")
					return signers, nil
				}))
			}
		}
	}

	pwds := opts.Auth.Passwords
	if len(pwds) > 0 {
		pwIdx := 0
		methods = append(methods, ssh.PasswordCallback(func() (string, error) {
			if pwIdx >= len(pwds) {
				return "", tracker.exhausted()
			}
			c := pwds[pwIdx]
			pwIdx++
			tracker.record("password", c.Label)
			return c.Secret, nil
		}))

		kbdIdx := 0
		methods = append(methods, ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			if kbdIdx >= len(pwds) {
				return nil, tracker.exhausted()
			}
			c := pwds[kbdIdx]
			kbdIdx++
			tracker.record("keyboard-interactive", c.Label)
			answers := make([]string, len(questions))
			for i := range answers {
				answers[i] = c.Secret
			}
			return answers, nil
		}))
	}

	return methods
}

func loadSigner(path, passphrase string) (ssh.Signer, error) {
	path = ExpandPath(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if passphrase == "" {
		signer, err := ssh.ParsePrivateKey(data)
		if err == nil {
			return signer, nil
		}
		var pe *ssh.PassphraseMissingError
		if errors.As(err, &pe) {
			return nil, errors.New("key requires a passphrase")
		}
		return nil, err
	}
	return ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
}

// hostKeyCallback returns the host key verification callback. It
// verifies against the user's known_hosts file and, when
// AutoAddHostKey is set, records unknown hosts on first contact.
func hostKeyCallback(opts *Options) ssh.HostKeyCallback {
	if opts.InsecureHostKey {
		return ssh.InsecureIgnoreHostKey()
	}

	khFile := opts.KnownHostsFile
	if khFile == "" {
		if home, err := os.UserHomeDir(); err == nil {
			khFile = filepath.Join(home, ".ssh", "known_hosts")
		}
	}
	if khFile == "" {
		return ssh.InsecureIgnoreHostKey()
	}

	cb, err := knownhosts.New(khFile)
	if err != nil {
		if !os.IsNotExist(err) {
			// Unreadable known_hosts: fail closed with a clear error.
			return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				return fmt.Errorf("cannot read known_hosts %q: %w", khFile, err)
			}
		}
		// No known_hosts yet: treat every host as unknown.
		cb = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return &knownhosts.KeyError{}
		}
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := cb(hostname, remote, key)
		if err == nil {
			return nil
		}
		var ke *knownhosts.KeyError
		if errors.As(err, &ke) && len(ke.Want) == 0 && opts.AutoAddHostKey {
			return appendHostKey(khFile, hostname, remote, key)
		}
		if errors.As(err, &ke) && len(ke.Want) == 0 {
			return fmt.Errorf("host key for %s is unknown (first time connecting?); fingerprint: %s",
				hostname, ssh.FingerprintSHA256(key))
		}
		if errors.As(err, &ke) && len(ke.Want) > 0 {
			var expected []string
			for _, k := range ke.Want {
				expected = append(expected, k.Key.Type()+" "+ssh.FingerprintSHA256(k.Key))
			}
			return fmt.Errorf("host key mismatch for %s (host re-imaged or MITM?); expected [%s], got %s %s",
				hostname, strings.Join(expected, ", "), key.Type(), ssh.FingerprintSHA256(key))
		}
		return err
	}
}

func appendHostKey(khFile, hostname string, remote net.Addr, key ssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(khFile), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(khFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	addresses := []string{knownhosts.Normalize(hostname)}
	if remote != nil {
		if h, _, err := net.SplitHostPort(remote.String()); err == nil {
			addresses = append(addresses, knownhosts.Normalize(h))
		}
	}
	line := knownhosts.Line(addresses, key) + "\n"
	if _, err := f.WriteString(line); err != nil {
		return err
	}
	return f.Sync()
}

func jumpAddr(opts *Options) string {
	port := opts.Port
	if port == 0 {
		port = 22
	}
	return net.JoinHostPort(opts.Host, strconv.Itoa(port))
}

// authTracker records which credential was handed to the server last.
// Because the SSH client stops on the first accepted method, the last
// recorded credential is the one that succeeded.
type authTracker struct {
	mu        sync.Mutex
	last      string
	attempted []string
}

func newAuthTracker() *authTracker {
	return &authTracker{}
}

func (t *authTracker) record(method, source string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.last = method + ":" + source
	t.attempted = append(t.attempted, source)
}

func (t *authTracker) result() *AuthResult {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last == "" {
		return &AuthResult{Method: "none", Source: "no credentials supplied"}
	}
	parts := strings.SplitN(t.last, ":", 2)
	return &AuthResult{Method: parts[0], Source: parts[1]}
}

// exhausted returns the error surfaced when every credential has been
// rejected. The returned error replaces the final ssh auth error so the
// user sees which credentials were actually tried.
func (t *authTracker) exhausted() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return fmt.Errorf("all %d credential(s) rejected: %s", len(t.attempted), strings.Join(t.attempted, ", "))
}

// describeFailure augments a handshake error with the list of
// credentials that were attempted, unless the error already carries
// that information.
func (t *authTracker) describeFailure(err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.attempted) == 0 {
		return err
	}
	if strings.Contains(err.Error(), "credential(s) rejected") {
		return err
	}
	return fmt.Errorf("%w (tried: %s)", err, strings.Join(t.attempted, ", "))
}
