# SSH Manager (`sshx`) - Pure Go TUI & CLI Edition

A modern, blazing-fast, single-binary SSH and Port Forwarding manager written in pure Go.

Built to completely replace `sshpass`, eliminating the hassle of manually typing passwords, IPs, ports, and jump host parameters. Delivers Tabby/K9s-level host and tunnel orchestration directly in your terminal.

---

## ✨ Key Features

- **Zero `sshpass` & Zero Secret Leakage**:
  - Pure Go PTY terminal session takeover (`golang.org/x/crypto/ssh` + `golang.org/x/term` in raw mode).
  - Passwords and keys are negotiated purely in memory. No temporary askpass shell scripts in `/tmp` and no secrets visible in process argument lists.
- **Dual-Mode Interaction**:
  - **Multi-Panel TUI (Bubbletea + Lipgloss)**: Lazygit/K9s-style interactive dashboard with grouped host trees, live search filter (`/`), jump chain visualization, active tunnel monitors, and modal forms for adding/editing servers.
  - **Blazing Fast CLI & FZF**: Instant connect via `sshx <alias>` or fuzzy picker `sshx fzf`.
- **All-Round Port Forwarding & Tunnels (`-L`, `-R`, `-D`)**:
  - **`-L` (Local Port Forwarding)**: Map remote internal services (e.g. VNC `5900`, NewAPI `3000`) to local ports.
  - **`-D` (Dynamic SOCKS5 Proxy)**: Built-in RFC 1928 SOCKS5 proxy server (e.g. `127.0.0.1:1080`) for full intranet subnet roaming (such as `10.0.0.0/8`).
  - **`-R` (Remote Port Forwarding)**: Reverse tunnels.
  - Run tunnels concurrently with interactive SSH shells, or run standalone in the background/foreground.
- **Arbitrary Multi-Hop Bastion (Jump Host) Support**:
  - RFC 4254 `direct-tcpip` recursive channel chaining (`Local ➔ Bastion 1 ➔ Bastion 2 ➔ Target`).
- **One-Click Public Key Deployment (`ssh-copy-id`)**:
  - Press `c` in TUI or run `sshx copy-id <alias>` to automatically deploy your local `~/.ssh/id_*.pub` to the remote server's `~/.ssh/authorized_keys`, upgrading password-based hosts to passwordless key auth.
- **Standalone Configuration**:
  - Dedicated configuration file at `~/.ssh/ssh-manager.json`. Does not modify or pollute your system `~/.ssh/config`.
  - Automatically migrates existing entries from legacy `~/.server_manager.json`.
- **Cloud Backup with GitHub Secret Gist**:
  - Backup and restore configurations across machines via GitHub PAT.
  - Supports client-side **AES-256-GCM** encryption with a Master Password before uploading to the cloud.

---

## 🚀 Installation & Build

Requires Go 1.20+:

```bash
# Clone the repository
git clone https://github.com/cronglu/ssh-manager.git
cd ssh-manager

# Build single binary
go build -o sshx .

# Optional: install to your PATH
sudo cp sshx /usr/local/bin/
```

---

## 📖 Usage

### 1. Multi-Panel TUI Mode
Simply run without arguments:
```bash
sshx
```

#### TUI Keyboard Shortcuts:
| Key | Action |
| :--- | :--- |
| `↑` / `k`, `↓` / `j` | Navigate server list |
| `Enter` | Connect to interactive SSH shell (auto-starts configured tunnels) |
| `t` | Run tunnels only (background/foreground daemon mode) |
| `c` (or `Shift+K`) | One-click copy local public key (`ssh-copy-id`) |
| `p` | Probe connection latency, auth method, and server fingerprint |
| `a` | Add new server (modal form) |
| `e` | Edit selected server |
| `d` | Delete selected server |
| `T` | Add port forwarding / tunnel rule to selected server |
| `s` | Backup / restore configuration to GitHub Secret Gist |
| `/` | Instant fuzzy search filter |
| `q` / `Esc` | Quit / Close modal |

---

### 2. Fast CLI & FZF Mode

```bash
# Quick connect (matches name or launches fzf if multiple matches)
sshx 238
sshx 197

# Direct interactive connection
sshx connect 238

# Test / probe latency and credentials
sshx test 238
sshx test 197

# Execute remote command directly
sshx exec 238 "hostname && uname -a"

# Start configured port forwardings (-L, -R, -D) in foreground
sshx tunnel 238

# Deploy public key to remote host
sshx copy-id 238

# List all configured servers
sshx list

# Fuzzy search picker
sshx fzf

# Backup or restore configuration to GitHub Secret Gist
sshx sync push
sshx sync pull
```

---

## 🛠 Configuration Schema (`~/.ssh/ssh-manager.json`)

```json
{
  "version": 1,
  "settings": {
    "defaultPrivateKey": "~/.ssh/id_rsa_uos"
  },
  "servers": {
    "238": {
      "name": "238",
      "group": "wh_arm",
      "host": "10.0.35.238",
      "port": 22,
      "user": "mode",
      "authMethod": "auto",
      "password": "Uniontech@2023",
      "useCommonPasswords": true,
      "tunnels": [
        {
          "type": "local",
          "local": "127.0.0.1:5900",
          "remote": "10.20.13.115:5900",
          "description": "VNC Desktop"
        },
        {
          "type": "dynamic",
          "local": "127.0.0.1:1080",
          "description": "SOCKS5 Proxy for 10.0.0.0/8"
        }
      ]
    },
    "197": {
      "name": "197",
      "group": "wh_arm",
      "host": "10.0.35.197",
      "port": 22,
      "user": "mode",
      "authMethod": "auto",
      "privateKeyPath": "~/.ssh/id_rsa_uos"
    }
  }
}
```

---

## 📄 License

MIT License. See [LICENSE](file:///Users/crl/www/self-www/ai/ssh-manager/LICENSE) for details.
