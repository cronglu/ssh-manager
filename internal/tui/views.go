package tui

import (
	"fmt"
	"strings"

	"ssh-manager/internal/config"

	"github.com/charmbracelet/lipgloss"
)

func renderLayout(m *Model) string {
	termWidth := m.width
	termHeight := m.height
	if termWidth < 80 {
		termWidth = 80
	}
	if termHeight < 24 {
		termHeight = 24
	}

	headerView := renderHeader(m, termWidth)
	footerView := renderFooter(m, termWidth)

	bodyHeight := termHeight - lipgloss.Height(headerView) - lipgloss.Height(footerView) - 1
	if bodyHeight < 12 {
		bodyHeight = 12
	}

	leftWidth := termWidth * 38 / 100
	if leftWidth < 28 {
		leftWidth = 28
	}
	rightWidth := termWidth - leftWidth - 3

	leftPanel := renderLeftPanel(m, leftWidth, bodyHeight)
	rightPanel := renderRightPanel(m, rightWidth, bodyHeight)

	bodyView := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
	return lipgloss.JoinVertical(lipgloss.Left, headerView, bodyView, footerView)
}

func renderHeader(m *Model, width int) string {
	title := styleTitle.Render("SSH Manager (TUI Edition)")
	info := styleSubtitle.Render(fmt.Sprintf("%d Servers", len(m.cfg.Servers)))

	activeTunnels := 0
	if m.tunnelMgr != nil {
		for _, t := range m.tunnelMgr.GetTunnels() {
			if t.Status == "active" {
				activeTunnels++
			}
		}
	}
	tunnelStatus := ""
	if activeTunnels > 0 {
		tunnelStatus = lipgloss.NewStyle().Foreground(colorSecondary).Bold(true).Render(fmt.Sprintf(" • %d Tunnels Active", activeTunnels))
	}

	left := lipgloss.JoinHorizontal(lipgloss.Center, title, " ", info, tunnelStatus)
	return lipgloss.NewStyle().Width(width).BorderBottom(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(colorMuted).Render(left)
}

func renderLeftPanel(m *Model, width, height int) string {
	var b strings.Builder

	// Search filter row
	if m.filterMode {
		b.WriteString(styleSubtitle.Render("Search: ") + m.filterInput.View() + "\n")
	} else if m.filterQuery != "" {
		b.WriteString(styleSubtitle.Render("Filter: ") + m.filterQuery + styleFooter.Render(" (Esc to clear)") + "\n")
	} else {
		b.WriteString(styleDesc.Render("Servers [/ to search]") + "\n")
	}

	servers := m.filteredServers
	if len(servers) == 0 {
		b.WriteString("\n" + styleFooter.Render("No matching servers found.\nPress 'a' to add one."))
	} else {
		currentGroup := ""
		for idx, s := range servers {
			grp := s.EffectiveGroup()
			if grp != currentGroup {
				currentGroup = grp
				b.WriteString(styleGroupHeader.Render("▾ "+currentGroup) + "\n")
			}

			cursor := "  "
			isCur := idx == m.cursor
			if isCur {
				cursor = "▶ "
			}

			badges := ""
			if s.Bastion != "" {
				badges += " " + badgeBastion.Render()
			}
			if len(s.Tunnels) > 0 {
				badges += " " + badgeTunnel.Render()
			}
			if s.AuthMethod == "key" || s.PrivateKeyPath != "" {
				badges += " " + badgeKey.Render()
			} else if s.Password != "" {
				badges += " " + badgePwd.Render()
			}

			line := fmt.Sprintf("%s%-16s%s", cursor, s.Name, badges)
			if isCur {
				b.WriteString(styleSelectedItem.Render(line) + "\n")
			} else {
				b.WriteString(styleNormalItem.Render(line) + "\n")
			}
		}
	}

	panelStyle := stylePanel
	if !m.rightPanelActive {
		panelStyle = styleActivePanel
	}
	return panelStyle.Width(width).Height(height).Render(b.String())
}

func renderRightPanel(m *Model, width, height int) string {
	topHeight := height*60/100 - 1
	botHeight := height - topHeight - 2
	if topHeight < 6 {
		topHeight = 6
	}
	if botHeight < 5 {
		botHeight = 5
	}

	topPanel := renderDetailsPanel(m, width, topHeight)
	botPanel := renderMonitorPanel(m, width, botHeight)

	return lipgloss.JoinVertical(lipgloss.Left, topPanel, botPanel)
}

func renderDetailsPanel(m *Model, width, height int) string {
	s := m.selectedServer()
	if s == nil {
		return stylePanel.Width(width).Height(height).Render(styleFooter.Render("Select a server to view details."))
	}

	var b strings.Builder
	b.WriteString(styleTitle.Render("Host Details: " + s.Name))
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("%-12s: %s@%s:%d\n", styleSubtitle.Render("Endpoint"), s.User, s.Host, s.EffectivePort()))
	b.WriteString(fmt.Sprintf("%-12s: %s\n", styleSubtitle.Render("Group"), s.EffectiveGroup()))

	authDesc := s.AuthMethod
	if authDesc == "" {
		authDesc = "auto"
	}
	if s.PrivateKeyPath != "" {
		authDesc += fmt.Sprintf(" (Key: %s)", s.PrivateKeyPath)
	}
	b.WriteString(fmt.Sprintf("%-12s: %s\n", styleSubtitle.Render("Auth Method"), authDesc))

	// Bastion Chain
	if s.Bastion != "" {
		chain := m.buildBastionChainString(s)
		b.WriteString(fmt.Sprintf("%-12s: %s\n", styleSubtitle.Render("Jump Chain"), badgeBastion.Render()+" "+chain))
	}

	// Tunnels
	b.WriteString("\n" + styleSubtitle.Render("Port Forwarding Rules (-L / -R / -D):") + "\n")
	if len(s.Tunnels) == 0 {
		b.WriteString(styleFooter.Render("  No forwarding rules configured (Press 'T' to add)\n"))
	} else {
		for i, r := range s.Tunnels {
			prefix := fmt.Sprintf("  [%s]", strings.ToUpper(r.Type))
			desc := r.Description
			if desc != "" {
				desc = " (" + desc + ")"
			}
			target := r.Remote
			if r.Type == "dynamic" || r.Type == "socks5" {
				target = "SOCKS5 Proxy"
			}
			b.WriteString(fmt.Sprintf("%s %s -> %s%s\n", styleKeyHint.Render(prefix), r.Local, target, desc))
			if i >= 4 && len(s.Tunnels) > 5 {
				b.WriteString(styleFooter.Render(fmt.Sprintf("  ... and %d more rules\n", len(s.Tunnels)-5)))
				break
			}
		}
	}

	if s.Notes != "" {
		b.WriteString("\n" + styleSubtitle.Render("Notes:") + " " + s.Notes + "\n")
	}

	return stylePanel.Width(width).Height(height).Render(b.String())
}

func renderMonitorPanel(m *Model, width, height int) string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Diagnostics & Tunnels Monitor"))
	b.WriteString("\n")

	// Last test result
	if m.testResult != nil {
		res := m.testResult
		statusColor := colorSecondary
		statusText := "SUCCESS"
		if !res.Success {
			statusColor = colorDanger
			statusText = "FAILED"
		}
		b.WriteString(fmt.Sprintf("Connection Probe: %s | Latency: %d ms | Server: %s\n",
			lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render(statusText),
			res.LatencyMs,
			res.ServerVersion,
		))
		if res.Message != "" {
			b.WriteString(fmt.Sprintf("Result Info: %s\n", res.Message))
		}
	} else {
		b.WriteString(styleFooter.Render("Press 'p' to probe target connection latency and server fingerprint.\n"))
	}

	// Active running tunnels
	if m.tunnelMgr != nil {
		tunnels := m.tunnelMgr.GetTunnels()
		if len(tunnels) > 0 {
			b.WriteString(styleSubtitle.Render("Active Tunnels:") + "\n")
			for _, t := range tunnels {
				statusStyle := lipgloss.NewStyle().Foreground(colorSecondary)
				if t.Status == "error" {
					statusStyle = lipgloss.NewStyle().Foreground(colorDanger)
				}
				b.WriteString(fmt.Sprintf(" • [%s] %s -> %s | %s (In: %d B, Out: %d B)\n",
					t.Rule.Type, t.Rule.Local, t.Rule.Remote, statusStyle.Render(t.Status), t.BytesIn, t.BytesOut))
			}
		}
	}

	return stylePanel.Width(width).Height(height).Render(b.String())
}

func renderFooter(m *Model, width int) string {
	msg := m.statusMsg
	if msg == "" {
		shortcuts := []string{
			styleKeyHint.Render("Enter") + ":Connect",
			styleKeyHint.Render("t") + ":Tunnels",
			styleKeyHint.Render("c") + ":CopyKey",
			styleKeyHint.Render("p") + ":Probe",
			styleKeyHint.Render("a") + ":Add",
			styleKeyHint.Render("e") + ":Edit",
			styleKeyHint.Render("d") + ":Del",
			styleKeyHint.Render("T") + ":+Tunnel",
			styleKeyHint.Render("s") + ":Gist",
			styleKeyHint.Render("/") + ":Find",
			styleKeyHint.Render("q") + ":Quit",
		}
		msg = strings.Join(shortcuts, " | ")
	} else {
		msg = styleSubtitle.Render("● ") + msg
	}
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Render(msg)
}

func (m *Model) buildBastionChainString(s *config.Server) string {
	var chain []string
	seen := make(map[string]bool)
	curr := s

	for curr != nil && curr.Bastion != "" {
		if seen[curr.Name] {
			chain = append(chain, "[CYCLE ERROR]")
			break
		}
		seen[curr.Name] = true
		bastionName := curr.Bastion
		bServer, ok := m.cfg.Servers[bastionName]
		if !ok {
			chain = append(chain, fmt.Sprintf("%s (unknown)", bastionName))
			break
		}
		chain = append([]string{fmt.Sprintf("%s (%s)", bServer.Name, bServer.Host)}, chain...)
		curr = &bServer
	}

	if len(chain) == 0 {
		return "Direct"
	}
	return strings.Join(chain, " ➔ ") + " ➔ " + s.Name
}

func (m *Model) selectedServer() *config.Server {
	if len(m.filteredServers) == 0 || m.cursor < 0 || m.cursor >= len(m.filteredServers) {
		return nil
	}
	s := m.filteredServers[m.cursor]
	return &s
}
