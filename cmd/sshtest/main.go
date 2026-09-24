// Command sshtest is a CLI test harness for the sshlib package. It
// exercises password / key / common-password authentication, jump host
// (bastion) chaining, and remote command execution without launching
// the desktop app.
//
// Examples:
//
//	sshtest --host 10.0.35.39 --user uos --password 'Uniontech@2023'
//	sshtest --host 10.0.35.40 --user uos --common --bastion-host 10.0.35.39 --bastion-user uos
//	sshtest --host 10.0.35.40 --user uos --key ~/.ssh/id_ed25519
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zalando/go-keyring"

	"ssh-manager/internal/sshlib"
)

const (
	keyringServiceName = "server_manager_cli"
	appConfigFile      = ".server_manager.json"
)

func main() {
	var (
		host        = flag.String("host", "", "target host")
		port        = flag.Int("port", 22, "target port")
		user        = flag.String("user", "", "target user")
		passwords   = stringListFlag{}
		useCommon   = flag.Bool("common", false, "also try the built-in common passwords")
		keyPath     = flag.String("key", "", "private key path")
		keyPass     = flag.String("key-passphrase", "", "private key passphrase")
		agent       = flag.Bool("agent", false, "also try ssh-agent")
		bastionHost = flag.String("bastion-host", "", "bastion host (enables jump host)")
		bastionPort = flag.Int("bastion-port", 22, "bastion port")
		bastionUser = flag.String("bastion-user", "", "bastion user")
		bastionPwds = stringListFlag{}
		bastionKey  = flag.String("bastion-key", "", "bastion private key path")
		timeout     = flag.Duration("timeout", 10*time.Second, "per-connection timeout")
		insecure    = flag.Bool("insecure", false, "skip host key verification (testing only)")
		acceptNew   = flag.Bool("accept-new", false, "auto-add unknown host keys to known_hosts")
		command     = flag.String("command", "", "remote command to run after connecting")
		asJSON      = flag.Bool("json", false, "print result as JSON")
		serverName  = flag.String("server", "", "read host/user/port/bastion from the app config by name")
		keyringPwd  = flag.Bool("keyring", false, "also read the password from the app keychain by server name")
	)
	flag.Var(&passwords, "password", "password candidate (repeatable)")
	flag.Var(&bastionPwds, "bastion-password", "bastion password candidate (repeatable)")
	flag.Parse()

	if *host == "" && *serverName == "" {
		fail("--host (or --server) is required")
	}

	opts := &sshlib.Options{
		Host:            *host,
		Port:            *port,
		Timeout:         *timeout,
		InsecureHostKey: *insecure,
		AutoAddHostKey:  *acceptNew,
		Auth: sshlib.AuthOptions{
			User:           *user,
			PrivateKeyPath: *keyPath,
			KeyPassphrase:  *keyPass,
			TryAgent:       *agent,
		},
	}

	if *serverName != "" {
		if err := applyConfig(opts, *serverName); err != nil {
			fail(err.Error())
		}
		if *keyringPwd {
			if pw, err := keyring.Get(keyringServiceName, *serverName); err == nil && pw != "" {
				opts.Auth.Passwords = append(opts.Auth.Passwords, sshlib.Credential{Secret: pw, Label: "keyring password"})
			}
		}
	}

	for _, pw := range passwords {
		opts.Auth.Passwords = append(opts.Auth.Passwords, sshlib.Credential{Secret: pw, Label: "flag password"})
	}
	if *useCommon {
		for i, pw := range sshlib.CommonPasswords {
			opts.Auth.Passwords = append(opts.Auth.Passwords, sshlib.Credential{
				Secret: pw,
				Label:  fmt.Sprintf("common password #%d", i+1),
			})
		}
	}
	if *bastionHost != "" || *bastionUser != "" {
		opts.ProxyJump = &sshlib.Options{
			Host:            *bastionHost,
			Port:            *bastionPort,
			Timeout:         *timeout,
			InsecureHostKey: *insecure,
			AutoAddHostKey:  *acceptNew,
			Auth: sshlib.AuthOptions{
				User:           *bastionUser,
				PrivateKeyPath: *bastionKey,
			},
		}
		for _, pw := range bastionPwds {
			opts.ProxyJump.Auth.Passwords = append(opts.ProxyJump.Auth.Passwords, sshlib.Credential{Secret: pw, Label: "bastion flag password"})
		}
	}
	if opts.ProxyJump != nil {
		// Flag-provided credentials apply to every hop.
		for _, pw := range passwords {
			opts.ProxyJump.Auth.Passwords = append(opts.ProxyJump.Auth.Passwords, sshlib.Credential{Secret: pw, Label: "flag password"})
		}
		if *useCommon {
			for i, pw := range sshlib.CommonPasswords {
				opts.ProxyJump.Auth.Passwords = append(opts.ProxyJump.Auth.Passwords, sshlib.Credential{
					Secret: pw,
					Label:  fmt.Sprintf("common password #%d", i+1),
				})
			}
		}
	}

	if opts.Auth.User == "" {
		fail("--user is required")
	}
	if opts.Host == "" {
		fail("--host is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout*3)
	defer cancel()

	start := time.Now()
	client, auth, err := sshlib.Dial(ctx, opts)
	if err != nil {
		if *asJSON {
			printJSON(map[string]any{"success": false, "error": err.Error(), "latencyMs": time.Since(start).Milliseconds()})
		} else {
			fmt.Fprintf(os.Stderr, "connection failed after %v: %v\n", time.Since(start).Round(time.Millisecond), err)
		}
		os.Exit(1)
	}
	defer client.Close()

	chain := describeChain(opts)
	out := map[string]any{
		"success":       true,
		"host":          opts.Host,
		"port":          opts.Port,
		"user":          opts.Auth.User,
		"chain":         chain,
		"authMethod":    auth.Method,
		"authSource":    auth.Source,
		"serverVersion": string(client.ServerVersion()),
		"latencyMs":     time.Since(start).Milliseconds(),
	}

	if *command != "" {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, *timeout)
		defer cmdCancel()
		output, cmdErr := sshlib.RunCommand(cmdCtx, opts, *command)
		if cmdErr != nil {
			out["command"] = *command
			out["commandError"] = cmdErr.Error()
			out["commandOutput"] = output
		} else {
			out["command"] = *command
			out["commandOutput"] = strings.TrimRight(output, "\n")
		}
	}

	if *asJSON {
		printJSON(out)
	} else {
		fmt.Printf("OK   %s@%s:%d via %s\n", opts.Auth.User, opts.Host, opts.Port, chain)
		fmt.Printf("auth method: %s (source: %s)\n", auth.Method, auth.Source)
		fmt.Printf("server:      %s\n", out["serverVersion"])
		fmt.Printf("latency:     %d ms\n", out["latencyMs"])
		if cmdOut, ok := out["commandOutput"]; ok {
			fmt.Printf("command output:\n%s\n", cmdOut)
		}
		if cmdErr, ok := out["commandError"]; ok {
			fmt.Printf("command error: %s\n", cmdErr)
		}
	}
}

// applyConfig fills the target options from the app config file
// (~/.server_manager.json), including the bastion chain.
func applyConfig(opts *sshlib.Options, name string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(home, appConfigFile))
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var servers map[string]struct {
		Name       string `json:"name"`
		Host       string `json:"host"`
		Port       int    `json:"port"`
		User       string `json:"user"`
		Bastion    string `json:"bastion"`
		AuthMethod string `json:"authMethod"`
		KeyPath    string `json:"privateKeyPath"`
	}
	if err := json.Unmarshal(data, &servers); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	s, ok := servers[name]
	if !ok {
		return fmt.Errorf("server '%s' not found in config", name)
	}
	opts.Host = s.Host
	if s.Port > 0 {
		opts.Port = s.Port
	}
	opts.Auth.User = s.User
	opts.Auth.PrivateKeyPath = s.KeyPath
	if s.Bastion != "" {
		b, ok := servers[s.Bastion]
		if !ok {
			return fmt.Errorf("bastion '%s' not found in config", s.Bastion)
		}
		opts.ProxyJump = &sshlib.Options{
			Host:            b.Host,
			Port:            b.Port,
			Timeout:         opts.Timeout,
			InsecureHostKey: opts.InsecureHostKey,
			AutoAddHostKey:  opts.AutoAddHostKey,
			Auth: sshlib.AuthOptions{
				User:           b.User,
				PrivateKeyPath: b.KeyPath,
			},
		}
		if opts.ProxyJump.Port == 0 {
			opts.ProxyJump.Port = 22
		}
	}
	return nil
}

func describeChain(opts *sshlib.Options) string {
	var hops []string
	for o := opts; o != nil; o = o.ProxyJump {
		hops = append(hops, fmt.Sprintf("%s@%s:%d", o.Auth.User, o.Host, o.Port))
	}
	return strings.Join(hops, " -> ")
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(2)
}

// stringListFlag collects repeated string flags.
type stringListFlag []string

func (s *stringListFlag) String() string { return strings.Join(*s, ",") }

func (s *stringListFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}
