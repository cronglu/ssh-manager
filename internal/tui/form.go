package tui

import (
	"fmt"
	"strconv"
	"strings"

	"ssh-manager/internal/config"
	"ssh-manager/internal/sshlib"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type FormMode int

const (
	FormNone FormMode = iota
	FormAddServer
	FormEditServer
	FormAddTunnel
	FormGistSync
)

type ServerForm struct {
	inputs   []textinput.Model
	focusIdx int
	oldName  string
	commonPw bool
	isEdit   bool
}

func newServerForm(s *config.Server) ServerForm {
	labels := []string{
		"Server Name",
		"Group",
		"Host / IP",
		"Port",
		"User",
		"Password",
		"Auth Method (auto/password/key)",
		"Private Key Path",
		"Bastion Jump Host",
		"Notes",
	}

	defaults := make([]string, len(labels))
	commonPw := true
	oldName := ""

	if s != nil {
		oldName = s.Name
		defaults[0] = s.Name
		defaults[1] = s.Group
		defaults[2] = s.Host
		defaults[3] = strconv.Itoa(s.EffectivePort())
		defaults[4] = s.User
		defaults[5] = s.Password
		defaults[6] = s.AuthMethod
		defaults[7] = s.PrivateKeyPath
		defaults[8] = s.Bastion
		defaults[9] = s.Notes
		commonPw = s.UseCommonPasswords
	} else {
		defaults[3] = "22"
		defaults[6] = "auto"
		defaults[7] = "~/.ssh/id_ed25519"
	}

	inputs := make([]textinput.Model, len(labels))
	for i := range labels {
		t := textinput.New()
		t.Prompt = labels[i] + ": "
		t.SetValue(defaults[i])
		if i == 5 {
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		}
		inputs[i] = t
	}
	inputs[0].Focus()

	return ServerForm{
		inputs:   inputs,
		focusIdx: 0,
		oldName:  oldName,
		commonPw: commonPw,
		isEdit:   s != nil,
	}
}

func (f *ServerForm) Update(msg tea.Msg) (bool, *config.Server) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "up", "down":
			if msg.String() == "up" || msg.String() == "shift+tab" {
				f.focusIdx--
			} else {
				f.focusIdx++
			}

			if f.focusIdx < 0 {
				f.focusIdx = len(f.inputs) - 1
			} else if f.focusIdx >= len(f.inputs) {
				f.focusIdx = 0
			}

			for i := range f.inputs {
				if i == f.focusIdx {
					f.inputs[i].Focus()
				} else {
					f.inputs[i].Blur()
				}
			}
			return false, nil

		case "ctrl+t": // toggle common passwords
			f.commonPw = !f.commonPw
			return false, nil

		case "enter":
			if f.focusIdx == len(f.inputs)-1 {
				// Submit form
				port, _ := strconv.Atoi(strings.TrimSpace(f.inputs[3].Value()))
				if port <= 0 {
					port = 22
				}
				s := &config.Server{
					Name:               strings.TrimSpace(f.inputs[0].Value()),
					Group:              strings.TrimSpace(f.inputs[1].Value()),
					Host:               strings.TrimSpace(f.inputs[2].Value()),
					Port:               port,
					User:               strings.TrimSpace(f.inputs[4].Value()),
					Password:           f.inputs[5].Value(),
					AuthMethod:         strings.TrimSpace(f.inputs[6].Value()),
					PrivateKeyPath:     strings.TrimSpace(f.inputs[7].Value()),
					Bastion:            strings.TrimSpace(f.inputs[8].Value()),
					Notes:              strings.TrimSpace(f.inputs[9].Value()),
					UseCommonPasswords: f.commonPw,
				}
				return true, s
			}
			// Move to next field
			f.focusIdx++
			if f.focusIdx >= len(f.inputs) {
				f.focusIdx = 0
			}
			for i := range f.inputs {
				if i == f.focusIdx {
					f.inputs[i].Focus()
				} else {
					f.inputs[i].Blur()
				}
			}
			return false, nil
		}
	}

	var cmd tea.Cmd
	f.inputs[f.focusIdx], cmd = f.inputs[f.focusIdx].Update(msg)
	_ = cmd
	return false, nil
}

func (f *ServerForm) View() string {
	title := "Add New Server"
	if f.isEdit {
		title = "Edit Server: " + f.oldName
	}

	var b strings.Builder
	b.WriteString(styleTitle.Render("=== " + title + " ==="))
	b.WriteString("\n\n")

	for i := range f.inputs {
		b.WriteString(f.inputs[i].View())
		b.WriteString("\n")
	}

	commonPwText := "[x] Enabled"
	if !f.commonPw {
		commonPwText = "[ ] Disabled"
	}
	b.WriteString(fmt.Sprintf("\nCommon Passwords Fallback (Ctrl+T): %s\n", styleSubtitle.Render(commonPwText)))
	b.WriteString(styleFooter.Render("\n[Tab/Shift+Tab] Navigate | [Enter on last field] Save | [Esc] Cancel\n"))

	return styleModal.Render(b.String())
}

// TunnelForm for adding/editing tunnels
type TunnelForm struct {
	inputs   []textinput.Model
	focusIdx int
}

func newTunnelForm() TunnelForm {
	labels := []string{
		"Tunnel Type (local / remote / dynamic)",
		"Local Address (e.g. 127.0.0.1:5900)",
		"Remote Address (e.g. 10.20.13.115:5900, leave empty for dynamic)",
		"Description",
	}

	inputs := make([]textinput.Model, len(labels))
	for i := range labels {
		t := textinput.New()
		t.Prompt = labels[i] + ": "
		inputs[i] = t
	}
	inputs[0].SetValue("local")
	inputs[1].SetValue("127.0.0.1:5900")
	inputs[0].Focus()

	return TunnelForm{
		inputs:   inputs,
		focusIdx: 0,
	}
}

func (f *TunnelForm) Update(msg tea.Msg) (bool, *sshlib.TunnelRule) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "up", "down":
			if msg.String() == "up" || msg.String() == "shift+tab" {
				f.focusIdx--
			} else {
				f.focusIdx++
			}
			if f.focusIdx < 0 {
				f.focusIdx = len(f.inputs) - 1
			} else if f.focusIdx >= len(f.inputs) {
				f.focusIdx = 0
			}
			for i := range f.inputs {
				if i == f.focusIdx {
					f.inputs[i].Focus()
				} else {
					f.inputs[i].Blur()
				}
			}
			return false, nil

		case "enter":
			if f.focusIdx == len(f.inputs)-1 {
				r := &sshlib.TunnelRule{
					Type:        strings.TrimSpace(f.inputs[0].Value()),
					Local:       strings.TrimSpace(f.inputs[1].Value()),
					Remote:      strings.TrimSpace(f.inputs[2].Value()),
					Description: strings.TrimSpace(f.inputs[3].Value()),
					AutoStart:   true,
				}
				r.Normalize()
				return true, r
			}
			f.focusIdx++
			for i := range f.inputs {
				if i == f.focusIdx {
					f.inputs[i].Focus()
				} else {
					f.inputs[i].Blur()
				}
			}
			return false, nil
		}
	}

	var cmd tea.Cmd
	f.inputs[f.focusIdx], cmd = f.inputs[f.focusIdx].Update(msg)
	_ = cmd
	return false, nil
}

func (f *TunnelForm) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("=== Add Port Forward / Tunnel Rule ==="))
	b.WriteString("\n\n")

	for i := range f.inputs {
		b.WriteString(f.inputs[i].View())
		b.WriteString("\n")
	}

	b.WriteString(styleFooter.Render("\n[Tab] Next Field | [Enter on last] Save | [Esc] Cancel\n"))
	return styleModal.Render(b.String())
}

// GistForm for GitHub Gist configuration and backup/restore
type GistForm struct {
	inputs   []textinput.Model
	focusIdx int
	isPush   bool
}

func newGistForm(g *config.GistConfig, isPush bool) GistForm {
	labels := []string{
		"GitHub Token (PAT)",
		"Gist ID (leave empty to create new)",
		"Master Password (for AES-256-GCM encryption)",
	}

	inputs := make([]textinput.Model, len(labels))
	for i := range labels {
		t := textinput.New()
		t.Prompt = labels[i] + ": "
		inputs[i] = t
	}

	if g != nil {
		inputs[0].SetValue(g.Token)
		inputs[1].SetValue(g.GistID)
	}
	inputs[2].EchoMode = textinput.EchoPassword
	inputs[2].EchoCharacter = '•'
	inputs[0].Focus()

	return GistForm{
		inputs:   inputs,
		focusIdx: 0,
		isPush:   isPush,
	}
}

func (f *GistForm) Update(msg tea.Msg) (bool, string, string, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "up", "down":
			if msg.String() == "up" || msg.String() == "shift+tab" {
				f.focusIdx--
			} else {
				f.focusIdx++
			}
			if f.focusIdx < 0 {
				f.focusIdx = len(f.inputs) - 1
			} else if f.focusIdx >= len(f.inputs) {
				f.focusIdx = 0
			}
			for i := range f.inputs {
				if i == f.focusIdx {
					f.inputs[i].Focus()
				} else {
					f.inputs[i].Blur()
				}
			}
			return false, "", "", ""

		case "enter":
			if f.focusIdx == len(f.inputs)-1 {
				token := strings.TrimSpace(f.inputs[0].Value())
				gistID := strings.TrimSpace(f.inputs[1].Value())
				masterPass := f.inputs[2].Value()
				return true, token, gistID, masterPass
			}
			f.focusIdx++
			for i := range f.inputs {
				if i == f.focusIdx {
					f.inputs[i].Focus()
				} else {
					f.inputs[i].Blur()
				}
			}
			return false, "", "", ""
		}
	}

	var cmd tea.Cmd
	f.inputs[f.focusIdx], cmd = f.inputs[f.focusIdx].Update(msg)
	_ = cmd
	return false, "", "", ""
}

func (f *GistForm) View() string {
	action := "Upload to GitHub Secret Gist"
	if !f.isPush {
		action = "Restore from GitHub Secret Gist"
	}

	var b strings.Builder
	b.WriteString(styleTitle.Render("=== " + action + " ==="))
	b.WriteString("\n\n")

	for i := range f.inputs {
		b.WriteString(f.inputs[i].View())
		b.WriteString("\n")
	}

	b.WriteString(styleFooter.Render("\n[Tab] Next Field | [Enter on last] Execute | [Esc] Cancel\n"))
	return styleModal.Render(b.String())
}
