package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"ssh-manager/internal/config"
	"ssh-manager/internal/sshlib"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type sshExecCommand struct {
	opts           *sshlib.Options
	tunnels        []sshlib.TunnelRule
	initialCommand string
	stdin          io.Reader
	stdout         io.Writer
	stderr         io.Writer
}

func (c *sshExecCommand) SetStdin(r io.Reader)  { c.stdin = r }
func (c *sshExecCommand) SetStdout(w io.Writer) { c.stdout = w }
func (c *sshExecCommand) SetStderr(w io.Writer) { c.stderr = w }

func (c *sshExecCommand) Run() error {
	ctx := context.Background()
	client, _, err := sshlib.Dial(ctx, c.opts)
	if err != nil {
		return err
	}
	defer client.Close()

	if len(c.tunnels) > 0 {
		tm := sshlib.NewTunnelManager(client)
		defer tm.Close()
		tm.StartAll(c.tunnels)
	}

	return client.InteractiveSession(ctx, c.initialCommand)
}

// Model represents the Bubbletea UI state.
type Model struct {
	cfg              *config.Config
	cursor           int
	filteredServers  []config.Server
	filterMode       bool
	filterInput      textinput.Model
	filterQuery      string
	formMode         FormMode
	serverForm       ServerForm
	tunnelForm       TunnelForm
	gistForm         GistForm
	testResult       *sshlib.TestResult
	tunnelMgr        *sshlib.TunnelManager
	statusMsg        string
	width            int
	height           int
	rightPanelActive bool
}

// Msg types
type testResultMsg struct {
	res *sshlib.TestResult
	err error
}

type sshExitMsg struct {
	err error
}

type copyKeyMsg struct {
	err error
}

type tunnelResultMsg struct {
	tm  *sshlib.TunnelManager
	err error
}

type gistResultMsg struct {
	msg string
	err error
}

type clearStatusMsg struct{}

// NewModel creates an initialized TUI Model.
func NewModel(cfg *config.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "Type to filter servers..."
	ti.Prompt = "/ "

	m := Model{
		cfg:         cfg,
		cursor:      0,
		filterInput: ti,
		formMode:    FormNone,
		width:       100,
		height:      30,
	}
	m.applyFilter()
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m *Model) applyFilter() {
	all := m.cfg.SortedServers()
	if m.filterQuery == "" {
		m.filteredServers = all
	} else {
		q := strings.ToLower(m.filterQuery)
		var matched []config.Server
		for _, s := range all {
			if strings.Contains(strings.ToLower(s.Name), q) ||
				strings.Contains(strings.ToLower(s.Host), q) ||
				strings.Contains(strings.ToLower(s.EffectiveGroup()), q) ||
				strings.Contains(strings.ToLower(s.User), q) {
				matched = append(matched, s)
			}
		}
		m.filteredServers = matched
	}

	if m.cursor >= len(m.filteredServers) {
		m.cursor = len(m.filteredServers) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case clearStatusMsg:
		m.statusMsg = ""
		return m, nil

	case testResultMsg:
		if msg.err != nil {
			m.testResult = &sshlib.TestResult{
				Success: false,
				Message: msg.err.Error(),
			}
			m.statusMsg = "Probe failed: " + msg.err.Error()
		} else {
			m.testResult = msg.res
			m.statusMsg = fmt.Sprintf("Probe OK (%d ms, auth: %s)", msg.res.LatencyMs, msg.res.AuthMethod)
		}
		return m, tea.Tick(4*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} })

	case copyKeyMsg:
		if msg.err != nil {
			m.statusMsg = "Copy key error: " + msg.err.Error()
		} else {
			m.statusMsg = "Successfully deployed public key to remote authorized_keys!"
		}
		return m, tea.Tick(4*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} })

	case tunnelResultMsg:
		if msg.err != nil {
			m.statusMsg = "Tunnel error: " + msg.err.Error()
		} else {
			m.tunnelMgr = msg.tm
			m.statusMsg = "Tunnels started in background successfully!"
		}
		return m, tea.Tick(4*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} })

	case gistResultMsg:
		if msg.err != nil {
			m.statusMsg = "Gist error: " + msg.err.Error()
		} else {
			m.statusMsg = msg.msg
			m.applyFilter()
		}
		return m, tea.Tick(4*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} })

	case sshExitMsg:
		if msg.err != nil {
			m.statusMsg = "SSH session ended with error: " + msg.err.Error()
		} else {
			m.statusMsg = "SSH session disconnected."
		}
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} })
	}

	// Modal form updates
	if m.formMode != FormNone {
		return m.updateForm(msg)
	}

	// Filter mode update
	if m.filterMode {
		return m.updateFilter(msg)
	}

	// Normal navigation keys
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.tunnelMgr != nil {
				_ = m.tunnelMgr.Close()
			}
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.filteredServers)-1 {
				m.cursor++
			}

		case "g", "home":
			m.cursor = 0

		case "G", "end":
			if len(m.filteredServers) > 0 {
				m.cursor = len(m.filteredServers) - 1
			}

		case "/":
			m.filterMode = true
			m.filterInput.Focus()
			return m, nil

		case "enter":
			s := m.selectedServer()
			if s == nil {
				return m, nil
			}
			opts, err := m.cfg.ServerToSSHOptions(*s)
			if err != nil {
				m.statusMsg = "Config error: " + err.Error()
				return m, nil
			}
			execCmd := &sshExecCommand{
				opts:    opts,
				tunnels: s.Tunnels,
			}
			return m, tea.Exec(execCmd, func(err error) tea.Msg {
				return sshExitMsg{err: err}
			})

		case "t": // Run tunnels only (daemon/background)
			s := m.selectedServer()
			if s == nil || len(s.Tunnels) == 0 {
				m.statusMsg = "No tunnels configured for this server. Press 'T' to add one."
				return m, nil
			}
			opts, err := m.cfg.ServerToSSHOptions(*s)
			if err != nil {
				m.statusMsg = "Config error: " + err.Error()
				return m, nil
			}
			return m, func() tea.Msg {
				client, _, err := sshlib.Dial(context.Background(), opts)
				if err != nil {
					return tunnelResultMsg{err: err}
				}
				tm := sshlib.NewTunnelManager(client)
				errs := tm.StartAll(s.Tunnels)
				if len(errs) > 0 {
					return tunnelResultMsg{tm: tm, err: errs[0]}
				}
				return tunnelResultMsg{tm: tm, err: nil}
			}

		case "K", "c": // One-click ssh-copy-id (c or Shift+K)
			s := m.selectedServer()
			if s == nil {
				return m, nil
			}
			opts, err := m.cfg.ServerToSSHOptions(*s)
			if err != nil {
				m.statusMsg = "Config error: " + err.Error()
				return m, nil
			}
			m.statusMsg = fmt.Sprintf("Deploying public key to %s...", s.Host)
			return m, func() tea.Msg {
				err := sshlib.CopyPublicKey(context.Background(), opts, s.PrivateKeyPath)
				return copyKeyMsg{err: err}
			}

		case "p": // Probe / ping server
			s := m.selectedServer()
			if s == nil {
				return m, nil
			}
			opts, err := m.cfg.ServerToSSHOptions(*s)
			if err != nil {
				m.statusMsg = "Config error: " + err.Error()
				return m, nil
			}
			m.statusMsg = fmt.Sprintf("Testing connection to %s...", s.Host)
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				res, err := sshlib.TestConnection(ctx, opts)
				return testResultMsg{res: res, err: err}
			}

		case "a": // Add server
			m.serverForm = newServerForm(nil)
			m.formMode = FormAddServer
			return m, nil

		case "e": // Edit server
			s := m.selectedServer()
			if s != nil {
				m.serverForm = newServerForm(s)
				m.formMode = FormEditServer
			}
			return m, nil

		case "d": // Delete server
			s := m.selectedServer()
			if s != nil {
				name := s.Name
				delete(m.cfg.Servers, name)
				_ = config.SaveConfig(m.cfg)
				m.statusMsg = fmt.Sprintf("Deleted server '%s'", name)
				m.applyFilter()
			}
			return m, nil

		case "T": // Add tunnel rule to selected server
			s := m.selectedServer()
			if s != nil {
				m.tunnelForm = newTunnelForm()
				m.formMode = FormAddTunnel
			}
			return m, nil

		case "s": // Gist sync
			m.gistForm = newGistForm(&m.cfg.Settings.Gist, true)
			m.formMode = FormGistSync
			return m, nil
		}
	}

	return m, nil
}

func (m Model) updateFilter(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.filterMode = false
			m.filterQuery = ""
			m.filterInput.SetValue("")
			m.applyFilter()
			return m, nil
		case "enter":
			m.filterMode = false
			m.filterQuery = m.filterInput.Value()
			m.applyFilter()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filterQuery = m.filterInput.Value()
	m.applyFilter()
	return m, cmd
}

func (m Model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.formMode = FormNone
			return m, nil
		}
	}

	switch m.formMode {
	case FormAddServer, FormEditServer:
		done, s := m.serverForm.Update(msg)
		if done && s != nil {
			if m.formMode == FormEditServer && m.serverForm.oldName != "" && m.serverForm.oldName != s.Name {
				delete(m.cfg.Servers, m.serverForm.oldName)
			}
			m.cfg.Servers[s.Name] = *s
			_ = config.SaveConfig(m.cfg)
			m.formMode = FormNone
			m.statusMsg = fmt.Sprintf("Saved server '%s'", s.Name)
			m.applyFilter()
		}
		return m, nil

	case FormAddTunnel:
		done, rule := m.tunnelForm.Update(msg)
		if done && rule != nil {
			s := m.selectedServer()
			if s != nil {
				s.Tunnels = append(s.Tunnels, *rule)
				m.cfg.Servers[s.Name] = *s
				_ = config.SaveConfig(m.cfg)
				m.statusMsg = fmt.Sprintf("Added tunnel to '%s'", s.Name)
			}
			m.formMode = FormNone
			m.applyFilter()
		}
		return m, nil

	case FormGistSync:
		done, token, gistID, masterPass := m.gistForm.Update(msg)
		if done {
			m.cfg.Settings.Gist.Token = token
			m.cfg.Settings.Gist.GistID = gistID
			if masterPass != "" {
				m.cfg.Settings.Gist.Encrypted = true
			}
			_ = config.SaveConfig(m.cfg)
			m.formMode = FormNone
			m.statusMsg = "Syncing with GitHub Secret Gist..."

			return m, func() tea.Msg {
				id, err := config.PushToGist(m.cfg, masterPass)
				if err != nil {
					return gistResultMsg{err: err}
				}
				return gistResultMsg{msg: fmt.Sprintf("Gist synced successfully! (ID: %s)", id)}
			}
		}
		return m, nil
	}

	return m, nil
}

func (m Model) View() string {
	switch m.formMode {
	case FormAddServer, FormEditServer:
		return m.serverForm.View()
	case FormAddTunnel:
		return m.tunnelForm.View()
	case FormGistSync:
		return m.gistForm.View()
	}
	return renderLayout(&m)
}
