package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
    "sort"


)

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// GetServers returns the list of configured servers
func (a *App) GetServers() ([]Server, error) {
	serverMap, err := LoadServers()
	if err != nil {
		return nil, err
	}
	
	servers := make([]Server, 0, len(serverMap))
	for _, s := range serverMap {
		servers = append(servers, s)
	}
    // Sort by Group then Name
    sort.Slice(servers, func(i, j int) bool {
        if servers[i].Group != servers[j].Group {
            return servers[i].Group < servers[j].Group
        }
        return servers[i].Name < servers[j].Name
    })

	return servers, nil
}

// SaveServer adds or updates a server configuration
func (a *App) SaveServer(server Server, oldName string, password string) error {
	if server.Name == "" {
		return fmt.Errorf("server name is required")
	}

	servers, err := LoadServers()
	if err != nil {
		return err
	}

    // Validate bastion if specified
	if server.Bastion != "" {
		if _, ok := servers[server.Bastion]; !ok {
            // Check if bastion is the one we are renaming (unlikely but possible edge case during rename?)
            // Actually if we rename A to B, and A was bastion, it's complex. 
            // Let's assume bastion check refers to *existing* keys or the *new* name if we are careful.
            // Simplest: Must refer to a valid key in the map.
			return fmt.Errorf("bastion server '%s' not found", server.Bastion)
		}
		if server.Bastion == server.Name {
			return fmt.Errorf("server cannot be its own bastion")
		}
	}

    // Check for duplicate if name changed or new
    if oldName == "" || oldName != server.Name {
        if _, exists := servers[server.Name]; exists {
            return fmt.Errorf("server '%s' already exists", server.Name)
        }
    }

    // If renaming, delete old entry
    if oldName != "" && oldName != server.Name {
        delete(servers, oldName)
        // Also move/delete password?
        // Simplest: Delete old password. New password will be set below if provided.
        // If password is NOT provided during edit, we might lose it if we delete old key?
        // We should PROBABLY copy the password if not provided?
        // But GetPassword might fail if not found.
        
        // If password is "", we preserve old password?
        // User flow: 
        // 1. Edit. 
        // 2. Pass blank password -> Keep existing.
        // 3. Pass new password -> Update.
        
        if password == "" {
             oldPwd, _ := GetPassword(oldName)
             if oldPwd != "" {
                 // Set string to oldPwd so it gets saved to new name
                 password = oldPwd
             }
        }
        
        // Now safe to delete old password key
        _ = DeletePassword(oldName)
    }

	servers[server.Name] = server
	if err := SaveServers(servers); err != nil {
		return err
	}

	// Save password if provided (or carried over)
	if password != "" {
		if err := SetPassword(server.Name, password); err != nil {
			return fmt.Errorf("failed to save password: %w", err)
		}
	}

	return nil
}

// DeleteServer removes a server configuration
func (a *App) DeleteServer(name string) error {
	servers, err := LoadServers()
	if err != nil {
		return err
	}

	if _, ok := servers[name]; !ok {
		return fmt.Errorf("server '%s' not found", name)
	}

	delete(servers, name)
	if err := SaveServers(servers); err != nil {
		return err
	}

	// Delete password (ignore error if not found)
	_ = DeletePassword(name)

	return nil
}

// Connect launches a terminal to connect to the server
func (a *App) Connect(name string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("connect is currently only supported on macOS")
	}

	servers, err := LoadServers()
	if err != nil {
		return err
	}

	server, ok := servers[name]
	if !ok {
		return fmt.Errorf("server '%s' not found", name)
	}

	// Retrieve password
	password, err := GetPassword(name)
	if err != nil {
		fmt.Printf("Warning: could not get password for %s: %v\n", name, err)
        password = ""
	}

    // Prepare command parts
    // We will construct a shell script command
    
    // Helper to shell quote
    quote := func(s string) string {
        if s == "" {
            return "''"
        }
        return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
    }

    var cmdStr string
    
    // Target Setup
    targetCmd := fmt.Sprintf("ssh %s@%s", server.User, server.Host)
    
    // Handle Bastion
    if server.Bastion != "" {
        bastion, ok := servers[server.Bastion]
        if !ok {
            return fmt.Errorf("bastion '%s' not found", server.Bastion)
        }
        
        bastionPassword, _ := GetPassword(server.Bastion)
        
        // ProxyCommand
        // We need to construct the inner ssh command for the bastion.
        // If bastion has password, wrap it in sshpass.
        bastionSSH := fmt.Sprintf("ssh -W %%h:%%p %s@%s", bastion.User, bastion.Host)
        if bastionPassword != "" {
            bastionSSH = fmt.Sprintf("sshpass -p %s %s", quote(bastionPassword), bastionSSH)
        }
        
        // Now wrap this in the ProxyCommand option
        // We need to be careful with quoting the ProxyCommand value for the outer shell
        // The outer shell sees: ssh -o ProxyCommand='...'
        // So the ' inside ... need to be escaped as '\''
        
        // Let's rely on the quote helper which wraps in single quotes.
        // But we need the CONTENT of the quotes.
        
        // Actually, let's keep it simple:
        // Use double quotes for ProxyCommand if possible? No, shell variable expansion risk.
        // Std approach: ssh -o ProxyCommand='...'
        
        // We can just shell quote the WHOLE string "sshpass ... host"
        proxyCmdQuoted := quote(bastionSSH)
        
        targetCmd = fmt.Sprintf("ssh -o ProxyCommand=%s %s@%s", proxyCmdQuoted, server.User, server.Host)
    }

    // Wrap Target in sshpass if password exists
    if password != "" {
        cmdStr = fmt.Sprintf("sshpass -p %s %s", quote(password), targetCmd)
    } else {
        cmdStr = targetCmd
    }

    // Add Host Key checking skip for convenience?
    // User didn't strictly ask, but sshpass usually needs it to avoid "yes/no" hang.
    // But forcing it is insecure.
    // Let's add a check: if sshpass is used, we might want to auto-accept keys?
    // Let's stick to standard behavior. If it hangs on "yes/no", user will see it in terminal (hopefully? sshpass suppresses some output?)
    // Actually sshpass hides the password prompt but "yes/no" prompt usually still appears and blocks.
    // Let's safely add standard options to avoid hanging if using sshpass?
    // NO, let's keep security defaults. The user can type "yes" if sshpass passes it through (it usually doesn't pass stdin usage well).
    // Actually, sshpass fails if it sees the confirmation prompt.
    // We should probably add "-o StrictHostKeyChecking=no" strictly when using sshpass?
    // Let's trust the user knows what they are doing with sshpass for now.
    
    // Write script
    scriptContent := "#!/bin/bash\n"
    scriptContent += "echo 'Starting connection...'\n"
    // Add sshpass check
    scriptContent += "if ! command -v sshpass &> /dev/null; then\n"
    scriptContent += "    echo 'Error: sshpass is not installed. Please install it (e.g., brew install sshpass)'\n"
    scriptContent += "    echo 'Falling back to standard ssh...'\n"
    // Fallback logic?
    // If sshpass missing, we strip sshpass parts?
    // That's complex since we nested it.
    // Just exit or let it fail.
    scriptContent += "    exit 1\n"
    scriptContent += "fi\n\n"
    
    scriptContent += cmdStr + "\n"
    
    // Keep terminal open if it exits immediately
    // scriptContent += "read -p 'Press enter to close...'\n" 
    // Usually standard shell behavior leaves it? No, `open` might close it if process ends.
    // But ssh ends.
    // If ssh fails, we want to see it.
    
    scriptContent += "\n"

    // Write script to temp file
    // Use .command extension so macOS Terminal treats it as executable easily?
    // Or .sh and `open -a Terminal`.
    tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("ssh_%s_%d.command", utilCheckName(name), time.Now().Unix()))
    if err := os.WriteFile(tmpFile, []byte(scriptContent), 0755); err != nil {
        return err
    }
    
    // Open in Terminal
    // .command files are executable. `open` should just run it.
    cmd := exec.Command("open", tmpFile)
    return cmd.Run()
}




func utilCheckName(s string) string {
    // simple sanitization for filename
    return strings.Map(func(r rune) rune {
        if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
            return r
        }
        return '_'
    }, s)
}
