package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"ssh-manager/internal/sshlib"

	"github.com/zalando/go-keyring"
)

const (
	ConfigFileName     = "ssh-manager.json"
	keyringServiceName = "ssh_manager_cli"
	keyPassSuffix      = "_key_pass"
)

// Server defines an SSH host configuration.
type Server struct {
	Name               string              `json:"name"`
	Group              string              `json:"group"`
	Host               string              `json:"host"`
	Port               int                 `json:"port,omitempty"`
	User               string              `json:"user"`
	AuthMethod         string              `json:"authMethod,omitempty"` // auto, password, key
	PrivateKeyPath     string              `json:"privateKeyPath,omitempty"`
	Password           string              `json:"password,omitempty"`
	KeyPassphrase      string              `json:"keyPassphrase,omitempty"`
	UseCommonPasswords bool                `json:"useCommonPasswords,omitempty"`
	Bastion            string              `json:"bastion,omitempty"`
	Tunnels            []sshlib.TunnelRule `json:"tunnels,omitempty"`
	Notes              string              `json:"notes,omitempty"`
}

// EffectivePort returns the SSH port defaulting to 22.
func (s Server) EffectivePort() int {
	if s.Port <= 0 || s.Port > 65535 {
		return 22
	}
	return s.Port
}

// EffectiveGroup returns the group defaulting to "Default".
func (s Server) EffectiveGroup() string {
	if strings.TrimSpace(s.Group) == "" {
		return "Default"
	}
	return strings.TrimSpace(s.Group)
}

// GistConfig stores GitHub Gist backup options.
type GistConfig struct {
	Token     string `json:"token,omitempty"`
	GistID    string `json:"gistId,omitempty"`
	Encrypted bool   `json:"encrypted,omitempty"`
}

// Settings stores user preferences.
type Settings struct {
	DefaultPrivateKey string     `json:"defaultPrivateKey,omitempty"`
	Theme             string     `json:"theme,omitempty"`
	Gist              GistConfig `json:"gist,omitempty"`
}

// Config is the root configuration structure.
type Config struct {
	Version  int               `json:"version"`
	Settings Settings          `json:"settings"`
	Servers  map[string]Server `json:"servers"`
}

var (
	cfgMu sync.RWMutex
)

// GetConfigPath returns ~/.ssh/ssh-manager.json.
func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(sshDir, ConfigFileName), nil
}

// LoadConfig reads the configuration file from ~/.ssh/ssh-manager.json.
func LoadConfig() (*Config, error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := &Config{
			Version:  1,
			Settings: Settings{DefaultPrivateKey: "~/.ssh/id_rsa_uos"},
			Servers:  make(map[string]Server),
		}
		migrateLegacy(cfg)
		_ = saveConfigLocked(cfg)
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Servers == nil {
		cfg.Servers = make(map[string]Server)
	}

	// Normalize
	for name, s := range cfg.Servers {
		if s.Name == "" {
			s.Name = name
			cfg.Servers[name] = s
		}
	}
	return &cfg, nil
}

func migrateLegacy(cfg *Config) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	legacyPath := filepath.Join(home, ".server_manager.json")
	data, err := os.ReadFile(legacyPath)
	if err == nil {
		var legacyMap map[string]Server
		if err := json.Unmarshal(data, &legacyMap); err == nil {
			for k, v := range legacyMap {
				if v.Name == "" {
					v.Name = k
				}
				if v.UseCommonPasswords == false {
					v.UseCommonPasswords = true
				}
				cfg.Servers[v.Name] = v
			}
		}
	}

	// Ensure 197 is present if not already added
	if _, ok := cfg.Servers["197"]; !ok {
		cfg.Servers["197"] = Server{
			Name:               "197",
			Group:              "wh_arm",
			Host:               "10.0.35.197",
			Port:               22,
			User:               "mode",
			AuthMethod:         "auto",
			Password:           "Uniontech@2023",
			PrivateKeyPath:     "~/.ssh/id_rsa_uos",
			UseCommonPasswords: true,
		}
	}

	// Ensure 238 has password or common passwords enabled
	if s, ok := cfg.Servers["238"]; ok {
		if s.Password == "" {
			s.Password = "Uniontech@2023"
		}
		s.UseCommonPasswords = true
		if s.PrivateKeyPath == "" {
			s.PrivateKeyPath = "~/.ssh/id_rsa_uos"
		}
		cfg.Servers["238"] = s
	}
}

// SaveConfig atomically writes the configuration to ~/.ssh/ssh-manager.json.
func SaveConfig(cfg *Config) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	return saveConfigLocked(cfg)
}

func saveConfigLocked(cfg *Config) error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	if cfg.Version == 0 {
		cfg.Version = 1
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write config tmp: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic save config: %w", err)
	}
	return nil
}

// GetServerPassword retrieves the password from server config or system Keyring fallback.
func GetServerPassword(s *Server) string {
	if s.Password != "" {
		return s.Password
	}
	pw, err := keyring.Get(keyringServiceName, s.Name)
	if err == nil && pw != "" {
		return pw
	}
	// Fallback to legacy service name
	pwLegacy, err := keyring.Get("server_manager_cli", s.Name)
	if err == nil && pwLegacy != "" {
		return pwLegacy
	}
	return ""
}

// SetServerPassword stores password in keyring.
func SetServerPassword(serverName, password string) error {
	return keyring.Set(keyringServiceName, serverName, password)
}

// GetServerKeyPassphrase retrieves key passphrase from server config or keyring.
func GetServerKeyPassphrase(s *Server) string {
	if s.KeyPassphrase != "" {
		return s.KeyPassphrase
	}
	pw, err := keyring.Get(keyringServiceName, s.Name+keyPassSuffix)
	if err == nil && pw != "" {
		return pw
	}
	pwLegacy, err := keyring.Get("server_manager_cli", s.Name+"_key_passphrase")
	if err == nil && pwLegacy != "" {
		return pwLegacy
	}
	return ""
}

// SetServerKeyPassphrase stores key passphrase in keyring.
func SetServerKeyPassphrase(serverName, pass string) error {
	return keyring.Set(keyringServiceName, serverName+keyPassSuffix, pass)
}

// SortedServers returns a slice of servers sorted by Group then Name.
func (cfg *Config) SortedServers() []Server {
	list := make([]Server, 0, len(cfg.Servers))
	for _, s := range cfg.Servers {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool {
		gi := list[i].EffectiveGroup()
		gj := list[j].EffectiveGroup()
		if gi != gj {
			return gi < gj
		}
		return list[i].Name < list[j].Name
	})
	return list
}

// ServerToSSHOptions converts a Server configuration into internal sshlib.Options,
// resolving multi-hop bastions recursively and gathering credentials.
func (cfg *Config) ServerToSSHOptions(s Server) (*sshlib.Options, error) {
	if s.Host == "" {
		return nil, fmt.Errorf("server '%s' has no host configured", s.Name)
	}
	if s.User == "" {
		return nil, fmt.Errorf("server '%s' has no user configured", s.Name)
	}

	opts := &sshlib.Options{
		Host:           s.Host,
		Port:           s.EffectivePort(),
		AutoAddHostKey: true,
		Auth: sshlib.AuthOptions{
			User: s.User,
		},
	}

	authMethod := s.AuthMethod
	if authMethod == "" {
		authMethod = "auto"
	}

	password := GetServerPassword(&s)
	keyPass := GetServerKeyPassphrase(&s)

	switch authMethod {
	case "auto":
		opts.Auth.PrivateKeyPath = s.PrivateKeyPath
		opts.Auth.KeyPassphrase = keyPass
		opts.Auth.TryAgent = true
		if password != "" {
			opts.Auth.Passwords = append(opts.Auth.Passwords, sshlib.Credential{Secret: password, Label: "configured password"})
		}
		if s.UseCommonPasswords {
			for i, pw := range sshlib.CommonPasswords {
				opts.Auth.Passwords = append(opts.Auth.Passwords, sshlib.Credential{
					Secret: pw,
					Label:  fmt.Sprintf("common password #%d", i+1),
				})
			}
		}

	case "password":
		if password != "" {
			opts.Auth.Passwords = append(opts.Auth.Passwords, sshlib.Credential{Secret: password, Label: "configured password"})
		}
		if s.UseCommonPasswords {
			for i, pw := range sshlib.CommonPasswords {
				opts.Auth.Passwords = append(opts.Auth.Passwords, sshlib.Credential{
					Secret: pw,
					Label:  fmt.Sprintf("common password #%d", i+1),
				})
			}
		}

	case "key":
		opts.Auth.PrivateKeyPath = s.PrivateKeyPath
		opts.Auth.KeyPassphrase = keyPass
	}

	// Resolve Bastion jump chain recursively
	if s.Bastion != "" {
		bastionServer, ok := cfg.Servers[s.Bastion]
		if !ok {
			return nil, fmt.Errorf("bastion server '%s' not found in configuration", s.Bastion)
		}
		bOpts, err := cfg.ServerToSSHOptions(bastionServer)
		if err != nil {
			return nil, fmt.Errorf("bastion '%s': %w", s.Bastion, err)
		}
		opts.ProxyJump = bOpts
	}

	return opts, nil
}
