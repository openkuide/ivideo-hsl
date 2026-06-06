package main

import tea "github.com/charmbracelet/bubbletea"

func teaNewProgram(m tea.Model) *tea.Program {
	return tea.NewProgram(m, tea.WithAltScreen())
}
