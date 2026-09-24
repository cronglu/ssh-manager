package config

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

type gistFile struct {
	Content string `json:"content"`
}

type gistPayload struct {
	Description string              `json:"description"`
	Public      bool                `json:"public"`
	Files       map[string]gistFile `json:"files"`
}

type gistResponse struct {
	ID    string              `json:"id"`
	Files map[string]gistFile `json:"files"`
}

const (
	gistFileName = "ssh-manager.json"
	pbkdf2Iter   = 100000
)

// EncryptedPayload envelope for AES-GCM encrypted gist backups
type EncryptedPayload struct {
	Version    int    `json:"version"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// EncryptConfigData encrypts raw JSON config using AES-256-GCM and PBKDF2 derived key.
func EncryptConfigData(data []byte, masterPassword string) (string, error) {
	if masterPassword == "" {
		return "", errors.New("master password cannot be empty")
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}

	key := pbkdf2.Key([]byte(masterPassword), salt, pbkdf2Iter, 32, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, data, nil)

	env := EncryptedPayload{
		Version:    1,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}

	envBytes, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return string(envBytes), nil
}

// DecryptConfigData decrypts an AES-256-GCM encrypted envelope using masterPassword.
func DecryptConfigData(content string, masterPassword string) ([]byte, error) {
	if masterPassword == "" {
		return nil, errors.New("master password cannot be empty")
	}

	var env EncryptedPayload
	if err := json.Unmarshal([]byte(content), &env); err != nil {
		return nil, fmt.Errorf("invalid encrypted payload: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return nil, errors.New("invalid salt encoding")
	}

	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, errors.New("invalid nonce encoding")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, errors.New("invalid ciphertext encoding")
	}

	key := pbkdf2.Key([]byte(masterPassword), salt, pbkdf2Iter, 32, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("failed to decrypt (incorrect master password or corrupted data)")
	}

	return plaintext, nil
}

// PushToGist uploads the current configuration to GitHub Secret Gist.
func PushToGist(cfg *Config, masterPassword string) (string, error) {
	token := cfg.Settings.Gist.Token
	if token == "" {
		return "", errors.New("GitHub Personal Access Token (PAT) is required")
	}

	rawJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}

	content := string(rawJSON)
	if cfg.Settings.Gist.Encrypted {
		if masterPassword == "" {
			return "", errors.New("master password required for encrypted Gist sync")
		}
		content, err = EncryptConfigData(rawJSON, masterPassword)
		if err != nil {
			return "", fmt.Errorf("encryption failed: %w", err)
		}
	}

	payload := gistPayload{
		Description: "SSH Manager Config Backup (Secret Gist)",
		Public:      false,
		Files: map[string]gistFile{
			gistFileName: {Content: content},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	var req *http.Request
	client := &http.Client{Timeout: 15 * time.Second}

	if cfg.Settings.Gist.GistID != "" {
		// Update existing gist
		url := fmt.Sprintf("https://api.github.com/gists/%s", cfg.Settings.Gist.GistID)
		req, err = http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
	} else {
		// Create new secret gist
		url := "https://api.github.com/gists"
		req, err = http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	}
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ssh-manager-cli")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var gistResp gistResponse
	if err := json.NewDecoder(resp.Body).Decode(&gistResp); err != nil {
		return "", fmt.Errorf("decode GitHub response: %w", err)
	}

	cfg.Settings.Gist.GistID = gistResp.ID
	_ = SaveConfig(cfg)
	return gistResp.ID, nil
}

// PullFromGist downloads and restores configuration from GitHub Secret Gist.
func PullFromGist(token, gistID, masterPassword string) (*Config, error) {
	if token == "" {
		return nil, errors.New("GitHub Personal Access Token is required")
	}
	if gistID == "" {
		return nil, errors.New("Gist ID is required")
	}

	url := fmt.Sprintf("https://api.github.com/gists/%s", gistID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ssh-manager-cli")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var gistResp gistResponse
	if err := json.NewDecoder(resp.Body).Decode(&gistResp); err != nil {
		return nil, fmt.Errorf("decode GitHub response: %w", err)
	}

	file, ok := gistResp.Files[gistFileName]
	if !ok {
		return nil, fmt.Errorf("gist does not contain file '%s'", gistFileName)
	}

	rawBytes := []byte(file.Content)
	// Check if content is encrypted
	var env EncryptedPayload
	if err := json.Unmarshal(rawBytes, &env); err == nil && env.Ciphertext != "" && env.Salt != "" {
		decrypted, err := DecryptConfigData(file.Content, masterPassword)
		if err != nil {
			return nil, err
		}
		rawBytes = decrypted
	}

	var newCfg Config
	if err := json.Unmarshal(rawBytes, &newCfg); err != nil {
		return nil, fmt.Errorf("parse pulled config: %w", err)
	}

	// Update local gist settings
	newCfg.Settings.Gist.Token = token
	newCfg.Settings.Gist.GistID = gistID
	if err := SaveConfig(&newCfg); err != nil {
		return nil, fmt.Errorf("save restored config: %w", err)
	}

	return &newCfg, nil
}
