package sshlib

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPublicKeyPaths returns common public key locations in order of preference.
func DefaultPublicKeyPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	sshDir := filepath.Join(home, ".ssh")
	return []string{
		filepath.Join(sshDir, "id_ed25519.pub"),
		filepath.Join(sshDir, "id_ecdsa.pub"),
		filepath.Join(sshDir, "id_rsa.pub"),
	}
}

// FindDefaultPublicKey searches for the first existing public key file.
func FindDefaultPublicKey() (string, error) {
	for _, p := range DefaultPublicKeyPaths() {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no public key found in ~/.ssh/ (expected id_ed25519.pub, id_ecdsa.pub, or id_rsa.pub)")
}

// CopyPublicKey deploys a local public key to the remote server's authorized_keys.
func CopyPublicKey(ctx context.Context, opts *Options, pubKeyPath string) error {
	if pubKeyPath == "" {
		var err error
		pubKeyPath, err = FindDefaultPublicKey()
		if err != nil {
			return err
		}
	}

	pubKeyBytes, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return fmt.Errorf("read public key %s: %w", pubKeyPath, err)
	}

	pubKey := strings.TrimSpace(string(pubKeyBytes))
	if pubKey == "" {
		return fmt.Errorf("public key file %s is empty", pubKeyPath)
	}

	client, _, err := Dial(ctx, opts)
	if err != nil {
		return fmt.Errorf("connect target: %w", err)
	}
	defer client.Close()

	// 1. Prepare ~/.ssh and authorized_keys with strict permissions
	prepCmd := `umask 077 && mkdir -p ~/.ssh && chmod 700 ~/.ssh && touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys`
	if _, err := RunCommand(ctx, opts, prepCmd); err != nil {
		return fmt.Errorf("prepare ~/.ssh directory on remote host: %w", err)
	}

	// 2. Check if key is already installed to avoid duplicate lines
	// Compare key comment or raw key payload
	parts := strings.Fields(pubKey)
	keyPattern := pubKey
	if len(parts) >= 2 {
		// match key type and base64 body
		keyPattern = parts[0] + " " + parts[1]
	}

	checkCmd := fmt.Sprintf("grep -q -F %s ~/.ssh/authorized_keys", shellQuote(keyPattern))
	_, checkErr := RunCommand(ctx, opts, checkCmd)
	if checkErr == nil {
		return fmt.Errorf("public key already present in ~/.ssh/authorized_keys on %s", opts.Host)
	}

	// 3. Append public key safely
	appendCmd := fmt.Sprintf("printf '%%s\\n' %s >> ~/.ssh/authorized_keys", shellQuote(pubKey))
	if _, err := RunCommand(ctx, opts, appendCmd); err != nil {
		return fmt.Errorf("append public key to authorized_keys: %w", err)
	}

	return nil
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
