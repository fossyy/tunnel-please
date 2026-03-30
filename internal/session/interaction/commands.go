package interaction

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) handleCommandSelection(item commandItem) (tea.Model, tea.Cmd) {
	switch item.name {
	case "slug":
		m.showingCommands = false
		m.editingSlug = true
		m.slugInput.SetValue(m.interaction.slug.String())
		m.slugInput.Focus()
		return m, tea.Batch(tea.ClearScreen, textinput.Blink)
	case "tunnel-type":
		m.showingCommands = false
		m.showingComingSoon = true
		return m, tea.Batch(tickCmd(5*time.Second), tea.ClearScreen, textinput.Blink)
	default:
		m.showingCommands = false
		return m, nil
	}
}

func (m *model) commandsUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch {
	case key.Matches(msg, m.keymap.quit), msg.String() == "esc":
		m.showingCommands = false
		return m, tea.Batch(tea.ClearScreen, textinput.Blink)
	case msg.String() == "enter":
		selectedItem := m.commandList.SelectedItem()
		if selectedItem != nil {
			item := selectedItem.(commandItem)
			return m.handleCommandSelection(item)
		}
	}
	m.commandList, cmd = m.commandList.Update(msg)
	return m, cmd
}

func (m *model) commandsView() string {
	isCompact := shouldUseCompactLayout(m.width, 60)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		PaddingTop(1).
		PaddingBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		Italic(true).
		MarginTop(1)

	var b strings.Builder
	b.WriteString("\n")

	var title string
	if shouldUseCompactLayout(m.width, 40) {
		title = "Commands"
	} else {
		title = "⚡ Commands"
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(m.commandList.View())
	b.WriteString("\n")

	var helpText string
	if isCompact {
		helpText = "↑/↓ Nav • Enter Select • Esc Cancel"
	} else {
		helpText = "↑/↓ Navigate • Enter Select • Esc Cancel"
	}
	b.WriteString(helpStyle.Render(helpText))

	return b.String()
}
