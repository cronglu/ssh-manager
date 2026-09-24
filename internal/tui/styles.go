package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Palette
	colorPrimary   = lipgloss.Color("#7D56F4") // Purple
	colorSecondary = lipgloss.Color("#04B575") // Green
	colorAccent    = lipgloss.Color("#EE6FF8") // Pink
	colorWarning   = lipgloss.Color("#FFB86C") // Orange
	colorDanger    = lipgloss.Color("#FF5555") // Red
	colorMuted     = lipgloss.Color("#6272A4") // Gray/Muted
	colorBg        = lipgloss.Color("#1E1E2E")
	colorHighlight = lipgloss.Color("#2A2A3C")

	// Base panel styles
	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1)

	styleActivePanel = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(0, 1)

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Padding(0, 1)

	styleSubtitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSecondary)

	// List item styles
	styleNormalItem = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8F8F2"))

	styleSelectedItem = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#50FA7B")).
				Background(colorHighlight)

	styleGroupHeader = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorWarning).
				PaddingTop(1)

	// Badges
	badgeBastion = lipgloss.NewStyle().
			Foreground(colorAccent).
			SetString("[jump]")

	badgeKey = lipgloss.NewStyle().
			Foreground(colorSecondary).
			SetString("[key]")

	badgeTunnel = lipgloss.NewStyle().
			Foreground(colorPrimary).
			SetString("[tunnel]")

	badgePwd = lipgloss.NewStyle().
			Foreground(colorMuted).
			SetString("[pwd]")

	// Status & Footer
	styleFooter = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)

	styleKeyHint = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	styleDesc = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8F8F2"))

	// Modal / Form styles
	styleModal = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(colorPrimary).
			Background(colorBg).
			Padding(1, 2)

	styleInputActive = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(colorSecondary).
				Padding(0, 1)

	styleInputInactive = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(colorMuted).
				Padding(0, 1)
)
