package tui

import (
	"fmt"
	"strings"

	"ssh-manager/internal/config"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type FzfModel struct {
	cfg      *config.Config
	input    textinput.Model
	filtered []config.Server
	cursor   int
	chosen   *config.Server
	canceled bool
	width    int
	height   int
}

func NewFzfModel(cfg *config.Config, initialQuery string) FzfModel {
	ti := textinput.New()
	ti.Placeholder = "Search servers..."
	ti.Prompt = "ssh > "
	ti.SetValue(initialQuery)
	ti.Focus()

	m := FzfModel{
		cfg:    cfg,
		input:  ti,
		cursor: 0,
		width:  80,
		height: 20,
	}
	m.applyFilter()
	return m
}

func (m *FzfModel) applyFilter() {
	all := m.cfg.SortedServers()
	q := strings.ToLower(m.input.Value())
	if q == "" {
		m.filtered = all
	} else {
		var matched []config.Server
		for _, s := range all {
			if strings.Contains(strings.ToLower(s.Name), q) ||
				strings.Contains(strings.ToLower(s.Host), q) ||
				strings.Contains(strings.ToLower(s.EffectiveGroup()), q) ||
				strings.Contains(strings.ToLower(s.User), q) {
				matched = append(matched, s)
			}
		}
		m.filtered = matched
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m FzfModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m FzfModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.canceled = true
			return m, tea.Quit

		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil

		case "enter":
			if len(m.filtered) > 0 && m.cursor >= 0 && m.cursor < len(m.filtered) {
				sel := m.filtered[m.cursor]
				m.chosen = &sel
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.applyFilter()
	return m, cmd
}

func (m FzfModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Quick Connect (FZF Mode)"))
	b.WriteString("\n")
	b.WriteString(m.input.View())
	b.WriteString("\n\n")

	maxItems := 12
	start := 0
	if m.cursor >= maxItems {
		start = m.cursor - maxItems + 1
	}
	end := start + maxItems
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	if len(m.filtered) == 0 {
		b.WriteString(styleFooter.Render("  No matching servers found.\n"))
	} else {
		for i := start; i < end; i++ {
			s := m.filtered[i]
			cursor := "  "
			if i == m.cursor {
				cursor = "▶ "
			}

			badges := ""
			if s.Bastion != "" {
				badges += " [via " + s.Bastion + "]"
			}
			if len(s.Tunnels) > 0 {
				badges += fmt.Sprintf(" [%d tunnels]", len(s.Tunnels))
			}

			line := fmt.Sprintf("%s%-18s %s@%s:%d %s", cursor, s.Name, s.User, s.Host, s.EffectivePort(), badges)
			if i == m.cursor {
				b.WriteString(styleSelectedItem.Render(line) + "\n")
			} else {
				b.WriteString(styleNormalItem.Render(line) + "\n")
			}
		}
	}

	b.WriteString("\n" + styleFooter.Render("[↑/↓] Select | [Enter] Connect | [Esc] Cancel"))
	return b.String()
}

// RunFzfQuickSelect runs the fast fzf picker and returns the selected server (or nil if cancelled).
func RunFzfQuickSelect(cfg *config.Config, query string) (*config.Server, error) {
	// If query exact matches a server name, connect directly!
	if s, ok := cfg.Servers[query]; ok {
		return &s, nil
	}

	// If query matches only 1 server, return immediately
	var matches []config.Server
	q := strings.ToLower(query)
	for _, s := range cfg.Servers {
		if strings.EqualFold(s.Name, q) || strings.EqualFold(s.Host, q) {
			matches = append(matches, s)
		}
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}

	m := NewFzfModel(cfg, query)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	fm := finalModel.(FzfModel)
	if fm.canceled {
		return nil, nil
	}
	return fm.chosen, nil
}
