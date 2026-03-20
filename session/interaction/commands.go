package interaction

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) handleCommandSelection(item commandItem) (tea.Model, tea.Cmd) {
	switch item.name {
	case "slug":
		m.showingCommands = false
		m.editingSlug = true
		m.slugInput.SetValue(m.interaction.slug.String())
		m.slugInput.Focus()
		return m, nil
	case "tunnel-type":
		m.showingCommands = false
		m.showingComingSoon = true
		return m, tickCmd(5 * time.Second)
	default:
		m.showingCommands = false
		return m, nil
	}
}

func (m *model) commandsUpdate(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keymap.quit), msg.String() == "esc":
		m.showingCommands = false
		return m, nil
	case msg.String() == "enter":
		selectedItem := m.commandList.SelectedItem()
		if selectedItem != nil {
			item := selectedItem.(commandItem)
			return m.handleCommandSelection(item)
		}
	}
	m.commandList, _ = m.commandList.Update(msg)
	return m, nil
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
