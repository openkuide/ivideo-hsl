package tui

import "github.com/charmbracelet/lipgloss"

var (
	colPrimary  = lipgloss.Color("#7C3AED") // violet
	colAccent   = lipgloss.Color("#22D3EE") // cyan
	colSuccess  = lipgloss.Color("#10B981")
	colWarn     = lipgloss.Color("#F59E0B")
	colError    = lipgloss.Color("#EF4444")
	colDim      = lipgloss.Color("#6B7280")
	colText     = lipgloss.Color("#E5E7EB")
	colMuted    = lipgloss.Color("#9CA3AF")
	colBgSelect = lipgloss.Color("#1F2937")
)

var (
	styleTitle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colPrimary).
			Bold(true).
			Padding(0, 2)

	styleSubtitle = lipgloss.NewStyle().
			Foreground(colAccent).
			Italic(true)

	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colPrimary).
			Padding(0, 1)

	styleHelp = lipgloss.NewStyle().
			Foreground(colDim).
			Italic(true)

	styleSelected = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colBgSelect).
			Bold(true)

	styleCheck     = lipgloss.NewStyle().Foreground(colSuccess).Bold(true)
	styleUnchecked = lipgloss.NewStyle().Foreground(colDim)

	styleSuccess = lipgloss.NewStyle().Foreground(colSuccess).Bold(true)
	styleError   = lipgloss.NewStyle().Foreground(colError).Bold(true)
	styleWarn    = lipgloss.NewStyle().Foreground(colWarn)
	styleInfo    = lipgloss.NewStyle().Foreground(colText)
	styleDim     = lipgloss.NewStyle().Foreground(colDim)
	styleMuted   = lipgloss.NewStyle().Foreground(colMuted)
	styleAccent  = lipgloss.NewStyle().Foreground(colAccent).Bold(true)

	styleBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colAccent).
			Padding(0, 1).
			Bold(true)
)

const (
	iconCheckOn  = "●"
	iconCheckOff = "○"
	iconArrow    = "▶"
	iconDot      = "·"
	iconSpark    = "✦"
)
