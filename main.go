package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ssh-manager/internal/config"
	"ssh-manager/internal/sshlib"
	"ssh-manager/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	args := os.Args[1:]

	// If no arguments, launch the full multi-panel TUI
	if len(args) == 0 {
		runTUI(cfg)
		return
	}

	cmd := args[0]
	switch cmd {
	case "help", "-h", "--help":
		printUsage()

	case "list", "ls":
		listServers(cfg)

	case "fzf":
		query := ""
		if len(args) > 1 {
			query = args[1]
		}
		runFzf(cfg, query)

	case "connect", "c":
		if len(args) < 2 {
			fmt.Println("Usage: sshx connect <server-name>")
			os.Exit(1)
		}
		connectServer(cfg, args[1])

	case "exec", "e":
		if len(args) < 3 {
			fmt.Println("Usage: sshx exec <server-name> <command>")
			os.Exit(1)
		}
		execServer(cfg, args[1], strings.Join(args[2:], " "))

	case "test", "probe", "p":
		if len(args) < 2 {
			fmt.Println("Usage: sshx test <server-name>")
			os.Exit(1)
		}
		testServer(cfg, args[1])

	case "tunnel", "t":
		if len(args) < 2 {
			fmt.Println("Usage: sshx tunnel <server-name>")
			os.Exit(1)
		}
		runTunnels(cfg, args[1])

	case "copy-id", "k":
		if len(args) < 2 {
			fmt.Println("Usage: sshx copy-id <server-name> [pubkey-path]")
			os.Exit(1)
		}
		pubKey := ""
		if len(args) > 2 {
			pubKey = args[2]
		}
		copyID(cfg, args[1], pubKey)

	case "sync":
		action := "push"
		if len(args) > 1 {
			action = args[1]
		}
		syncGist(cfg, action)

	default:
		// If argument does not match subcommand, treat as server query (direct connect or fzf)
		query := args[0]
		// Check exact match
		if _, ok := cfg.Servers[query]; ok {
			connectServer(cfg, query)
			return
		}
		// Fzf fuzzy match
		runFzf(cfg, query)
	}
}

func runTUI(cfg *config.Config) {
	model := tui.NewModel(cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func runFzf(cfg *config.Config, query string) {
	chosen, err := tui.RunFzfQuickSelect(cfg, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if chosen != nil {
		connectServer(cfg, chosen.Name)
	}
}

func connectServer(cfg *config.Config, name string) {
	server, ok := cfg.Servers[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: server '%s' not found in configuration\n", name)
		os.Exit(1)
	}

	opts, err := cfg.ServerToSSHOptions(server)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Connecting to %s (%s@%s:%d)...\n", server.Name, server.User, server.Host, server.EffectivePort())

	ctx := context.Background()
	client, _, err := sshlib.Dial(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Start tunnels if configured
	if len(server.Tunnels) > 0 {
		tm := sshlib.NewTunnelManager(client)
		defer tm.Close()
		errs := tm.StartAll(server.Tunnels)
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "Tunnel warning: %v\n", e)
		}
		for _, t := range tm.GetTunnels() {
			if t.Status == "active" {
				fmt.Printf("✓ Tunnel [%s] %s -> %s active\n", t.Rule.Type, t.Rule.Local, t.Rule.Remote)
			}
		}
	}

	if err := client.InteractiveSession(ctx, ""); err != nil {
		fmt.Fprintf(os.Stderr, "Session ended: %v\n", err)
	}
}

func testServer(cfg *config.Config, name string) {
	server, ok := cfg.Servers[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: server '%s' not found\n", name)
		os.Exit(1)
	}

	opts, err := cfg.ServerToSSHOptions(server)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Testing connection to '%s' (%s@%s:%d)...\n", server.Name, server.User, server.Host, server.EffectivePort())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := sshlib.TestConnection(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ Connection failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Connection Successful!")
	fmt.Printf("  Latency:     %d ms\n", res.LatencyMs)
	fmt.Printf("  Auth Method: %s\n", res.AuthMethod)
	if res.AuthSource != "" {
		fmt.Printf("  Credential:  %s\n", res.AuthSource)
	}
	fmt.Printf("  Server:      %s\n", res.ServerVersion)
}

func execServer(cfg *config.Config, name, command string) {
	server, ok := cfg.Servers[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: server '%s' not found\n", name)
		os.Exit(1)
	}

	opts, err := cfg.ServerToSSHOptions(server)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out, err := sshlib.RunCommand(ctx, opts, command)
	if err != nil {
		if out != "" {
			fmt.Println(out)
		}
		fmt.Fprintf(os.Stderr, "Command failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(out)
}

func runTunnels(cfg *config.Config, name string) {
	server, ok := cfg.Servers[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: server '%s' not found\n", name)
		os.Exit(1)
	}

	if len(server.Tunnels) == 0 {
		fmt.Fprintf(os.Stderr, "Error: server '%s' has no tunnels configured\n", name)
		os.Exit(1)
	}

	opts, err := cfg.ServerToSSHOptions(server)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Connecting to %s to establish %d tunnels...\n", server.Name, len(server.Tunnels))
	client, _, err := sshlib.Dial(context.Background(), opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	tm := sshlib.NewTunnelManager(client)
	defer tm.Close()

	errs := tm.StartAll(server.Tunnels)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "Tunnel error: %v\n", e)
		}
	}

	for _, t := range tm.GetTunnels() {
		if t.Status == "active" {
			fmt.Printf("✓ Listening [%s]: %s -> %s\n", t.Rule.Type, t.Rule.Local, t.Rule.Remote)
		}
	}

	fmt.Println("\nTunnels are active. Press Ctrl+C to stop.")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\nStopping tunnels...")
}

func copyID(cfg *config.Config, name, pubKeyPath string) {
	server, ok := cfg.Servers[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: server '%s' not found\n", name)
		os.Exit(1)
	}

	opts, err := cfg.ServerToSSHOptions(server)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deploying public key to '%s' (%s@%s)...\n", server.Name, server.User, server.Host)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := sshlib.CopyPublicKey(ctx, opts, pubKeyPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Successfully installed public key in remote ~/.ssh/authorized_keys!")
	fmt.Println("  You can now log in passwordlessly with key authentication.")
}

func syncGist(cfg *config.Config, action string) {
	switch strings.ToLower(action) {
	case "push":
		fmt.Println("Pushing configuration to GitHub Secret Gist...")
		id, err := config.PushToGist(cfg, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Gist push failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Gist updated successfully! Gist ID: %s\n", id)
	case "pull":
		token := cfg.Settings.Gist.Token
		gistID := cfg.Settings.Gist.GistID
		if token == "" || gistID == "" {
			fmt.Fprintln(os.Stderr, "Error: GitHub Token or Gist ID not configured in settings")
			os.Exit(1)
		}
		fmt.Printf("Pulling configuration from Gist %s...\n", gistID)
		newCfg, err := config.PullFromGist(token, gistID, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Gist pull failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Successfully pulled %d servers from Gist!\n", len(newCfg.Servers))
	default:
		fmt.Println("Usage: sshx sync [push|pull]")
	}
}

func listServers(cfg *config.Config) {
	servers := cfg.SortedServers()
	if len(servers) == 0 {
		fmt.Println("No servers configured.")
		return
	}

	fmt.Printf("%-18s %-12s %-24s %-12s %s\n", "NAME", "GROUP", "ENDPOINT", "AUTH", "BASTION/TUNNELS")
	fmt.Println(strings.Repeat("-", 80))
	for _, s := range servers {
		endpoint := fmt.Sprintf("%s@%s:%d", s.User, s.Host, s.EffectivePort())
		auth := s.AuthMethod
		if auth == "" {
			auth = "auto"
		}
		extra := ""
		if s.Bastion != "" {
			extra += "via:" + s.Bastion + " "
		}
		if len(s.Tunnels) > 0 {
			extra += fmt.Sprintf("(%d tunnels)", len(s.Tunnels))
		}
		fmt.Printf("%-18s %-12s %-24s %-12s %s\n", s.Name, s.EffectiveGroup(), endpoint, auth, extra)
	}
}

func printUsage() {
	fmt.Println(`SSH Manager (sshx) - Terminal SSH & Port Forwarding Manager

Usage:
  sshx                           Launch interactive multi-panel TUI (Lazygit/K9s style)
  sshx <server-name>             Connect directly or search via fast fzf
  sshx connect <server-name>     Connect to interactive SSH terminal session
  sshx exec <server-name> <cmd>  Execute remote command directly
  sshx test <server-name>        Probe connection latency, auth method and server version
  sshx tunnel <server-name>      Start port forwarding rules (-L, -R, -D) in foreground
  sshx copy-id <server-name>     Deploy local public key to remote authorized_keys (ssh-copy-id)
  sshx list                      List all configured servers and status
  sshx fzf [query]               Interactive quick search and connect
  sshx sync [push|pull]          Backup or restore configuration to/from GitHub Secret Gist

Configuration:
  Stored in ~/.ssh/ssh-manager.json (standalone, does not modify system ~/.ssh/config)`)
}
