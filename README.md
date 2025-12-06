# SSH Manager (Wails Edition)

A desktop application for managing SSH server connections, ported from the original Python script. Built with Wails (Go + Vue 3).

## Features

- **Server Management**: Add, list, and delete server configurations.
- **Grouping**: Organize servers by groups (e.g., Production, Staging).
- **Secure Password Storage**: Passwords are stored securely in the system Keychain/Keyring (via `github.com/zalando/go-keyring`).
- **Bastion/Jump Host Support**: Connect to servers via a bastion host seamlessly.
- **One-Click Connect**: Opens your system Terminal and automatically logs you in.
- **Data Compatibility**: Reads and writes to `~/.server_manager.json`, compatible with the original Python CLI tool.

## Prerequisites

- **Go**: v1.20+
- **Node.js**: v16+
- **Wails CLI**: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **sshpass**: Required for password automation. Install via `brew install sshpass`.
- **macOS**: Currently optimized for macOS (uses `open` command).

## Build & Run

### Development

To run in development mode with hot-reloading:

```bash
wails dev
```

### Production Build

To build the application for distribution:

```bash
wails build
```

The output will be in `build/bin/ssh-manager-wails.app`.

## Usage

1.  **Add Server**: Click the "Add Server" button.

    - **Name**: Unique identifier.
    - **Group**: Category for organization.
    - **Host**: IP address or hostname.
    - **User**: SSH username.
    - **Password**: Optional. If provided, it's saved in the Keychain.
    - **Bastion**: Select an existing server to use as a jump host.

2.  **Connect**: Click the "Connect" button next to a server.

    - A new Terminal window will open.
    - If a password is stored, it will attempt to auto-enter it.
    - If no password is stored, you will be prompted in the terminal.

3.  **Delete**: Remove a server configuration and its stored password.

## Project Structure

- `main.go`, `app.go`: Go backend logic.
- `models.go`, `config.go`: Data structures and configuration management.
- `keyring_utils.go`: Keyring integration.
- `frontend/`: Vue 3 frontend application.
- `frontend/wailsjs/`: Auto-generated bindings.
